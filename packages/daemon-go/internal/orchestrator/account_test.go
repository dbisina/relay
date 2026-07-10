package orchestrator

import "testing"

func specs(labels ...string) []AccountSpec {
	out := make([]AccountSpec, len(labels))
	for i, l := range labels {
		out[i] = AccountSpec{Label: l, Env: []string{"CLAUDE_CONFIG_DIR=/cfg/" + l}}
	}
	return out
}

func TestNextAccountIndex(t *testing.T) {
	acc := specs("a", "b", "c")

	// From a, nothing exhausted → next is b.
	if got := nextAccountIndex(acc, 0, map[string]bool{}); got != 1 {
		t.Errorf("from a: got %d, want 1 (b)", got)
	}
	// From a with b exhausted → skip to c.
	if got := nextAccountIndex(acc, 0, map[string]bool{"b": true}); got != 2 {
		t.Errorf("from a, b dead: got %d, want 2 (c)", got)
	}
	// From c wraps around to a.
	if got := nextAccountIndex(acc, 2, map[string]bool{}); got != 0 {
		t.Errorf("from c: got %d, want 0 (a) cyclic", got)
	}
	// All exhausted → -1.
	if got := nextAccountIndex(acc, 0, map[string]bool{"a": true, "b": true, "c": true}); got != -1 {
		t.Errorf("all dead: got %d, want -1", got)
	}
	// Current marked exhausted (as tryAccountFailover does) must not pick itself.
	if got := nextAccountIndex(acc, 1, map[string]bool{"b": true}); got != 2 {
		t.Errorf("from b (b dead): got %d, want 2 (c)", got)
	}
}

func TestAccountEnvForAndLabel(t *testing.T) {
	o := &Orchestrator{
		opts: Options{
			ProviderAccounts:   map[string][]AccountSpec{"claude": specs("work", "personal")},
			ProviderAccountEnv: map[string][]string{"codex": {"CODEX_HOME=/x"}},
		},
		acctIdx: map[string]int{"claude": 1},
	}

	if got := o.activeAccountLabel("claude"); got != "personal" {
		t.Errorf("label = %q, want personal", got)
	}
	if env := o.accountEnvFor("claude"); len(env) != 1 || env[0] != "CLAUDE_CONFIG_DIR=/cfg/personal" {
		t.Errorf("claude env = %v, want personal cfg", env)
	}
	// Provider with no ProviderAccounts entry falls back to ProviderAccountEnv.
	if env := o.accountEnvFor("codex"); len(env) != 1 || env[0] != "CODEX_HOME=/x" {
		t.Errorf("codex env = %v, want fallback", env)
	}
	// Unknown provider → no env, no label.
	if env := o.accountEnvFor("ollama"); env != nil {
		t.Errorf("ollama env = %v, want nil", env)
	}
	if got := o.activeAccountLabel("ollama"); got != "" {
		t.Errorf("ollama label = %q, want empty", got)
	}
}
