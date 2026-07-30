// internal/worktree/worktree.go
//
// Per-session git worktrees — agents operate on an isolated branch so they
// can never destroy the user's working tree. Lifecycle:
//   Create  → git worktree add .relay/sessions/<id> -b relay/<id>
//   Diff    → git diff main...relay/<id>
//   Merge   → user opts in: fast-forward or PR
//   Discard → git worktree remove --force; git branch -D relay/<id>

package worktree

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dbisina/relay/internal/process"
)

// Manager — owns one worktree per session.
type Manager struct {
	// RepoDir is the user's main repository working directory.
	RepoDir string
	// BaseRef is the branch the worktree was branched from (e.g. "main").
	BaseRef string
}

// Session — a created worktree.
type Session struct {
	ID     string
	Path   string // absolute path to the worktree
	Branch string // e.g. relay/abc123
}

// New constructs a Manager. BaseRef defaults to HEAD.
func New(repoDir, baseRef string) *Manager {
	if baseRef == "" {
		baseRef = "HEAD"
	}
	return &Manager{RepoDir: repoDir, BaseRef: baseRef}
}

// IsGitRepo reports whether RepoDir is inside a git repository.
func (m *Manager) IsGitRepo() bool {
	out, err := m.git("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Create makes a fresh worktree at .relay/sessions/<id> on a new branch.
// Returns the Session on success; ("", error) if anything fails.
// If the repo isn't a git repo, returns (nil, nil) — caller works directly.
func (m *Manager) Create(sessionID string) (*Session, error) {
	if !m.IsGitRepo() {
		return nil, nil
	}
	branch := "relay/" + sessionID[:8]
	path := filepath.Join(m.RepoDir, ".relay", "sessions", sessionID[:8])

	if _, err := m.git("worktree", "add", "-b", branch, path, m.BaseRef); err != nil {
		// If branch already exists (resume case), try plain add
		if _, err2 := m.git("worktree", "add", path, branch); err2 != nil {
			return nil, fmt.Errorf("worktree add: %w", err)
		}
	}
	return &Session{ID: sessionID, Path: path, Branch: branch}, nil
}

// Diff returns `git diff <base>...<branch>` as a unified diff.
func (m *Manager) Diff(s *Session) (string, error) {
	if s == nil {
		return "", nil
	}
	return m.git("diff", m.BaseRef+"..."+s.Branch)
}

// DiffSummary returns name+stat summary suitable for tabular display.
func (m *Manager) DiffSummary(s *Session) (string, error) {
	if s == nil {
		return "", nil
	}
	return m.git("diff", "--stat", m.BaseRef+"..."+s.Branch)
}

// Merge fast-forwards BaseRef to the session branch (requires clean BaseRef).
func (m *Manager) Merge(s *Session) error {
	if s == nil {
		return nil
	}
	_, err := m.git("merge", "--ff-only", s.Branch)
	return err
}

// Discard removes the worktree and deletes the branch.
func (m *Manager) Discard(s *Session) error {
	if s == nil {
		return nil
	}
	_, _ = m.git("worktree", "remove", "--force", s.Path)
	_, _ = m.git("branch", "-D", s.Branch)
	return nil
}

// DirtySnapshot is RepoDir's uncommitted state, captured without mutating
// RepoDir's working tree or index — safe to call while another process (an
// agent Relay did not spawn) may still be actively editing there.
type DirtySnapshot struct {
	Patch     string          // `git diff HEAD` — tracked, unstaged and staged changes
	Untracked []UntrackedFile // files git does not know about at all
}

// UntrackedFile is a file's full content at capture time.
type UntrackedFile struct {
	Path    string // relative to RepoDir
	Content []byte
}

// CaptureUncommittedDiff reads everything different from HEAD in RepoDir:
// tracked changes via `git diff` (read-only, mutates nothing) and untracked
// files via a plain filesystem read (never staged, so RepoDir's index is
// never touched either). Returns (nil, nil) when there is nothing to carry —
// a clean tree, or RepoDir is not a git repo.
func (m *Manager) CaptureUncommittedDiff() (*DirtySnapshot, error) {
	if !m.IsGitRepo() {
		return nil, nil
	}
	// -c core.autocrlf=false regardless of the repo's or the machine's global
	// git config: this is copying code, not normalising it, so the captured
	// bytes must match what was actually on disk. Without this, a common
	// Windows git config ("Checkout Windows-style, commit Unix-style") can
	// make the diff itself carry translated endings.
	patch, err := m.git("-c", "core.autocrlf=false", "diff", "HEAD")
	if err != nil {
		// A repo with no commits yet has no HEAD to diff against; that is not
		// a capture failure, just nothing tracked to carry over.
		patch = ""
	}

	listing, err := m.git("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("worktree: list untracked files: %w", err)
	}
	var untracked []UntrackedFile
	for _, rel := range strings.Split(listing, "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(m.RepoDir, rel))
		if rerr != nil {
			// Best-effort: the source may still be actively edited, a file can
			// vanish between listing and reading. Skip it rather than fail the
			// whole capture over one file.
			continue
		}
		untracked = append(untracked, UntrackedFile{Path: rel, Content: data})
	}

	if strings.TrimSpace(patch) == "" && len(untracked) == 0 {
		return nil, nil
	}
	return &DirtySnapshot{Patch: patch, Untracked: untracked}, nil
}

// ApplyDirtySnapshot writes snap into worktreePath — never RepoDir — so a
// freshly created, isolated worktree starts with the same live, uncommitted
// state as the directory it was captured from.
func (m *Manager) ApplyDirtySnapshot(worktreePath string, snap *DirtySnapshot) error {
	if snap == nil {
		return nil
	}
	if strings.TrimSpace(snap.Patch) != "" {
		// Same autocrlf override as capture, and for the same reason: apply the
		// patch's bytes exactly, not translated by whatever this worktree's
		// local config happens to be.
		cmd := exec.Command("git", "-c", "core.autocrlf=false", "apply", "--whitespace=nowarn", "-")
		cmd.Dir = worktreePath
		cmd.Stdin = strings.NewReader(snap.Patch)
		process.HideWindow(cmd)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("worktree: apply captured diff: %w (%s)", err, strings.TrimSpace(out.String()))
		}
	}
	for _, f := range snap.Untracked {
		dst := filepath.Join(worktreePath, f.Path)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("worktree: mkdir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(dst, f.Content, 0600); err != nil {
			return fmt.Errorf("worktree: write %s: %w", f.Path, err)
		}
	}
	return nil
}

// git runs a git command in RepoDir, returning combined stdout.
func (m *Manager) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.RepoDir
	process.HideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, out.String())
	}
	return out.String(), nil
}
