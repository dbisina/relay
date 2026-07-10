// internal/config/config.go
//
// Relay configuration loader.
// Config file: .relay/relay.toml (TOML; minimal inline parser for v0).
// relay init writes the default config.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dbisina/relay/internal/retry"
)

// ProviderConfig holds per-provider settings.
type ProviderConfig struct {
	Enabled     bool
	DeclaredCap int64
	Model       string    // for Ollama
	BaseURL     string    // for Ollama override
	Accounts    []Account // optional: multiple logins for the same provider
}

// Account is one login for a provider (e.g. a personal vs work Claude account,
// or a desktop vs terminal config dir). Switching the active account before a
// cross-provider handoff lets Relay rotate quota across subscriptions, which is
// the whole point: people who run several plans instead of buying API credits.
type Account struct {
	Label     string   // stable id: "personal", "work", "desktop"
	ConfigDir string   // provider config/cred dir; mapped to the provider's env var
	Env       []string // extra KEY=VAL overrides applied when this account is active
	Active    bool     // default-active in relay.toml; runtime override wins (see accounts.go)
}

// ProfileConfig defines a task-routing profile.
type ProfileConfig struct {
	Chain       []string // ordered list of provider names
	Kinds       []string // task kinds this profile handles (refactor, test, ...)
	Skills      []string // domain skills (go, react, sql, ...)
	ContextHint string   // free-form hint about when to use this profile
}

// VisionConfig controls the screenshot-based fallback for IDE/extension providers.
type VisionConfig struct {
	Enabled     bool
	Provider    string // "ollama" | "gemini" | "openai" | "anthropic"
	Model       string // e.g. "qwen2.5-vl:7b"
	APIKeyEnv   string // env var name for cloud providers
	BaseURL     string // override (ollama url, etc.)
	PollMs      int    // milliseconds between polls
	WindowMatch string // regex to match target window title
}

// RetryConfig controls the wait-and-retry strategy: whether to wait out a
// provider's reset (or back off through transient overload) before falling
// through to a cross-account / cross-provider handoff.
type RetryConfig struct {
	Enabled        bool
	Prefer         string // "wait" | "handoff" | "wait-then-handoff"
	MaxWaitMinutes int
	BackoffSeconds int
	JitterPct      int
	MaxRetries     int
	MarginSeconds  int
	RetryMessage   string
}

// ToEngine converts the TOML-facing config into the retry engine's config.
func (r RetryConfig) ToEngine() retry.Config {
	return retry.Config{
		Enabled:      r.Enabled,
		Prefer:       r.Prefer,
		MaxWait:      time.Duration(r.MaxWaitMinutes) * time.Minute,
		BackoffBase:  time.Duration(r.BackoffSeconds) * time.Second,
		JitterPct:    r.JitterPct,
		MaxRetries:   r.MaxRetries,
		Margin:       time.Duration(r.MarginSeconds) * time.Second,
		RetryMessage: r.RetryMessage,
	}
}

// Config is the parsed relay.toml.
type Config struct {
	StateDir   string // default: .relay/
	DBPath     string // default: .relay/graph.db
	AuditPath  string // default: .relay/audit.jsonl
	ServerPort int    // default: 4748
	Providers  map[string]*ProviderConfig
	Profiles   map[string]*ProfileConfig
	Vision     VisionConfig
	Retry      RetryConfig
}

// Default returns the default config for a given workDir.
func Default(workDir string) *Config {
	stateDir := filepath.Join(workDir, ".relay")
	return &Config{
		StateDir:   stateDir,
		DBPath:     filepath.Join(stateDir, "graph.db"),
		AuditPath:  filepath.Join(stateDir, "audit.jsonl"),
		ServerPort: 4748,
		Providers: map[string]*ProviderConfig{
			"claude":      {Enabled: true, DeclaredCap: 40_000},
			"codex":       {Enabled: true, DeclaredCap: 500},
			"antigravity": {Enabled: true, DeclaredCap: 1000},
			"opencode":    {Enabled: true, DeclaredCap: 500},
			"ollama":      {Enabled: true, DeclaredCap: 0, Model: "qwen2.5-coder:32b", BaseURL: "http://localhost:11434"},
			"copilot":     {Enabled: true, DeclaredCap: 300},
			"continue":    {Enabled: true, DeclaredCap: 500},
			"cline":       {Enabled: true, DeclaredCap: 500},
		},
		Profiles: map[string]*ProfileConfig{
			"backend":  {Chain: []string{"claude", "codex"}, Kinds: []string{"api", "service", "refactor"}, Skills: []string{"go", "ts"}, ContextHint: "backend services"},
			"frontend": {Chain: []string{"claude", "codex"}, Kinds: []string{"component", "style", "a11y"}, Skills: []string{"react", "css"}, ContextHint: "user interface"},
			"database": {Chain: []string{"codex", "claude"}, Kinds: []string{"migration", "query"}, Skills: []string{"sql"}, ContextHint: "schema and queries"},
			"reviewer": {Chain: []string{"claude"}, Kinds: []string{"review"}, Skills: []string{"any"}, ContextHint: "code review only"},
		},
		Vision: VisionConfig{
			Enabled:     false,
			Provider:    "ollama",
			Model:       "qwen2.5-vl:7b",
			BaseURL:     "http://localhost:11434",
			PollMs:      2500,
			WindowMatch: "(?i)continue|cline|antigravity|copilot",
		},
		Retry: RetryConfig{
			Enabled:        true,
			Prefer:         "wait-then-handoff",
			MaxWaitMinutes: 360,
			BackoffSeconds: 5,
			JitterPct:      20,
			MaxRetries:     8,
			MarginSeconds:  30,
			RetryMessage:   "continue",
		},
	}
}

// Load parses .relay/relay.toml. Falls back to defaults if file absent.
func Load(workDir string) (*Config, error) {
	cfg := Default(workDir)
	tomlPath := filepath.Join(cfg.StateDir, "relay.toml")

	data, err := os.ReadFile(tomlPath)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", tomlPath, err)
	}

	return parseTOML(cfg, string(data))
}

// WriteDefault writes the default relay.toml to stateDir.
func WriteDefault(workDir string) error {
	cfg := Default(workDir)
	if err := os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg.StateDir, "relay.toml"), []byte(defaultTOML), 0600)
}

const defaultTOML = `# Relay configuration — relay.toml
# Generated by relay init

[relay]
server_port = 4748

[providers.claude]
enabled     = true
declared_cap = 40000   # tokens; calibrated from first proxy header

# ── Multiple accounts per provider (optional) ───────────────────────
# Define a login per subscription/config-dir, then switch the active one
# before a handoff to rotate quota. config_dir maps to the provider's env
# var (claude → CLAUDE_CONFIG_DIR, codex → CODEX_HOME). Use env for anything
# else. The active account is tracked in .relay/active-accounts.json.
#
# [providers.claude.accounts.personal]
# active     = true
# config_dir = "C:/Users/you/.claude"
#
# [providers.claude.accounts.work]
# config_dir = "C:/Users/you/.claude-work"
# env        = ["ANTHROPIC_DEFAULT_HEADERS=x-account:work"]

[providers.codex]
enabled     = true
declared_cap = 500     # requests

[providers.antigravity]
enabled     = true
declared_cap = 1000    # requests/day (agy quota)

[providers.opencode]
enabled     = true
declared_cap = 500

[providers.ollama]
enabled     = true
model       = "qwen2.5-coder:32b"
base_url    = "http://localhost:11434"

[providers.copilot]
enabled     = true
declared_cap = 300

[providers.continue]
enabled     = true
declared_cap = 500

[providers.cline]
enabled     = true
declared_cap = 500

# ─── Profiles ───────────────────────────────────────────────────────
# Task-routing profiles. Chain order = handoff priority.
[profiles.backend]
chain        = ["claude", "codex"]
kinds        = ["api", "service", "refactor"]
skills       = ["go", "ts"]
context_hint = "backend services"

[profiles.frontend]
chain        = ["claude", "codex"]
kinds        = ["component", "style", "a11y"]
skills       = ["react", "css"]
context_hint = "user interface"

[profiles.database]
chain        = ["codex", "claude"]
kinds        = ["migration", "query"]
skills       = ["sql"]
context_hint = "schema and queries"

[profiles.reviewer]
chain        = ["claude"]
kinds        = ["review"]
skills       = ["any"]
context_hint = "code review only"

# ─── Vision fallback ────────────────────────────────────────────────
# Screenshot-based observation for IDE/extension providers we can't hook.
[vision]
enabled       = false
provider      = "ollama"          # ollama | gemini | openai | anthropic
model         = "qwen2.5-vl:7b"
api_key_env   = ""                # GEMINI_API_KEY / OPENAI_API_KEY / ANTHROPIC_API_KEY
base_url      = "http://localhost:11434"
poll_ms       = 2500
window_match  = "(?i)continue|cline|antigravity|copilot"

# ─── Wait-and-retry ─────────────────────────────────────────────────
# When a provider hits a usage limit or transient overload, prefer waiting
# for the same subscription to reset over an immediate handoff. Works across
# every agent Relay drives.
[retry]
enabled          = true
prefer           = "wait-then-handoff"  # wait | handoff | wait-then-handoff
max_wait_minutes = 360                  # don't wait longer than this; hand off instead
backoff_seconds  = 5                    # base for exponential backoff on overload
jitter_pct       = 20
max_retries      = 8
margin_seconds   = 30                   # grace period added after a printed reset time
retry_message    = "continue"
`

// parseTOML — minimal inline TOML parser. Sections: [relay], [providers.X],
// [profiles.X], [vision], [retry]. Arrays parsed as ["a", "b"].
func parseTOML(cfg *Config, input string) (*Config, error) {
	section := ""
	providerName := ""
	profileName := ""
	accProvider := ""
	accLabel := ""

	for _, rawLine := range strings.Split(input, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			providerName, profileName, accProvider, accLabel = "", "", "", ""
			switch {
			// providers.<p>.accounts.<label> — must precede the providers. arm.
			case strings.HasPrefix(section, "providers.") && strings.Contains(section, ".accounts."):
				seg := strings.SplitN(strings.TrimPrefix(section, "providers."), ".accounts.", 2)
				if len(seg) == 2 && seg[0] != "" && seg[1] != "" {
					accProvider, accLabel = seg[0], seg[1]
					if cfg.Providers[accProvider] == nil {
						cfg.Providers[accProvider] = &ProviderConfig{}
					}
					accountPtr(cfg.Providers[accProvider], accLabel) // ensure exists
				}
			case strings.HasPrefix(section, "providers."):
				providerName = strings.TrimPrefix(section, "providers.")
				if cfg.Providers[providerName] == nil {
					cfg.Providers[providerName] = &ProviderConfig{}
				}
			case strings.HasPrefix(section, "profiles."):
				profileName = strings.TrimPrefix(section, "profiles.")
				if cfg.Profiles == nil {
					cfg.Profiles = map[string]*ProfileConfig{}
				}
				if cfg.Profiles[profileName] == nil {
					cfg.Profiles[profileName] = &ProfileConfig{}
				}
			}
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if idx := strings.Index(val, " #"); idx >= 0 {
			val = strings.TrimSpace(val[:idx])
		}

		switch {
		case section == "relay":
			if key == "server_port" {
				if p, err := strconv.Atoi(strings.Trim(val, `"`)); err == nil {
					cfg.ServerPort = p
				}
			}

		case section == "retry":
			val = strings.Trim(val, `"`)
			switch key {
			case "enabled":
				cfg.Retry.Enabled = val == "true"
			case "prefer":
				cfg.Retry.Prefer = val
			case "max_wait_minutes":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Retry.MaxWaitMinutes = n
				}
			case "backoff_seconds":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Retry.BackoffSeconds = n
				}
			case "jitter_pct":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Retry.JitterPct = n
				}
			case "max_retries":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Retry.MaxRetries = n
				}
			case "margin_seconds":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Retry.MarginSeconds = n
				}
			case "retry_message":
				cfg.Retry.RetryMessage = val
			}

		case section == "vision":
			val = strings.Trim(val, `"`)
			switch key {
			case "enabled":
				cfg.Vision.Enabled = val == "true"
			case "provider":
				cfg.Vision.Provider = val
			case "model":
				cfg.Vision.Model = val
			case "api_key_env":
				cfg.Vision.APIKeyEnv = val
			case "base_url":
				cfg.Vision.BaseURL = val
			case "poll_ms":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Vision.PollMs = n
				}
			case "window_match":
				cfg.Vision.WindowMatch = val
			}

		case accProvider != "":
			acc := accountPtr(cfg.Providers[accProvider], accLabel)
			v := strings.Trim(val, `"`)
			switch key {
			case "active":
				acc.Active = v == "true"
			case "config_dir":
				acc.ConfigDir = v
			case "env":
				acc.Env = parseTomlStringArray(val)
			}

		case providerName != "":
			pc := cfg.Providers[providerName]
			v := strings.Trim(val, `"`)
			switch key {
			case "enabled":
				pc.Enabled = v == "true"
			case "declared_cap":
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					pc.DeclaredCap = n
				}
			case "model":
				pc.Model = v
			case "base_url":
				pc.BaseURL = v
			}

		case profileName != "":
			pf := cfg.Profiles[profileName]
			switch key {
			case "chain":
				pf.Chain = parseTomlStringArray(val)
			case "kinds":
				pf.Kinds = parseTomlStringArray(val)
			case "skills":
				pf.Skills = parseTomlStringArray(val)
			case "context_hint":
				pf.ContextHint = strings.Trim(val, `"`)
			}
		}
	}
	return cfg, nil
}

// parseTomlStringArray parses ["a", "b", "c"] into []string.
func parseTomlStringArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.Trim(strings.TrimSpace(p), `"`)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
