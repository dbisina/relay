package main

import (
	"strings"
	"testing"
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
