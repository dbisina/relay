package config

import (
	"path/filepath"
	"testing"
)

const accountsTOML = `
[providers.claude]
enabled = true

[providers.claude.accounts.personal]
active = true
config_dir = "/home/me/.claude"

[providers.claude.accounts.work]
config_dir = "/home/me/.claude-work"
env = ["X=1", "Y=2"]
`

func TestParseAccountsAndEnv(t *testing.T) {
	cfg, err := parseTOML(Default("/tmp/x"), accountsTOML)
	if err != nil {
		t.Fatal(err)
	}
	pc := cfg.Providers["claude"]
	if pc == nil || len(pc.Accounts) != 2 {
		t.Fatalf("want 2 claude accounts, got %+v", pc)
	}

	// Active is "personal" → CLAUDE_CONFIG_DIR points at its dir, no extra env.
	if env := cfg.AccountEnv("claude"); len(env) != 1 || env[0] != "CLAUDE_CONFIG_DIR=/home/me/.claude" {
		t.Fatalf("active env = %v, want [CLAUDE_CONFIG_DIR=/home/me/.claude]", env)
	}

	// Switch to "work" → its config dir + explicit env overrides, in order.
	cfg.ApplyActiveAccounts(map[string]string{"claude": "work"})
	env := cfg.AccountEnv("claude")
	want := []string{"CLAUDE_CONFIG_DIR=/home/me/.claude-work", "X=1", "Y=2"}
	if len(env) != len(want) {
		t.Fatalf("work env = %v, want %v", env, want)
	}
	for i := range want {
		if env[i] != want[i] {
			t.Fatalf("work env[%d] = %q, want %q", i, env[i], want[i])
		}
	}

	// AllAccountEnv only includes providers that actually have accounts.
	all := cfg.AllAccountEnv()
	if _, ok := all["claude"]; !ok || len(all) != 1 {
		t.Fatalf("AllAccountEnv = %v, want only claude", all)
	}
}

func TestActiveAccountsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sel := map[string]string{"claude": "work", "codex": "personal"}
	if err := SaveActiveAccounts(dir, sel); err != nil {
		t.Fatal(err)
	}
	got, err := LoadActiveAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["claude"] != "work" || got["codex"] != "personal" {
		t.Fatalf("round trip mismatch: %v", got)
	}

	// A missing file is not an error — it means "use TOML defaults".
	empty, err := LoadActiveAccounts(filepath.Join(dir, "does-not-exist"))
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing file should be empty/no-error, got %v err %v", empty, err)
	}
}

func TestNoAccountsYieldsNilEnv(t *testing.T) {
	cfg := Default("/tmp/x")
	if env := cfg.AccountEnv("claude"); env != nil {
		t.Errorf("provider without accounts should yield nil env, got %v", env)
	}
}
