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
	"os/exec"
	"path/filepath"
	"strings"
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

// git runs a git command in RepoDir, returning combined stdout.
func (m *Manager) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.RepoDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, out.String())
	}
	return out.String(), nil
}
