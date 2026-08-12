package detect

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestScanCapturesRecentTurns proves the verbatim conversation tail is lifted
// from a Claude transcript: both roles, in order, with harness scaffolding
// (system-reminder) stripped and pure tool_result turns skipped.
func TestScanCapturesRecentTurns(t *testing.T) {
	home := writeFixture(t)
	agents, err := Scan(Options{Home: home, IncludeProcesses: false, IncludeTranscripts: true, MaxAgeHours: 240})
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Session == nil {
		t.Fatalf("want 1 agent with a session, got %d", len(agents))
	}
	turns := agents[0].Session.RecentTurns
	// user "Build a login form", assistant "Starting now", (empty tool_result
	// user turn is skipped), user "Also add remember-me", assistant "Added…".
	if len(turns) != 4 {
		t.Fatalf("recentTurns = %d, want 4: %+v", len(turns), turns)
	}
	wantRoles := []string{"user", "assistant", "user", "assistant"}
	for i, w := range wantRoles {
		if turns[i].Role != w {
			t.Errorf("turn %d role = %q, want %q", i, turns[i].Role, w)
		}
	}
	if !strings.Contains(turns[0].Text, "Build a login form") {
		t.Errorf("first turn = %q, want it to contain the initial prompt", turns[0].Text)
	}
	if strings.Contains(turns[0].Text, "secret stuff") {
		t.Errorf("system-reminder scaffolding leaked into a carried turn: %q", turns[0].Text)
	}
	if turns[3].Text != "Added remember-me checkbox" {
		t.Errorf("last turn = %q", turns[3].Text)
	}
}

// TestTailTurnsWindow keeps only the last recentTurnWindow entries and copies.
func TestTailTurnsWindow(t *testing.T) {
	var in []Turn
	for i := 0; i < recentTurnWindow+5; i++ {
		in = append(in, Turn{Role: "user", Text: "t"})
	}
	out := tailTurns(in)
	if len(out) != recentTurnWindow {
		t.Fatalf("tailTurns len = %d, want %d", len(out), recentTurnWindow)
	}
	if tailTurns(nil) != nil {
		t.Error("tailTurns(nil) should be nil")
	}
	if got := tailTurns([]Turn{{Role: "user", Text: "x"}}); len(got) != 1 {
		t.Errorf("short slice len = %d, want 1", len(got))
	}
}

// TestTruncateRuneSafe proves truncate never splits a multibyte UTF-8 rune,
// which matters now that it clamps verbatim conversation turns (emoji/CJK).
func TestTruncateRuneSafe(t *testing.T) {
	// A 4-byte rune straddling the byte-1600 cut point.
	s := strings.Repeat("a", 1599) + "😀" + strings.Repeat("b", 20)
	got := truncate(s, 1600)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated string should end with the ellipsis, got %q", got[len(got)-6:])
	}
	// Under the cap: returned unchanged (after trim).
	if got := truncate("short", 1600); got != "short" {
		t.Errorf("short string = %q, want unchanged", got)
	}
}
