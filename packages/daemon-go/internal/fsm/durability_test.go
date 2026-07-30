// internal/fsm/durability_test.go — regression for the P0 worktree bug:
// snapshots must land in the directory SetWorkDir points at (the agent's
// session worktree), never in the directory the manager was constructed with
// (the user's checkout).

package fsm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "config", "user.email", "test@relay.local")
	gitIn(t, dir, "config", "user.name", "relay test")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "seed")
	return dir
}

func TestEmergencySnapshotCapturesInFlightWork(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	sessionRepo := initRepo(t)
	stateDir := t.TempDir()

	d, err := NewDurabilityManager(sessionRepo, stateDir, "sessEMER1")
	if err != nil {
		t.Fatal(err)
	}
	defer d.ReleaseLock() //nolint:errcheck

	// In-flight, uncommitted work in the session tree — exactly what a limit
	// hit mid-task leaves behind, and exactly what the receiver must inherit.
	wip := filepath.Join(sessionRepo, "in-flight.txt")
	if err := os.WriteFile(wip, []byte("half-finished feature\n"), 0600); err != nil {
		t.Fatal(err)
	}

	sha, err := d.EmergencySnapshot("sessEMER1")
	if err != nil {
		t.Fatalf("EmergencySnapshot: %v", err)
	}
	if sha == "" || strings.HasPrefix(sha, "nogit-") {
		t.Fatalf("expected a real commit SHA, got %q", sha)
	}

	// The whole point: the returned SHA's tree must contain the in-flight file.
	// The old stash-and-branch-from-clean-HEAD path returned a SHA whose tree
	// was empty of WIP, which is the bug this guards against.
	tree := gitIn(t, sessionRepo, "ls-tree", "-r", "--name-only", sha)
	if !strings.Contains(tree, "in-flight.txt") {
		t.Fatalf("emergency snapshot %s does not contain the in-flight work; tree was:\n%s", sha, tree)
	}

	// And the file content is the WIP content, not some stale version.
	blob := gitIn(t, sessionRepo, "show", sha+":in-flight.txt")
	if !strings.Contains(blob, "half-finished feature") {
		t.Errorf("in-flight content not preserved in snapshot: %q", blob)
	}
}

func TestSnapshotFollowsSetWorkDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	userRepo := initRepo(t)    // the user's checkout
	sessionRepo := initRepo(t) // stands in for the per-session worktree
	stateDir := t.TempDir()

	d, err := NewDurabilityManager(userRepo, stateDir, "sess1234")
	if err != nil {
		t.Fatal(err)
	}
	defer d.ReleaseLock() //nolint:errcheck

	// The orchestrator re-points durability after creating the worktree.
	d.SetWorkDir(sessionRepo)

	// User WIP in the main checkout that must NOT be committed.
	if err := os.WriteFile(filepath.Join(userRepo, "user-wip.txt"), []byte("precious\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Agent work in the session tree that must be captured.
	agentFile := filepath.Join(sessionRepo, "agent.txt")
	if err := os.WriteFile(agentFile, []byte("agent change\n"), 0600); err != nil {
		t.Fatal(err)
	}

	sha, err := d.Snapshot("sess1234", []string{"agent.txt"})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if sha == "" || strings.HasPrefix(sha, "nogit-") {
		t.Fatalf("expected a real commit SHA, got %q", sha)
	}

	// Snapshot commit exists in the session repo…
	if got := gitIn(t, sessionRepo, "rev-parse", "HEAD"); got != sha {
		t.Errorf("session repo HEAD = %s, want snapshot %s", got, sha)
	}
	if msg := gitIn(t, sessionRepo, "log", "-1", "--format=%s"); !strings.Contains(msg, "relay: snapshot") {
		t.Errorf("session repo HEAD is not a snapshot commit: %q", msg)
	}

	// …and the user's checkout is untouched: still one commit, WIP unstaged.
	if n := gitIn(t, userRepo, "rev-list", "--count", "HEAD"); n != "1" {
		t.Errorf("user repo gained commits: %s", n)
	}
	status := gitIn(t, userRepo, "status", "--porcelain")
	if !strings.Contains(status, "user-wip.txt") {
		t.Errorf("user WIP disappeared from status: %q", status)
	}
	if strings.Contains(status, "A ") {
		t.Errorf("user WIP was staged: %q", status)
	}
}
