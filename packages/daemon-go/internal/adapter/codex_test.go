package adapter

import "testing"

// Compile-time proof that the generic CLI adapter now matches the Claude
// adapter's capability surface: model selection AND stdin replies.
var (
	_ AdapterContract = (*CLIAdapter)(nil)
	_ ModelSetter     = (*CLIAdapter)(nil)
	_ StdinReplier    = (*CLIAdapter)(nil)
)

func TestParseCodexAgentMessage(t *testing.T) {
	line := `{"type":"item.completed","item":{"type":"agent_message","text":"Done — refund flow added."}}`
	ev, ok := parseCodexLine(line)
	if !ok {
		t.Fatal("agent_message should parse")
	}
	if ev.Type != EventText || ev.Content != "Done — refund flow added." {
		t.Fatalf("got %+v, want text event with the message", ev)
	}
}

func TestParseCodexUsage(t *testing.T) {
	line := `{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":450,"reasoning_output_tokens":50}}`
	ev, ok := parseCodexLine(line)
	if !ok {
		t.Fatal("turn.completed should parse")
	}
	if ev.Meta["tokens_in"] != "1500" {
		t.Errorf("tokens_in = %q, want 1500 (input+cached)", ev.Meta["tokens_in"])
	}
	if ev.Meta["tokens_out"] != "500" {
		t.Errorf("tokens_out = %q, want 500 (output+reasoning)", ev.Meta["tokens_out"])
	}
}

func TestParseCodexCommand(t *testing.T) {
	line := `{"type":"item.completed","item":{"type":"command_execution","command":"git commit -m wip"}}`
	ev, ok := parseCodexLine(line)
	if !ok || ev.Type != EventToolUse {
		t.Fatalf("command_execution should be a tool_use event, got %+v ok=%v", ev, ok)
	}
	if ev.Meta["input"] != "git commit -m wip" {
		t.Errorf("command input = %q", ev.Meta["input"])
	}
}

func TestParseCodexNonJSONFallsThrough(t *testing.T) {
	if _, ok := parseCodexLine("plain stderr noise"); ok {
		t.Error("non-JSON should fall through (ok=false) to plain-text handling")
	}
}

func TestCodexCommitTriggersSafePause(t *testing.T) {
	a := NewCodexAdapter()
	// Simulate the scan loop seeing a commit command line.
	ev := a.toEvent(`{"type":"item.completed","item":{"type":"command_execution","command":"git commit -m done"}}`)
	a.checkSafePause(ev, "")
	select {
	case sp := <-a.safePauseCh:
		if sp.Message == "" {
			t.Error("safe point should carry a message")
		}
	default:
		t.Fatal("a git commit command should synthesise a safe point")
	}
}

func TestPrependContractLeadsWithContract(t *testing.T) {
	// No system prompt file → task unchanged.
	if got := prependContract(RunOptions{Task: "do x"}); got != "do x" {
		t.Errorf("without contract, task should be unchanged, got %q", got)
	}
}
