// internal/config/config_parse_test.go — parseTOML edge cases: quote-aware
// comment stripping and comma-safe string arrays.

package config

import (
	"reflect"
	"testing"
)

func TestStripTomlCommentRespectsQuotes(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"plain"`, `"plain"`},
		{`"has # inside"`, `"has # inside"`},
		{`"value" # trailing comment`, `"value"`},
		{`"a#b" # real comment`, `"a#b"`},
		{`42 # answer`, `42`},
		{`"esc \" # still quoted" # comment`, `"esc \" # still quoted"`},
	}
	for _, c := range cases {
		if got := stripTomlComment(c.in); got != c.want {
			t.Errorf("stripTomlComment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseTomlStringArrayCommaInQuotes(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`["a", "b", "c"]`, []string{"a", "b", "c"}},
		{`["HEADERS=x-account:work,y=2", "B=1"]`, []string{"HEADERS=x-account:work,y=2", "B=1"}},
		{`[]`, nil},
		{`["single"]`, []string{"single"}},
	}
	for _, c := range cases {
		if got := parseTomlStringArray(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseTomlStringArray(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestParseTOMLQuotedValuesSurviveComments(t *testing.T) {
	cfg := Default(t.TempDir())
	input := `
[vision]
window_match = "Claude #1|Codex" # window title regex

[providers.claude]
enabled = true # keep on

  [providers.claude.accounts.work]
  env = ["ANTHROPIC_DEFAULT_HEADERS=x-account:work #1", "A=1,2"]
`
	got, err := parseTOML(cfg, input)
	if err != nil {
		t.Fatalf("parseTOML: %v", err)
	}
	if got.Vision.WindowMatch != "Claude #1|Codex" {
		t.Errorf("window_match mangled: %q", got.Vision.WindowMatch)
	}
	pc := got.Providers["claude"]
	if pc == nil || !pc.Enabled {
		t.Fatalf("claude provider not enabled: %#v", pc)
	}
	var envs []string
	for _, a := range pc.Accounts {
		if a.Label == "work" {
			envs = a.Env
		}
	}
	want := []string{"ANTHROPIC_DEFAULT_HEADERS=x-account:work #1", "A=1,2"}
	if !reflect.DeepEqual(envs, want) {
		t.Errorf("account env = %#v, want %#v", envs, want)
	}
}
