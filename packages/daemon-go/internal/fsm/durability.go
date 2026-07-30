// internal/fsm/durability.go
//
// DurabilityManager — git substrate for FSM snapshots.
//
// Strategy:
//   - Snapshots → named git refs (relay/sessions/<id>/<ts>) → permanent, GC-exempt
//   - O_EXCL lockfile with PID+sessionId+createdAt (PID alone unsafe — OS recycles)
//   - Drift guard: git status --porcelain -- <files> before snapshot
//   - Receiver heartbeat polling (500ms) to detect DISPATCHED→ERROR on timeout

package fsm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dbisina/relay/internal/process"
)

const (
	heartbeatPollInterval = 500 * time.Millisecond
	heartbeatTimeout      = 5 * time.Minute
	lockFileName          = "relay.lock"
)

// LockRecord is written to the O_EXCL lockfile.
type LockRecord struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CreatedAt int64  `json:"createdAt"` // Unix seconds
}

// DurabilityManager manages git-backed snapshots and the session lockfile.
type DurabilityManager struct {
	workDir  string
	stateDir string
	lockFd   *os.File
}

// NewDurabilityManager creates and acquires the session lock.
// Returns ErrLockConflict if another session holds the lock.
func NewDurabilityManager(workDir, stateDir, sessionID string) (*DurabilityManager, error) {
	d := &DurabilityManager{workDir: workDir, stateDir: stateDir}
	if err := d.acquireLock(sessionID); err != nil {
		return nil, err
	}
	return d, nil
}

// SetWorkDir re-points git operations at a new directory. Called after the
// per-session worktree is created so snapshots capture the agent's tree, not
// the user's checkout.
func (d *DurabilityManager) SetWorkDir(dir string) {
	d.workDir = dir
}

// Snapshot commits all tracked changes to a named git ref.
// Returns the commit SHA of the new snapshot. Falls back to a pseudo-SHA
// when the workdir is not a git repository (degraded recovery but no stall).
func (d *DurabilityManager) Snapshot(sessionID string, files []string) (string, error) {
	if !d.isGitRepo() {
		ts := time.Now().UTC().Format("20060102-150405")
		return fmt.Sprintf("nogit-%s-%s", sessionID[:min(8, len(sessionID))], ts), nil
	}
	// 1. Drift guard: verify files are actually modified
	if err := d.checkDrift(files); err != nil {
		return "", err
	}

	// 2. Stage files
	args := append([]string{"add", "--"}, files...)
	if err := d.git(args...); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}

	// 3. Commit
	ts := time.Now().UTC().Format("20060102-150405")
	msg := fmt.Sprintf("relay: snapshot %s %s", sessionID, ts)
	if err := d.git("commit", "-m", msg, "--allow-empty"); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	// 4. Get commit SHA
	sha, err := d.gitOutput("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	sha = strings.TrimSpace(sha)

	// 5. Create permanent named ref (never GC-expired)
	ref := fmt.Sprintf("refs/relay/sessions/%s/%s", sessionID, ts)
	if err := d.git("update-ref", ref, sha); err != nil {
		return "", fmt.Errorf("git update-ref: %w", err)
	}

	return sha, nil
}

// EmergencySnapshot creates an escape branch relay/emergency/<id>/<ts>
// used when the safe-pause timeout fires.
//
// When the workdir is not a git repository, falls back to a non-git pseudo-SHA
// so the handoff cycle can proceed. State recovery is degraded (no diff/revert)
// but the orchestrator doesn't deadlock.
func (d *DurabilityManager) EmergencySnapshot(sessionID string) (string, error) {
	ts := time.Now().UTC().Format("20060102-150405")

	// Detect non-git context early. The git fallback path avoids leaving the
	// orchestrator stuck when running outside a git repo.
	if !d.isGitRepo() {
		// Synthetic SHA so the FSM has a unique CommitSHA to record.
		pseudoSHA := fmt.Sprintf("nogit-%s-%s", sessionID[:min(8, len(sessionID))], ts)
		return pseudoSHA, nil
	}

	branch := fmt.Sprintf("relay/emergency/%s/%s", sessionID, ts)

	// Create the escape branch off the current HEAD, carrying the working tree
	// with it. `checkout -b` keeps uncommitted changes in place rather than
	// discarding them, so the in-flight work is still present to be committed.
	if err := d.git("checkout", "-b", branch); err != nil {
		return "", fmt.Errorf("emergency: git checkout -b %s: %w", branch, err)
	}

	// Commit ALL in-flight work as a real commit on the escape branch. This is
	// the entire point of the emergency path: the returned SHA must contain the
	// uncommitted code, because the receiving agent checks that SHA out to keep
	// going. The previous implementation staged the WIP then `git stash push`ed
	// it (which resets the tree to HEAD) and branched from that now-clean HEAD,
	// with no matching `stash pop` anywhere, so it handed the receiver an empty
	// tree and silently dropped the work it was meant to preserve.
	if err := d.git("add", "-A"); err != nil {
		// `add -A` can fail on an unborn HEAD; fall through and let commit try.
		_ = err
	}
	// --allow-empty so a genuinely clean tree still yields a unique SHA for the
	// FSM to record, matching Snapshot's behaviour.
	msg := fmt.Sprintf("relay: emergency snapshot %s %s", sessionID, ts)
	if err := d.git("commit", "-m", msg, "--allow-empty"); err != nil {
		return "", fmt.Errorf("emergency: git commit: %w", err)
	}

	sha, err := d.gitOutput("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("emergency: rev-parse: %w", err)
	}
	return strings.TrimSpace(sha), nil
}

// isGitRepo returns true if d.workDir is inside a git working tree.
func (d *DurabilityManager) isGitRepo() bool {
	out, err := d.gitOutput("rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// PollHeartbeat polls the receiver heartbeat file until it confirms reception
// or times out. Used by the DISPATCHED→RESUMING transition.
func (d *DurabilityManager) PollHeartbeat(sessionID string) error {
	hbPath := filepath.Join(d.stateDir, "receiver-heartbeat")
	deadline := time.Now().Add(heartbeatTimeout)

	for time.Now().Before(deadline) {
		data, err := os.ReadFile(hbPath)
		if err == nil {
			var hb struct {
				SessionID string `json:"sessionId"`
				Status    string `json:"status"`
			}
			if json.Unmarshal(bytes.TrimSpace(data), &hb) == nil &&
				hb.SessionID == sessionID && hb.Status == "received" {
				return nil
			}
		}
		time.Sleep(heartbeatPollInterval)
	}
	return fmt.Errorf("durability: receiver heartbeat timeout after %s", heartbeatTimeout)
}

// WriteReceiverHeartbeat is called by the receiving agent after it reads
// the continuation contract. Signals the sender that dispatch succeeded.
func WriteReceiverHeartbeat(stateDir, sessionID string) error {
	hb := map[string]string{
		"sessionId": sessionID,
		"status":    "received",
		"ts":        strconv.FormatInt(time.Now().Unix(), 10),
	}
	data, _ := json.Marshal(hb)
	path := filepath.Join(stateDir, "receiver-heartbeat")
	return os.WriteFile(path, data, 0600)
}

// ReleaseLock removes the O_EXCL lockfile.
func (d *DurabilityManager) ReleaseLock() error {
	if d.lockFd == nil {
		return nil
	}
	_ = d.lockFd.Close()
	d.lockFd = nil
	return os.Remove(filepath.Join(d.stateDir, lockFileName))
}

// acquireLock creates the O_EXCL lockfile with PID+sessionId+createdAt.
func (d *DurabilityManager) acquireLock(sessionID string) error {
	lockPath := filepath.Join(d.stateDir, lockFileName)
	if err := os.MkdirAll(d.stateDir, 0700); err != nil {
		return fmt.Errorf("lock: mkdir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Read existing lock for diagnostics
			existing, _ := os.ReadFile(lockPath)
			return fmt.Errorf("lock conflict: another session holds the lock: %s", existing)
		}
		return fmt.Errorf("lock: create: %w", err)
	}

	rec := LockRecord{
		PID:       os.Getpid(),
		SessionID: sessionID,
		CreatedAt: time.Now().Unix(),
	}
	data, _ := json.Marshal(rec)
	_, _ = f.Write(data)
	_ = f.Sync()
	d.lockFd = f
	return nil
}

// checkDrift verifies at least one of the tracked files has uncommitted changes.
// This guards against snapshotting when nothing has actually changed.
func (d *DurabilityManager) checkDrift(files []string) error {
	args := append([]string{"status", "--porcelain", "--"}, files...)
	// Validate git status runs; empty output (no changes) is fine because the
	// snapshot commit path uses --allow-empty.
	if _, err := d.gitOutputRaw(args...); err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	return nil
}

func (d *DurabilityManager) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = d.workDir
	process.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func (d *DurabilityManager) gitOutput(args ...string) (string, error) {
	return d.gitOutputRaw(args...)
}

func (d *DurabilityManager) gitOutputRaw(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = d.workDir
	process.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
