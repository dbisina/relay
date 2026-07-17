// internal/redact/redact_test.go
//
// Table-driven coverage of DefaultRules: for each rule a matching case with
// the exact scrubbed output (so partial or over-eager matches surface) and a
// near-miss that must survive untouched (false-positive bound). Also covers
// rule ordering (anthropic before openai, pem before ssh) and the
// url_password structure-preserving replacement.

package redact

import (
	"strings"
	"testing"
)

func TestScrubDefaultRules(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "anthropic key",
			in:   "credential sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWx here",
			want: "credential [REDACTED:anthropic_key] here",
		},
		{
			// sk-ant-… also matches the broader openai pattern; the anthropic
			// rule must win because it runs first.
			name: "anthropic key takes precedence over openai rule",
			in:   "sk-ant-abcdefghijklmnopqrstuvwxyz",
			want: "[REDACTED:anthropic_key]",
		},
		{
			name: "openai project key",
			in:   "sk-proj-AbCd1234EfGh5678IjKl9012",
			want: "[REDACTED:openai_key]",
		},
		{
			name: "openai classic key",
			in:   "sk-AbCdEfGhIjKlMnOpQrStUv",
			want: "[REDACTED:openai_key]",
		},
		{
			name: "gemini key",
			in:   "AIzaSyD-1234567890abcdefghijklmnopqr",
			want: "[REDACTED:gemini_key]",
		},
		{
			name: "github classic PAT",
			in:   "ghp_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
			want: "[REDACTED:github_pat]",
		},
		{
			name: "github fine-grained PAT",
			in:   "github_pat_Ab1Cd2Ef3GAb1Cd2Ef3GAb1Cd2Ef3GAb1Cd2Ef3GAb1Cd2Ef3G",
			want: "[REDACTED:github_pat]",
		},
		{
			name: "slack bot token",
			// Split so the raw source text never contains a contiguous
			// Slack-token-shaped string (avoids tripping push protection
			// on this synthetic fixture).
			in:   "xoxb-1234567890-0987654321-" + "AbCdEfGhIjKlMnOpQrSt",
			want: "[REDACTED:slack_token]",
		},
		{
			name: "stripe live secret key",
			in:   "sk_live_AbCdEfGh1234IjKlMnOp",
			want: "[REDACTED:stripe_key]",
		},
		{
			name: "aws access key",
			in:   "AKIAIOSFODNN7EXAMPLE",
			want: "[REDACTED:aws_access_key]",
		},
		{
			name: "aws session key",
			in:   "ASIAIOSFODNN7EXAMPLE",
			want: "[REDACTED:aws_session_key]",
		},
		{
			name: "jwt",
			in:   "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dOzo3rs2QbG9tJgPJas4vCUFuUxc0lZY",
			want: "[REDACTED:jwt]",
		},
		{
			name: "pem private key block spanning lines",
			in:   "before\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----\nafter",
			want: "before\n[REDACTED:pem_block]\nafter",
		},
		{
			// The pem_block pattern ([A-Z ]*PRIVATE KEY) also covers OPENSSH
			// blocks and runs first, so the dedicated ssh_private_key rule is
			// shadowed. This asserts the actual label emitted today.
			name: "openssh private key caught by pem rule",
			in:   "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjE\n-----END OPENSSH PRIVATE KEY-----",
			want: "[REDACTED:pem_block]",
		},
		{
			name: "url password preserves scheme user and host",
			in:   "postgres://relay:hunter2secret@db.internal:5432/app",
			want: "postgres://relay:[REDACTED:password]@db.internal:5432/app",
		},
		{
			name: "authorization bearer header",
			in:   "Authorization: Bearer AbCdEf123456GhIjKl789012",
			want: "[REDACTED:auth_bearer]",
		},
		{
			name: "env secret pair with equals",
			in:   "API_KEY=AbCd1234EfGh5678",
			want: "[REDACTED:env_secret_pair]",
		},
		{
			name: "env secret pair with colon and quotes",
			in:   `password: "SuperSecret12345"`,
			want: "[REDACTED:env_secret_pair]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewDefault().Scrub(tc.in); got != tc.want {
				t.Errorf("Scrub(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestScrubNearMisses bounds false positives: none of these look-alikes may
// be altered by any default rule.
func TestScrubNearMisses(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"sk- prefix too short for any key rule", "sk-tooShort123"},
		{"gemini prefix too short", "AIzaShort99"},
		{"github pat too short", "ghp_tooShort123"},
		{"slack wrong token class letter", "xoxq-1234567890-0987654321-AbCdEfGhIjKlMnOpQrSt"},
		{"stripe key too short", "sk_live_short"},
		{"aws key lowercase body", "AKIAiosfodnn7example"},
		{"jwt middle segment too short", "eyJhbGciOiJIUzI1NiJ9.short.sig"},
		{"public key block is not a private key", "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQ\n-----END PUBLIC KEY-----"},
		{"url with port but no password", "https://db.internal:5432/app"},
		{"scp-style git remote has no scheme", "git@github.com:dbisina/relay.git"},
		{"bearer token too short", "Authorization: Bearer short"},
		{"env value too short", "token=abc123"},
		{"name embedding token needs separator", "tokenizer=abcdefghijkl12"},
		{
			// KNOWN GAP: \b does not fire between `_` and the secret word, so
			// prefixed names like FOO_SECRET or DB_PASSWORD are not redacted.
			// If the rule gains prefix support, move this to the match table.
			"prefixed env secret name is not matched today",
			"FOO_SECRET=AbCd1234EfGh5678",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewDefault().Scrub(tc.in); got != tc.in {
				t.Errorf("Scrub(%q) = %q, want input unchanged", tc.in, got)
			}
		})
	}
}

func TestScrubEmptyInput(t *testing.T) {
	if got := NewDefault().Scrub(""); got != "" {
		t.Errorf("Scrub(\"\") = %q, want empty", got)
	}
}

func TestStatsSummaryAndReset(t *testing.T) {
	r := NewDefault()
	r.Scrub("sk-ant-abcdefghijklmnopqrstuvwxyz")
	r.Scrub("sk-ant-abcdefghijklmnopqrstuvwxyz")
	if sum := r.Summary(); !strings.Contains(sum, "anthropic_key:2") {
		t.Errorf("Summary() = %q, want it to contain anthropic_key:2", sum)
	}
	r.Reset()
	if sum := r.Summary(); sum != "" {
		t.Errorf("Summary() after Reset = %q, want empty", sum)
	}
}
