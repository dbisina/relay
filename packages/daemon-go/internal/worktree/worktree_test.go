// internal/worktree/worktree_test.go — regression for adoption's core promise:
// a session continuing an agent Relay did not spawn must inherit the live,
// uncommitted code that agent left behind, without ever mutating the source
// directory (which another process may still be actively editing).

package worktree

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
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "config", "user.email", "test@relay.local")
	gitIn(t, dir, "config", "user.name", "relay test")
	// Deterministic regardless of the machine's global core.autocrlf: without
	// this, `git apply` can translate the patch's line endings on Windows and
	// the byte-exact content assertions below become environment-dependent.
	gitIn(t, dir, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("committed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "seed")
	return dir
}

func TestCaptureUncommittedDiffReadOnly(t *testing.T) {
	dir := initRepo(t)

	// A tracked file with an uncommitted edit.
	tracked := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("mid-edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// A file git has never seen.
	untracked := filepath.Join(dir, "new-feature.go")
	if err := os.WriteFile(untracked, []byte("package x\n// half-written\n"), 0600); err != nil {
		t.Fatal(err)
	}

	statusBefore := gitIn(t, dir, "status", "--porcelain")

	m := New(dir, "")
	snap, err := m.CaptureUncommittedDiff()
	if err != nil {
		t.Fatalf("CaptureUncommittedDiff: %v", err)
	}
	if snap == nil {
		t.Fatal("expected a non-nil snapshot with dirty state present")
	}
	if !strings.Contains(snap.Patch, "mid-edit") {
		t.Errorf("patch missing tracked edit: %q", snap.Patch)
	}
	if len(snap.Untracked) != 1 || snap.Untracked[0].Path != "new-feature.go" {
		t.Errorf("expected exactly new-feature.go untracked, got %+v", snap.Untracked)
	}
	if !strings.Contains(string(snap.Untracked[0].Content), "half-written") {
		t.Errorf("untracked content not captured: %q", snap.Untracked[0].Content)
	}

	// The whole point: capturing must not have touched the source directory.
	statusAfter := gitIn(t, dir, "status", "--porcelain")
	if statusBefore != statusAfter {
		t.Errorf("capture mutated source repo status:\nbefore: %q\nafter:  %q", statusBefore, statusAfter)
	}
}

func TestCaptureUncommittedDiffCleanTreeReturnsNil(t *testing.T) {
	dir := initRepo(t)
	m := New(dir, "")
	snap, err := m.CaptureUncommittedDiff()
	if err != nil {
		t.Fatalf("CaptureUncommittedDiff: %v", err)
	}
	if snap != nil {
		t.Errorf("clean tree should yield a nil snapshot, got %+v", snap)
	}
}

func TestApplyDirtySnapshotLandsInWorktreeOnly(t *testing.T) {
	source := initRepo(t) // stands in for the source directory an agent adopted from
	worktreeDir := initRepo(t)

	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("mid-edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new-feature.go"), []byte("package x\n"), 0600); err != nil {
		t.Fatal(err)
	}

	m := New(source, "")
	snap, err := m.CaptureUncommittedDiff()
	if err != nil {
		t.Fatalf("CaptureUncommittedDiff: %v", err)
	}
	if snap == nil {
		t.Fatal("expected dirty snapshot")
	}

	if err := m.ApplyDirtySnapshot(worktreeDir, snap); err != nil {
		t.Fatalf("ApplyDirtySnapshot: %v", err)
	}

	// The fresh worktree now has the live edit and the untracked file.
	got, err := os.ReadFile(filepath.Join(worktreeDir, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "mid-edit\n" {
		t.Errorf("tracked edit not applied to worktree: %q", got)
	}
	if _, err := os.Stat(filepath.Join(worktreeDir, "new-feature.go")); err != nil {
		t.Errorf("untracked file not carried into worktree: %v", err)
	}

	// The source directory must be completely unaffected by applying elsewhere.
	sourceStatus := gitIn(t, source, "status", "--porcelain")
	if !strings.Contains(sourceStatus, "tracked.txt") || !strings.Contains(sourceStatus, "new-feature.go") {
		t.Errorf("source repo's own dirty state was disturbed: %q", sourceStatus)
	}
}

func TestApplyDirtySnapshotNilIsNoop(t *testing.T) {
	dir := initRepo(t)
	m := New(dir, "")
	if err := m.ApplyDirtySnapshot(dir, nil); err != nil {
		t.Errorf("nil snapshot should be a no-op, got error: %v", err)
	}
}
