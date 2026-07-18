package main

import (
	"strings"
	"testing"

	"github.com/dbisina/relay/internal/detect"
)

// adoptedTask must inline the full brief (so the receiving agent does not depend
// on reading .relay/adopted/ itself) and lead with a resume instruction so the
// brief is treated as work to continue, not a document to summarise.
func TestAdoptedTaskWrapsBrief(t *testing.T) {
	brief := "# Continuation brief (adopted session)\n\n## Do next\n- [ ] finish the login form"
	task := adoptedTask(brief)
	if !strings.Contains(task, brief) {
		t.Error("adoptedTask must inline the brief verbatim")
	}
	if !strings.HasPrefix(task, "Continue this adopted coding session") {
		t.Errorf("adoptedTask missing resume instruction, got prefix %q", task[:32])
	}
}

func TestTargetLabel(t *testing.T) {
	if got := targetLabel(""); got != "any provider" {
		t.Errorf("targetLabel(\"\") = %q, want \"any provider\"", got)
	}
	if got := targetLabel("codex"); got != "codex" {
		t.Errorf("targetLabel(\"codex\") = %q, want \"codex\"", got)
	}
}

func TestFilterByDirNoFilterIsNoOp(t *testing.T) {
	agents := []detect.DetectedAgent{{ID: "a", WorkDir: `C:\Users\me\proj1`}}
	got := filterByDir(agents, nil)
	if len(got) != 1 {
		t.Fatalf("expected no-op with empty filter, got %d agents", len(got))
	}
}

func TestFilterByDirMatchesSubstringCaseInsensitive(t *testing.T) {
	agents := []detect.DetectedAgent{
		{ID: "a", WorkDir: `C:\Users\me\Documents\GitHub\Relay`},
		{ID: "b", WorkDir: `C:\Users\me\Downloads\some-other-project`},
		{ID: "c", WorkDir: `C:\Users\me\Documents\GitHub\jenjay`},
	}
	got := filterByDir(agents, []string{"relay", "JenJay"})
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(got), got)
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["a"] || !ids["c"] {
		t.Errorf("expected agents a and c to match, got %+v", got)
	}
}

func TestFilterByDirNoMatchesReturnsEmpty(t *testing.T) {
	agents := []detect.DetectedAgent{{ID: "a", WorkDir: `C:\Users\me\proj1`}}
	got := filterByDir(agents, []string{"nonexistent"})
	if len(got) != 0 {
		t.Errorf("expected zero matches, got %d", len(got))
	}
}
