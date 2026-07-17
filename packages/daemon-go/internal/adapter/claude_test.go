// internal/adapter/claude_test.go
//
// Recorded-fixture tests for the Claude stream-json parser (parseLine) and
// its git-commit safe-pause synthesis. Each line of testdata/claude.jsonl is
// one synthesized `claude -p --output-format stream-json` event; the
// expectation table below is index-aligned with the fixture, so the two
// files must be edited together.

package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rawLine is a sentinel wantContent meaning "the unparsed fixture line".
const rawLine = "\x00RAW_LINE\x00"

// readFixtureLines loads a JSONL fixture from testdata/, tolerating CRLF
// checkouts (core.autocrlf) and skipping blank lines.
func readFixtureLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimRight(l, "\r")
		if l == "" {
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

func TestClaudeParseLineFixtures(t *testing.T) {
	lines := readFixtureLines(t, "claude.jsonl")

	cases := []struct {
		name          string
		wantType      AgentEventType
		wantContent   string            // rawLine = the fixture line itself
		wantMeta      map[string]string // asserted key by key
		absentMeta    []string          // keys that must NOT be set
		wantSafePause bool
	}{
		{
			name:        "assistant text",
			wantType:    EventText,
			wantContent: "I will add the refund flow now.",
		},
		{
			name:        "assistant without text falls to system",
			wantType:    EventSystem,
			wantContent: rawLine,
		},
		{
			name:        "tool_use with input",
			wantType:    EventToolUse,
			wantContent: "Bash",
			wantMeta:    map[string]string{"input": `{"command":"ls -la"}`},
		},
		{
			name:        "tool_use without input defaults to empty object",
			wantType:    EventToolUse,
			wantContent: "Read",
			wantMeta:    map[string]string{"input": "{}"},
		},
		{
			name:        "tool_result string content",
			wantType:    EventToolResult,
			wantContent: "main.go\nutil.go",
			wantMeta:    map[string]string{"input": `{"command":"ls"}`},
		},
		{
			name:        "tool_result array content kept as raw JSON",
			wantType:    EventToolResult,
			wantContent: `[{"type":"text","text":"ok"}]`,
			wantMeta:    map[string]string{"input": "{}"},
		},
		{
			name:          "tool_result for git commit synthesises safe pause",
			wantType:      EventToolResult,
			wantContent:   "[main abc1234] wip: refund flow",
			wantMeta:      map[string]string{"input": `{"command":"git commit -m \"wip: refund flow\""}`},
			wantSafePause: true,
		},
		{
			name:        "result with usage extracts tokens and cost",
			wantType:    EventToolResult,
			wantContent: "result:success Refund flow added.",
			wantMeta: map[string]string{
				"tokens_in":  "2000", // input_tokens + cache_read_input_tokens
				"tokens_out": "300",
				"cost_usd":   "0.012345",
			},
		},
		{
			name:        "result without usage omits token meta",
			wantType:    EventToolResult,
			wantContent: "result:error_during_execution ",
			absentMeta:  []string{"tokens_in", "tokens_out", "cost_usd"},
		},
		{
			name:        "system init",
			wantType:    EventSystem,
			wantContent: "system:init",
		},
		{
			name:        "malformed line degrades to plain text",
			wantType:    EventText,
			wantContent: rawLine,
		},
		{
			name:        "unknown event type falls to system",
			wantType:    EventSystem,
			wantContent: rawLine,
		},
	}

	if len(lines) != len(cases) {
		t.Fatalf("fixture has %d lines, expectation table has %d: edit them together", len(lines), len(cases))
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewClaudeAdapter(0)
			ev := a.parseLine(lines[i])
			a.checkSafePause(ev)

			if ev.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", ev.Type, tc.wantType)
			}
			want := tc.wantContent
			if want == rawLine {
				want = lines[i]
			}
			if ev.Content != want {
				t.Errorf("Content = %q, want %q", ev.Content, want)
			}
			for k, v := range tc.wantMeta {
				if got := ev.Meta[k]; got != v {
					t.Errorf("Meta[%q] = %q, want %q", k, got, v)
				}
			}
			for _, k := range tc.absentMeta {
				if got, ok := ev.Meta[k]; ok {
					t.Errorf("Meta[%q] = %q, want absent", k, got)
				}
			}

			gotPause := false
			select {
			case <-a.safePauseCh:
				gotPause = true
			default:
			}
			if gotPause != tc.wantSafePause {
				t.Errorf("safe pause synthesised = %v, want %v", gotPause, tc.wantSafePause)
			}
		})
	}
}
