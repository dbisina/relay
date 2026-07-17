// internal/redact/redact.go
//
// Secret scrubber. Pattern-matches common credential shapes and replaces
// them with [REDACTED:<kind>] tokens before content crosses provider
// boundaries (prompts → providers, provider stdout → audit log + UI).
//
// Defense in depth: belt + braces. Patterns are intentionally aggressive;
// false-positive redactions are a feature, not a bug.

package redact

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
)

// Rule — a single pattern + label.
type Rule struct {
	Kind    string
	Pattern *regexp.Regexp
}

// DefaultRules — high-precision patterns for common secrets.
// Order matters: more-specific patterns must run before broader ones to avoid
// e.g. `sk-ant-...` being eaten by the generic `sk-` rule.
var DefaultRules = []Rule{
	// API keys — anthropic before openai because sk-ant- is a sk- prefix
	{"anthropic_key", regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]{20,}`)},
	{"openai_key", regexp.MustCompile(`sk-(?:proj-)?[a-zA-Z0-9_-]{20,}`)},
	{"gemini_key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{30,}`)},
	{"github_pat", regexp.MustCompile(`(?:gh[pousr]_[0-9A-Za-z]{36,}|github_pat_[0-9A-Za-z_]{50,})`)},
	{"slack_token", regexp.MustCompile(`xox[abprs]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]{20,}`)},
	{"stripe_key", regexp.MustCompile(`(?:sk|pk|rk)_(?:live|test)_[0-9A-Za-z]{20,}`)},

	// AWS
	{"aws_access_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"aws_session_key", regexp.MustCompile(`ASIA[0-9A-Z]{16}`)},

	// Generic JWT
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},

	// Private keys
	{"pem_block", regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)},
	{"ssh_private_key", regexp.MustCompile(`(?s)-----BEGIN OPENSSH PRIVATE KEY-----.*?-----END OPENSSH PRIVATE KEY-----`)},

	// Connection strings with embedded passwords
	{"url_password", regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:/?#\s]+:)[^@/\s]+(@)`)},

	// Bearer headers
	{"auth_bearer", regexp.MustCompile(`(?i)\bAuthorization\s*:\s*Bearer\s+[A-Za-z0-9._\-/+=]{16,}`)},

	// .env-style assignments with sensitive names. The name may carry a prefix
	// (FOO_SECRET, DB_PASSWORD, AWS_SECRET_ACCESS_KEY): \b alone never fires
	// between `_` and the keyword, so anchor at the start of the whole
	// identifier and require the sensitive keyword at its end.
	{"env_secret_pair", regexp.MustCompile(`(?i)\b[A-Za-z0-9_]*(?:api[_-]?key|secret|password|passwd|token|access[_-]?key)\s*[:=]\s*['"]?[A-Za-z0-9._\-+/=]{12,}['"]?`)},
}

// Stats — atomically updated counters of redactions.
type Stats struct {
	ByKind map[string]*uint64
}

// Redactor — applies a rule set, tracking counts.
type Redactor struct {
	Rules []Rule
	Stats Stats
}

func New(rules []Rule) *Redactor {
	r := &Redactor{Rules: rules, Stats: Stats{ByKind: map[string]*uint64{}}}
	for _, rl := range rules {
		var c uint64
		r.Stats.ByKind[rl.Kind] = &c
	}
	return r
}

// NewDefault returns a Redactor with all DefaultRules.
func NewDefault() *Redactor { return New(DefaultRules) }

// Scrub returns input with all matches replaced.
// Special-cases url_password to preserve the URL structure.
func (r *Redactor) Scrub(s string) string {
	if s == "" {
		return s
	}
	out := s
	for _, rule := range r.Rules {
		out = rule.Pattern.ReplaceAllStringFunc(out, func(match string) string {
			if c, ok := r.Stats.ByKind[rule.Kind]; ok {
				atomic.AddUint64(c, 1)
			}
			switch rule.Kind {
			case "url_password":
				// Restore $1 + REDACTED + $2 (scheme://user:[REDACTED]@)
				m := rule.Pattern.FindStringSubmatch(match)
				if len(m) >= 3 {
					return m[1] + "[REDACTED:password]" + m[2]
				}
			}
			return fmt.Sprintf("[REDACTED:%s]", rule.Kind)
		})
	}
	return out
}

// Summary — human-readable counts.
func (r *Redactor) Summary() string {
	var parts []string
	for kind, c := range r.Stats.ByKind {
		n := atomic.LoadUint64(c)
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", kind, n))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// Reset — zero all counters (e.g. between sessions).
func (r *Redactor) Reset() {
	for _, c := range r.Stats.ByKind {
		atomic.StoreUint64(c, 0)
	}
}
