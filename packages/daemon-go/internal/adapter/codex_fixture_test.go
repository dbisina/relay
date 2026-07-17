// internal/adapter/codex_fixture_test.go
//
// Recorded-fixture tests for the Codex `codex exec --json` line parser
// (parseCodexLine) and the CLIAdapter fallthrough/safe-pause behaviour.
// Each line of testdata/codex.jsonl is one synthesized JSONL event; the
// expectation table below is index-aligned with the fixture, so the two
// files must be edited together.

package adapter

import "testing"

func TestCodexParseLineFixtures(t *testing.T) {
	lines := readFixtureLines(t, "codex.jsonl")

	cases := []struct {
		name          string
		wantOK        bool
		wantType      AgentEventType
		wantContent   string            // rawLine = the fixture line itself
		wantMeta      map[string]string // asserted key by key
		absentMeta    []string          // keys that must NOT be set
		wantSafePause bool
	}{
		{
			name:        "agent_message text",
			wantOK:      true,
			wantType:    EventText,
			wantContent: "Refund flow wired up.",
		},
		{
			name:        "agent_message whitespace-only falls to system",
			wantOK:      true,
			wantType:    EventSystem,
			wantContent: "item.completed",
		},
		{
			name:        "turn.completed usage sums cached and reasoning tokens",
			wantOK:      true,
			wantType:    EventToolResult,
			wantContent: "turn.completed",
			wantMeta: map[string]string{
				"tokens_in":  "1500", // input_tokens + cached_input_tokens
				"tokens_out": "500",  // output_tokens + reasoning_output_tokens
			},
		},
		{
			name:        "turn.completed with zero usage omits token meta",
			wantOK:      true,
			wantType:    EventToolResult,
			wantContent: "turn.completed",
			absentMeta:  []string{"tokens_in", "tokens_out"},
		},
		{
			name:          "command_execution git commit synthesises safe pause",
			wantOK:        true,
			wantType:      EventToolUse,
			wantContent:   "command",
			wantMeta:      map[string]string{"input": `git commit -m "wip"`},
			wantSafePause: true,
		},
		{
			name:        "command_execution on item.started",
			wantOK:      true,
			wantType:    EventToolUse,
			wantContent: "command",
			wantMeta:    map[string]string{"input": "ls"},
		},
		{
			name:        "file_change keeps raw line as input",
			wantOK:      true,
			wantType:    EventToolUse,
			wantContent: "edit",
			wantMeta:    map[string]string{"input": rawLine},
		},
		{
			name:        "mcp_tool_call keeps raw line as input",
			wantOK:      true,
			wantType:    EventToolUse,
			wantContent: "mcp",
			wantMeta:    map[string]string{"input": rawLine},
		},
		{
			name:        "unknown item type falls to system",
			wantOK:      true,
			wantType:    EventSystem,
			wantContent: "item.completed",
		},
		{
			name:        "turn.failed is an error event",
			wantOK:      true,
			wantType:    EventError,
			wantContent: rawLine,
		},
		{
			name:        "error is an error event",
			wantOK:      true,
			wantType:    EventError,
			wantContent: rawLine,
		},
		{
			name:        "thread.started is a system event",
			wantOK:      true,
			wantType:    EventSystem,
			wantContent: "thread.started",
		},
		{
			name:   "unknown top-level type falls through",
			wantOK: false,
		},
		{
			name:   "non-JSON noise falls through",
			wantOK: false,
		},
	}

	if len(lines) != len(cases) {
		t.Fatalf("fixture has %d lines, expectation table has %d: edit them together", len(lines), len(cases))
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := lines[i]
			ev, ok := parseCodexLine(line)
			if ok != tc.wantOK {
				t.Fatalf("parseCodexLine ok = %v, want %v (event %+v)", ok, tc.wantOK, ev)
			}
			if !tc.wantOK {
				// Fallthrough path: the CLIAdapter must degrade the line to
				// plain text, exactly as emitted.
				fallback := NewCodexAdapter().toEvent(line)
				if fallback.Type != EventText || fallback.Content != line {
					t.Errorf("toEvent fallback = %+v, want text event with raw line", fallback)
				}
				return
			}

			if ev.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", ev.Type, tc.wantType)
			}
			want := tc.wantContent
			if want == rawLine {
				want = line
			}
			if ev.Content != want {
				t.Errorf("Content = %q, want %q", ev.Content, want)
			}
			for k, v := range tc.wantMeta {
				wantVal := v
				if wantVal == rawLine {
					wantVal = line
				}
				if got := ev.Meta[k]; got != wantVal {
					t.Errorf("Meta[%q] = %q, want %q", k, got, wantVal)
				}
			}
			for _, k := range tc.absentMeta {
				if got, present := ev.Meta[k]; present {
					t.Errorf("Meta[%q] = %q, want absent", k, got)
				}
			}

			a := NewCodexAdapter()
			a.checkSafePause(ev, line)
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
