// providers.go — provider metadata, probe logic, and config write-back.
//
// Each provider has:
//   - Static metadata (display name, description, setup instructions)
//   - A probe function that detects if it's usable on this machine
//   - Config read/write backed by relay.toml

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dbisina/relay/internal/config"
)

// ─── provider metadata ────────────────────────────────────────────────────────

type ProviderMeta struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	SetupHint   string `json:"setupHint"`
	SetupURL    string `json:"setupUrl"`
	Kind        string `json:"kind"` // "cli", "api", "local", "extension"

	// Install: per-OS shell command(s) executed sequentially.
	InstallCmds map[string][]installStep `json:"-"`
	// Auth: per-OS shell command(s) to start OAuth flow (CLI tool opens browser).
	AuthCmds map[string][]installStep `json:"-"`
	// Can be auto-installed?
	CanInstall bool `json:"canInstall"`
	// Supports OAuth browser flow via CLI command.
	CanOAuth bool `json:"canOauth"`
	// Supports API key authentication.
	CanAPIKey bool `json:"canApiKey"`
	// Env var name used by this provider's CLI (e.g. "ANTHROPIC_API_KEY").
	APIKeyEnvVar string `json:"apiKeyEnvVar,omitempty"`
	// Where the user gets an API key.
	APIKeyURL string `json:"apiKeyUrl,omitempty"`
}

type installStep struct {
	cmd  string
	args []string
}

var allProviderMeta = []ProviderMeta{
	{
		Name:         "claude",
		DisplayName:  "Claude (Anthropic)",
		Description:  "Anthropic's Claude coding agent via the claude CLI.",
		SetupHint:    "OAuth: claude login (browser flow)  or  set ANTHROPIC_API_KEY",
		SetupURL:     "https://claude.ai/code",
		Kind:         "cli",
		CanInstall:   true,
		CanOAuth:     true,
		CanAPIKey:    true,
		APIKeyEnvVar: "ANTHROPIC_API_KEY",
		APIKeyURL:    "https://console.anthropic.com/settings/keys",
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "npm.cmd", args: []string{"install", "-g", "@anthropic-ai/claude-code"}}},
			"darwin":  {{cmd: "npm", args: []string{"install", "-g", "@anthropic-ai/claude-code"}}},
			"linux":   {{cmd: "npm", args: []string{"install", "-g", "@anthropic-ai/claude-code"}}},
		},
		AuthCmds: map[string][]installStep{
			"windows": {{cmd: "claude.cmd", args: []string{"login"}}},
			"darwin":  {{cmd: "claude", args: []string{"login"}}},
			"linux":   {{cmd: "claude", args: []string{"login"}}},
		},
	},
	{
		Name:         "codex",
		DisplayName:  "Codex (OpenAI)",
		Description:  "OpenAI's Codex CLI agent.",
		SetupHint:    "Set OPENAI_API_KEY — get one at platform.openai.com",
		SetupURL:     "https://github.com/openai/codex",
		Kind:         "cli",
		CanInstall:   true,
		CanOAuth:     false,
		CanAPIKey:    true,
		APIKeyEnvVar: "OPENAI_API_KEY",
		APIKeyURL:    "https://platform.openai.com/api-keys",
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "npm.cmd", args: []string{"install", "-g", "@openai/codex"}}},
			"darwin":  {{cmd: "npm", args: []string{"install", "-g", "@openai/codex"}}},
			"linux":   {{cmd: "npm", args: []string{"install", "-g", "@openai/codex"}}},
		},
	},
	{
		Name:        "antigravity",
		DisplayName: "Antigravity",
		Description: "Google Antigravity coding agent CLI (agy).",
		SetupHint:   "Install: irm https://antigravity.google/cli/install.ps1 | iex  (Windows)",
		SetupURL:    "https://antigravity.google/download",
		Kind:        "cli",
		CanInstall:  true,
		CanOAuth:    true,
		CanAPIKey:   false,
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "powershell.exe", args: []string{"-NoProfile", "-Command",
				"iwr -useb https://antigravity.google/cli/install.ps1 | iex"}}},
			"darwin": {{cmd: "sh", args: []string{"-c", "curl -fsSL https://antigravity.google/cli/install.sh | sh"}}},
			"linux":  {{cmd: "sh", args: []string{"-c", "curl -fsSL https://antigravity.google/cli/install.sh | sh"}}},
		},
		AuthCmds: map[string][]installStep{
			"windows": {{cmd: "agy.exe", args: []string{"login"}}},
			"darwin":  {{cmd: "agy", args: []string{"login"}}},
			"linux":   {{cmd: "agy", args: []string{"login"}}},
		},
	},
	{
		Name:         "opencode",
		DisplayName:  "OpenCode",
		Description:  "Open-source terminal AI coding assistant.",
		SetupHint:    "Set OPENCODE_API_KEY or configure provider in opencode config",
		SetupURL:     "https://opencode.ai",
		Kind:         "cli",
		CanInstall:   true,
		CanOAuth:     false,
		CanAPIKey:    true,
		APIKeyEnvVar: "OPENCODE_API_KEY",
		APIKeyURL:    "https://opencode.ai/docs",
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "npm.cmd", args: []string{"install", "-g", "opencode-ai"}}},
			"darwin":  {{cmd: "brew", args: []string{"install", "opencode-ai/tap/opencode"}}},
			"linux":   {{cmd: "npm", args: []string{"install", "-g", "opencode-ai"}}},
		},
	},
	{
		Name:        "ollama",
		DisplayName: "Ollama (local)",
		Description: "Run LLMs locally — no API key needed.",
		SetupHint:   "Install Ollama, then it serves on localhost:11434",
		SetupURL:    "https://ollama.ai",
		Kind:        "local",
		CanInstall:  true,
		CanOAuth:    false,
		CanAPIKey:   false,
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "winget", args: []string{"install", "--id", "Ollama.Ollama", "--accept-source-agreements", "--accept-package-agreements"}}},
			"darwin":  {{cmd: "brew", args: []string{"install", "ollama"}}},
			"linux":   {{cmd: "sh", args: []string{"-c", "curl -fsSL https://ollama.ai/install.sh | sh"}}},
		},
	},
	{
		Name:        "copilot",
		DisplayName: "GitHub Copilot",
		Description: "GitHub Copilot via the gh CLI extension.",
		SetupHint:   "OAuth: gh auth login (browser flow). Requires Copilot subscription.",
		SetupURL:    "https://github.com/features/copilot",
		Kind:        "cli",
		CanInstall:  true,
		CanOAuth:    true,
		CanAPIKey:   false,
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "gh.exe", args: []string{"extension", "install", "github/gh-copilot"}}},
			"darwin":  {{cmd: "gh", args: []string{"extension", "install", "github/gh-copilot"}}},
			"linux":   {{cmd: "gh", args: []string{"extension", "install", "github/gh-copilot"}}},
		},
		AuthCmds: map[string][]installStep{
			"windows": {{cmd: "gh.exe", args: []string{"auth", "login", "--web"}}},
			"darwin":  {{cmd: "gh", args: []string{"auth", "login", "--web"}}},
			"linux":   {{cmd: "gh", args: []string{"auth", "login", "--web"}}},
		},
	},
	{
		Name:        "continue",
		DisplayName: "Continue.dev",
		Description: "Open-source AI coding assistant — VS Code + JetBrains.",
		SetupHint:   "Install the Continue extension in VS Code",
		SetupURL:    "https://continue.dev",
		Kind:        "extension",
		CanInstall:  true,
		CanOAuth:    false,
		CanAPIKey:   false,
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "code.cmd", args: []string{"--install-extension", "Continue.continue"}}},
			"darwin":  {{cmd: "code", args: []string{"--install-extension", "Continue.continue"}}},
			"linux":   {{cmd: "code", args: []string{"--install-extension", "Continue.continue"}}},
		},
	},
	{
		Name:        "cline",
		DisplayName: "Cline (formerly Claude Dev)",
		Description: "Autonomous AI coding agent — VS Code extension.",
		SetupHint:   "Install the Cline extension in VS Code",
		SetupURL:    "https://github.com/cline/cline",
		Kind:        "extension",
		CanInstall:  true,
		CanOAuth:    false,
		CanAPIKey:   false,
		InstallCmds: map[string][]installStep{
			"windows": {{cmd: "code.cmd", args: []string{"--install-extension", "saoudrizwan.claude-dev"}}},
			"darwin":  {{cmd: "code", args: []string{"--install-extension", "saoudrizwan.claude-dev"}}},
			"linux":   {{cmd: "code", args: []string{"--install-extension", "saoudrizwan.claude-dev"}}},
		},
	},
}

// ─── probe result ─────────────────────────────────────────────────────────────

// ProbeStatus values.
const (
	ProbeAvailable   = "available"   // detected and ready
	ProbeNoKey       = "no_key"      // tool found but needs auth/key
	ProbeNotFound    = "not_found"   // binary/extension not installed
	ProbeUnavailable = "unavailable" // installed but not reachable
	ProbeManual      = "manual"      // can't auto-detect (extensions)
)

// ApiProviderDetail is served by GET /api/config.
type ApiProviderDetail struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	SetupHint   string `json:"setupHint"`
	SetupURL    string `json:"setupUrl"`
	Kind        string `json:"kind"`

	// From relay.toml
	Enabled     bool   `json:"enabled"`
	DeclaredCap int64  `json:"declaredCap"`
	Model       string `json:"model,omitempty"`
	BaseURL     string `json:"baseUrl,omitempty"`

	// Probe result
	ProbeStatus string `json:"probeStatus"` // available / no_key / not_found / unavailable / manual / installing / authenticating
	ProbeNote   string `json:"probeNote,omitempty"`

	// Install/auth capability flags
	CanInstall   bool   `json:"canInstall"`
	CanOAuth     bool   `json:"canOauth"`
	CanAPIKey    bool   `json:"canApiKey"`
	APIKeyEnvVar string `json:"apiKeyEnvVar,omitempty"`
	APIKeyURL    string `json:"apiKeyUrl,omitempty"`
	APIKeySet    bool   `json:"apiKeySet"` // true if env var or .relay/.env has a value

	// Ollama-launch fallback
	CanLaunchOllama        bool     `json:"canLaunchOllama"`
	OllamaModelSuggestions []string `json:"ollamaModelSuggestions,omitempty"`

	// Model picker support
	AvailableModels []string `json:"availableModels,omitempty"`

	// Account-aware handoff (pillar 3)
	Accounts      []ApiAccount `json:"accounts,omitempty"`
	ActiveAccount string       `json:"activeAccount,omitempty"`
}

// ApiAccount is one selectable login for a provider, surfaced to the UI.
type ApiAccount struct {
	Label     string `json:"label"`
	Active    bool   `json:"active"`
	ConfigDir string `json:"configDir,omitempty"`
}

// ─── probe functions ──────────────────────────────────────────────────────────

func probeAll(cfg map[string]providerCfgBrief) []ApiProviderDetail {
	results := make([]ApiProviderDetail, 0, len(allProviderMeta))
	for _, m := range allProviderMeta {
		pc := cfg[m.Name]
		status, note := probeProvider(m.Name, pc.baseURL)
		// Override status when an install/auth is in flight
		if s, ok := installState.Load(m.Name); ok {
			ss := s.(string)
			status = ss
			note = "running…"
		}
		// Check if API key is set (env var or .relay/.env)
		apiKeySet := false
		if m.APIKeyEnvVar != "" {
			if os.Getenv(m.APIKeyEnvVar) != "" {
				apiKeySet = true
			} else if loadEnvFileValue(m.APIKeyEnvVar) != "" {
				apiKeySet = true
			}
		}
		var ollamaSuggest []string
		if spec, ok := OllamaLaunchSpecs[m.Name]; ok {
			ollamaSuggest = spec.Recommended
		}
		results = append(results, ApiProviderDetail{
			Name:                   m.Name,
			DisplayName:            m.DisplayName,
			Description:            m.Description,
			SetupHint:              m.SetupHint,
			SetupURL:               m.SetupURL,
			Kind:                   m.Kind,
			Enabled:                pc.enabled,
			DeclaredCap:            pc.cap,
			Model:                  pc.model,
			BaseURL:                pc.baseURL,
			ProbeStatus:            status,
			ProbeNote:              note,
			CanInstall:             m.CanInstall,
			CanOAuth:               m.CanOAuth,
			CanAPIKey:              m.CanAPIKey,
			APIKeyEnvVar:           m.APIKeyEnvVar,
			APIKeyURL:              m.APIKeyURL,
			APIKeySet:              apiKeySet,
			CanLaunchOllama:        CanLaunchViaOllama(m.Name) && commandExists("ollama"),
			OllamaModelSuggestions: ollamaSuggest,
			AvailableModels:        CuratedModels[m.Name],
		})
	}
	return results
}

type providerCfgBrief struct {
	enabled bool
	cap     int64
	model   string
	baseURL string
}

func probeProvider(name, baseURL string) (status, note string) {
	switch name {
	case "claude":
		if commandExists("claude") {
			return ProbeAvailable, "claude CLI found"
		}
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			return ProbeAvailable, "ANTHROPIC_API_KEY set"
		}
		return ProbeNotFound, "claude CLI not found · ANTHROPIC_API_KEY not set"

	case "codex":
		if commandExists("codex") {
			return ProbeAvailable, "codex CLI found"
		}
		if os.Getenv("OPENAI_API_KEY") != "" {
			return ProbeNoKey, "OPENAI_API_KEY set but codex CLI missing"
		}
		return ProbeNotFound, "codex CLI not found · OPENAI_API_KEY not set"

	case "antigravity":
		// agy CLI is what the adapter spawns. IDE folder is informational.
		if commandExists("agy") {
			return ProbeAvailable, "agy CLI found"
		}
		if antigravityInstalled() {
			return ProbeManual, "IDE detected, install CLI: irm https://antigravity.google/cli/install.ps1 | iex"
		}
		return ProbeNotFound, "agy CLI not installed"

	case "opencode":
		if commandExists("opencode") {
			return ProbeAvailable, "opencode found"
		}
		return ProbeNotFound, "opencode not found in PATH"

	case "ollama":
		url := baseURL
		if url == "" {
			url = "http://localhost:11434"
		}
		if httpProbe(url + "/api/tags") {
			return ProbeAvailable, "Ollama running at " + url
		}
		if commandExists("ollama") {
			return ProbeUnavailable, "ollama installed but not running · run: ollama serve"
		}
		return ProbeNotFound, "ollama not installed"

	case "copilot":
		if !commandExists("gh") {
			return ProbeNotFound, "gh CLI not found · install GitHub CLI first"
		}
		out, err := exec.Command("gh", "extension", "list").Output()
		if err == nil && strings.Contains(string(out), "copilot") {
			return ProbeAvailable, "gh copilot extension found"
		}
		return ProbeNoKey, "gh found but copilot extension not installed · run: gh extension install github/gh-copilot"

	case "continue", "cline":
		// Can't reliably detect VS Code extensions without scanning extension dirs
		vscodePath := vsCodeExtensionsPath()
		if vscodePath != "" {
			entries, _ := os.ReadDir(vscodePath)
			prefix := name
			if name == "cline" {
				prefix = "saoudrizwan.claude-dev"
			} else {
				prefix = "continue.continue"
			}
			for _, e := range entries {
				if strings.HasPrefix(strings.ToLower(e.Name()), strings.ToLower(prefix[:4])) {
					return ProbeAvailable, "extension found in VS Code"
				}
			}
		}
		return ProbeManual, "install the " + name + " extension in VS Code, then enable it here"
	}

	return ProbeManual, ""
}

// ─── curated model lists ──────────────────────────────────────────────────────

// CuratedModels — recommended models per provider, ordered best-to-cheapest.
// Used by the desktop app model picker. For ollama the live list from
// /api/ollama/models supersedes this.
var CuratedModels = map[string][]string{
	"claude": {
		"claude-sonnet-4-5",
		"claude-opus-4-1",
		"claude-haiku-4",
		"claude-sonnet-4-0",
		"claude-opus-4-0",
	},
	"codex": {
		"gpt-5",
		"gpt-5-codex",
		"gpt-5-mini",
		"gpt-4o",
		"gpt-4o-mini",
	},
	"antigravity": {
		"gemini-3-pro-preview",
		"gemini-2.5-flash",
		"gemini-2.5-pro",
	},
	"opencode": {
		"anthropic/claude-sonnet-4-5",
		"openai/gpt-5",
		"openrouter/qwen-3-coder",
		"groq/llama-3.3-70b-versatile",
	},
	"ollama": {
		"qwen2.5-coder:32b",
		"qwen2.5-coder:14b",
		"qwen2.5-coder:7b",
		"deepseek-coder-v2:16b",
		"llama3.3:70b",
		"gemma3:1b",
	},
	"copilot":  {}, // copilot uses gh extension default
	"continue": {}, // continue picks via extension config
	"cline":    {}, // cline picks via extension config
}

// ─── install / auth runner ────────────────────────────────────────────────────

// installState tracks in-flight ops: name → "installing" | "authenticating"
var installState = newSyncMap()

type syncMap struct {
	inner map[string]string
	mu    sync.Mutex
}

func newSyncMap() *syncMap { return &syncMap{inner: map[string]string{}} }

func (m *syncMap) Load(k string) (interface{}, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.inner[k]
	if !ok {
		return nil, false
	}
	return v, true
}
func (m *syncMap) Store(k, v string) { m.mu.Lock(); m.inner[k] = v; m.mu.Unlock() }
func (m *syncMap) Delete(k string)   { m.mu.Lock(); delete(m.inner, k); m.mu.Unlock() }

// providerByName looks up a provider's metadata.
func providerByName(name string) *ProviderMeta {
	for i := range allProviderMeta {
		if allProviderMeta[i].Name == name {
			return &allProviderMeta[i]
		}
	}
	return nil
}

// InstallRequest is the body for POST /api/providers/install and /auth.
type InstallRequest struct {
	Name string `json:"name"`
}

// runInstall opens a visible terminal window with the install command running.
// User watches output live, sees prompts, can sign in inline.
// Best-effort: terminal closes when command finishes; user sees the result.
func runInstall(name string, emit func(tag, msg string)) error {
	meta := providerByName(name)
	if meta == nil {
		return fmt.Errorf("unknown provider: %s", name)
	}
	if !meta.CanInstall {
		return fmt.Errorf("install not supported for %s", name)
	}
	steps, ok := meta.InstallCmds[runtime.GOOS]
	if !ok || len(steps) == 0 {
		return fmt.Errorf("no install command for %s on %s", name, runtime.GOOS)
	}

	installState.Store(name, "installing")
	defer installState.Delete(name)

	emit("system", fmt.Sprintf("opening terminal to install %s…", name))
	for _, step := range steps {
		emit("system", fmt.Sprintf("$ %s %s", step.cmd, strings.Join(step.args, " ")))
		if err := openInTerminal(step.cmd, step.args, nil, fmt.Sprintf("Relay - install %s", name)); err != nil {
			emit("error", fmt.Sprintf("failed to open terminal: %v", err))
			return err
		}
	}
	emit("system", "  watch the new terminal window for install progress")
	return nil
}

// runOAuth opens a terminal so the user can complete the OAuth flow interactively
// (paste codes, follow CLI prompts). CLI tools like `claude login` open a browser.
func runOAuth(name string, emit func(tag, msg string)) error {
	meta := providerByName(name)
	if meta == nil {
		return fmt.Errorf("unknown provider: %s", name)
	}
	if !meta.CanOAuth {
		return fmt.Errorf("OAuth not supported for %s", name)
	}
	steps, ok := meta.AuthCmds[runtime.GOOS]
	if !ok || len(steps) == 0 {
		return fmt.Errorf("no OAuth command for %s on %s", name, runtime.GOOS)
	}

	installState.Store(name, "authenticating")
	defer installState.Delete(name)

	emit("system", fmt.Sprintf("opening terminal for %s sign-in…", name))
	for _, step := range steps {
		emit("system", fmt.Sprintf("$ %s %s", step.cmd, strings.Join(step.args, " ")))
		if err := openInTerminal(step.cmd, step.args, nil, fmt.Sprintf("Relay - sign in to %s", name)); err != nil {
			emit("error", fmt.Sprintf("failed to open terminal: %v", err))
			return err
		}
	}
	emit("system", "  complete sign-in in the new terminal · browser may also open")
	return nil
}

// resolveCommandForTerminal substitutes a fallback command when the primary
// CLI isn't in PATH yet (common right after `npm i -g` because Windows doesn't
// refresh env vars). Maps:
//
//	claude → npx --yes @anthropic-ai/claude-code
//	codex  → npx --yes @openai/codex
//	agy    → npx --yes antigravity-cli
//
// Returns (cmd, args) — possibly rewritten.
func resolveCommandForTerminal(cmd string, args []string) (string, []string) {
	if commandExists(cmd) {
		return cmd, args
	}
	npxFallback := func(pkg string) (string, []string) {
		npx := "npx"
		if runtime.GOOS == "windows" {
			npx = "npx.cmd"
		}
		return npx, append([]string{"--yes", pkg, "--"}, args...)
	}
	switch strings.TrimSuffix(strings.ToLower(cmd), ".cmd") {
	case "claude":
		return npxFallback("@anthropic-ai/claude-code")
	case "codex":
		return npxFallback("@openai/codex")
	case "agy":
		return npxFallback("antigravity-cli")
	}
	return cmd, args
}

// openInTerminal launches a new terminal window running the given command.
// The window stays open after the command exits so the user can see output.
// Platform-specific:
//
//	Windows: Windows Terminal (wt.exe) if present, else cmd.exe with /k
//	macOS:   osascript spawning Terminal.app
//	Linux:   first available of x-terminal-emulator, gnome-terminal, konsole, xterm
func openInTerminal(cmd string, args []string, env []string, title string) error {
	cmd, args = resolveCommandForTerminal(cmd, args)
	cmdLine := cmd
	for _, a := range args {
		// Quote args containing spaces
		if strings.ContainsAny(a, " \t\"") {
			cmdLine += " \"" + strings.ReplaceAll(a, `"`, `\"`) + "\""
		} else {
			cmdLine += " " + a
		}
	}

	// Optional env overrides (e.g. CLAUDE_CONFIG_DIR for a specific account).
	var winEnv, shEnv string
	for _, kv := range env {
		winEnv += "set \"" + kv + "\" && "
		shEnv += kv + " "
	}

	switch runtime.GOOS {
	case "windows":
		// Strategy: try Windows Terminal directly, then fall back to cmd.
		// Hold-open wrapper: append `& pause` so the window stays open after
		// the command exits (success or failure), letting user see output.
		holdCmd := winEnv + cmdLine + " & echo. & pause"

		if wt, err := exec.LookPath("wt.exe"); err == nil {
			// Spawn wt directly (no `start` wrapper) — more reliable.
			c := exec.Command(wt,
				"--title", title,
				"cmd.exe", "/k", holdCmd,
			)
			if err := c.Start(); err == nil {
				return nil
			}
			// If wt fails for any reason, fall through to plain cmd.
		}
		// Plain cmd window: spawn cmd.exe with /k, give it a title via `title` builtin.
		// We use `start` so the new window is detached from the daemon.
		c := exec.Command("cmd.exe", "/c",
			"start", title, "cmd.exe", "/k", holdCmd,
		)
		return c.Start()

	case "darwin":
		// osascript: tell Terminal to do script
		script := fmt.Sprintf(
			`tell application "Terminal" to do script %q
activate application "Terminal"`,
			shEnv+cmdLine,
		)
		c := exec.Command("osascript", "-e", script)
		return c.Start()

	default: // linux + unix
		// Use `bash -c` keeping window open via `; exec bash`
		holdCmd := shEnv + cmdLine + "; echo; read -p '[press Enter to close]' x"
		for _, term := range []string{
			"x-terminal-emulator", "gnome-terminal", "konsole", "xfce4-terminal",
			"alacritty", "kitty", "wezterm", "xterm",
		} {
			if _, err := exec.LookPath(term); err == nil {
				var c *exec.Cmd
				switch term {
				case "gnome-terminal":
					c = exec.Command(term, "--", "bash", "-c", holdCmd)
				case "konsole":
					c = exec.Command(term, "-e", "bash", "-c", holdCmd)
				default:
					c = exec.Command(term, "-e", "bash", "-c", holdCmd)
				}
				return c.Start()
			}
		}
		return fmt.Errorf("no terminal emulator found")
	}
}

// setAPIKey writes a provider's API key to .relay/.env (gitignored).
// Returns true if .env contents changed.
func setAPIKey(workDir, name, value string) error {
	meta := providerByName(name)
	if meta == nil {
		return fmt.Errorf("unknown provider: %s", name)
	}
	if !meta.CanAPIKey {
		return fmt.Errorf("%s does not support API key auth", name)
	}
	if meta.APIKeyEnvVar == "" {
		return fmt.Errorf("no API key env var defined for %s", name)
	}
	envPath := filepath.Join(workDir, ".relay", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0700); err != nil {
		return err
	}
	if err := upsertEnvVar(envPath, meta.APIKeyEnvVar, value); err != nil {
		return err
	}
	// Append .relay/.env to gitignore if missing
	addToGitignore(workDir, ".relay/.env")
	return nil
}

// augmentPATH prepends commonly-missed install dirs onto the daemon's PATH.
// Without this, agents installed to user-local locations (e.g. agy at
// %LOCALAPPDATA%\agy\bin\agy.exe, claude at ~/.local/bin/claude) can be
// detected by commandExists() but fail when exec.Command tries to spawn them.
func augmentPATH() {
	pathSep := string(os.PathListSeparator)
	current := os.Getenv("PATH")
	add := []string{}
	for _, d := range extraSearchDirs() {
		// Only add directories that actually exist
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			if !strings.Contains(strings.ToLower(current), strings.ToLower(d)) {
				add = append(add, d)
			}
		}
	}
	if len(add) > 0 {
		os.Setenv("PATH", strings.Join(add, pathSep)+pathSep+current) //nolint:errcheck
	}
}

// loadEnvFileIntoEnv reads .relay/.env from workDir and sets each KEY=VALUE
// into the daemon's environment so spawned adapter processes inherit them.
// Existing env vars are NOT overridden.
func loadEnvFileIntoEnv(workDir string) {
	envPath := filepath.Join(workDir, ".relay", ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(line[i+1:])
			v = strings.Trim(v, `"`)
			if k != "" && os.Getenv(k) == "" {
				os.Setenv(k, v) //nolint:errcheck
			}
		}
	}
}

// loadEnvFileValue reads a single KEY from .relay/.env in cwd.
// Returns "" if unset, missing file, or read error.
func loadEnvFileValue(key string) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	envPath := filepath.Join(wd, ".relay", ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(line[i+1:])
			v = strings.Trim(v, `"`)
			if k == key {
				return v
			}
		}
	}
	return ""
}

// upsertEnvVar adds or replaces KEY=value in a dotenv file.
// File created with 0600 perms if absent.
func upsertEnvVar(path, key, value string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(string(data), "\n")
	}
	found := false
	newLine := fmt.Sprintf("%s=%q", key, value)
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, key+"=") {
			lines[i] = newLine
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, newLine)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600)
}

func addToGitignore(workDir, entry string) {
	path := filepath.Join(workDir, ".gitignore")
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), entry) {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString("\n" + entry + "\n") //nolint:errcheck
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// commandExists checks PATH plus common install dirs that may not yet be in
// the daemon's PATH (npm global, brew, etc.). Useful right after install
// when the daemon process has a stale environment.
func commandExists(name string) bool {
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	for _, dir := range extraSearchDirs() {
		for _, ext := range searchExts() {
			full := filepath.Join(dir, name+ext)
			if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
				return true
			}
		}
	}
	return false
}

func searchExts() []string {
	if runtime.GOOS == "windows" {
		return []string{".exe", ".cmd", ".bat", ""}
	}
	return []string{""}
}

func extraSearchDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		localApp := os.Getenv("LOCALAPPDATA")
		dirs := []string{}
		if appdata != "" {
			dirs = append(dirs, filepath.Join(appdata, "npm"))
		}
		if localApp != "" {
			dirs = append(dirs,
				filepath.Join(localApp, "Programs", "Ollama"),
				filepath.Join(localApp, "Programs", "Microsoft VS Code", "bin"),
				filepath.Join(localApp, "agy", "bin"), // Antigravity CLI install dir
				filepath.Join(localApp, "Programs", "Antigravity"),
			)
		}
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "AppData", "Roaming", "npm"))
			dirs = append(dirs, filepath.Join(home, ".bun", "bin"))
			dirs = append(dirs, filepath.Join(home, ".local", "bin")) // claude default location
		}
		return dirs
	case "darwin":
		return []string{
			"/usr/local/bin", "/opt/homebrew/bin",
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".npm-global", "bin"),
		}
	default:
		return []string{
			"/usr/local/bin",
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".npm-global", "bin"),
		}
	}
}

func httpProbe(url string) bool {
	client := &http.Client{
		Timeout: 1500 * time.Millisecond,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 800 * time.Millisecond}).DialContext,
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// antigravityInstalled checks common install paths for Google Antigravity IDE.
func antigravityInstalled() bool {
	home, _ := os.UserHomeDir()
	candidates := []string{}
	switch runtime.GOOS {
	case "windows":
		if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
			candidates = append(candidates,
				filepath.Join(localApp, "Programs", "Antigravity"),
				filepath.Join(localApp, "Programs", "Google", "Antigravity"),
			)
		}
		if progFiles := os.Getenv("ProgramFiles"); progFiles != "" {
			candidates = append(candidates, filepath.Join(progFiles, "Antigravity"))
		}
	case "darwin":
		candidates = append(candidates,
			"/Applications/Antigravity.app",
			filepath.Join(home, "Applications", "Antigravity.app"),
		)
	default:
		candidates = append(candidates,
			"/opt/antigravity",
			filepath.Join(home, ".local", "share", "antigravity"),
		)
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

func vsCodeExtensionsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, ".vscode", "extensions")
	case "darwin":
		return filepath.Join(home, ".vscode", "extensions")
	default:
		return filepath.Join(home, ".vscode", "extensions")
	}
}

// ─── config write-back ────────────────────────────────────────────────────────

// UpdateProviderRequest is the body for POST /api/config/provider.
type UpdateProviderRequest struct {
	Name        string `json:"name"`
	Enabled     *bool  `json:"enabled"`
	DeclaredCap *int64 `json:"declaredCap"`
	Model       string `json:"model"`
	BaseURL     string `json:"baseUrl"`
}

// writeProviderConfig updates a single [providers.X] section in relay.toml.
// It reads the existing file, patches the relevant section, and writes it back.
func writeProviderConfig(tomlPath string, req UpdateProviderRequest) error {
	data, err := os.ReadFile(tomlPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	section := ""
	inTarget := false
	updated := map[string]bool{}
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section change
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			// If leaving target section, inject missing keys
			if inTarget {
				if req.Enabled != nil && !updated["enabled"] {
					out = append(out, fmt.Sprintf("enabled     = %v", *req.Enabled))
				}
				if req.DeclaredCap != nil && !updated["declared_cap"] {
					out = append(out, fmt.Sprintf("declared_cap = %d", *req.DeclaredCap))
				}
				if req.Model != "" && !updated["model"] {
					out = append(out, fmt.Sprintf("model       = %q", req.Model))
				}
				if req.BaseURL != "" && !updated["base_url"] {
					out = append(out, fmt.Sprintf("base_url    = %q", req.BaseURL))
				}
			}
			section = strings.Trim(trimmed, "[]")
			inTarget = section == "providers."+req.Name
			out = append(out, line)
			continue
		}

		if inTarget && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			key := strings.TrimSpace(parts[0])
			switch key {
			case "enabled":
				if req.Enabled != nil {
					out = append(out, fmt.Sprintf("enabled     = %v", *req.Enabled))
					updated["enabled"] = true
					continue
				}
			case "declared_cap":
				if req.DeclaredCap != nil {
					out = append(out, fmt.Sprintf("declared_cap = %d", *req.DeclaredCap))
					updated["declared_cap"] = true
					continue
				}
			case "model":
				if req.Model != "" {
					out = append(out, fmt.Sprintf("model       = %q", req.Model))
					updated["model"] = true
					continue
				}
			case "base_url":
				if req.BaseURL != "" {
					out = append(out, fmt.Sprintf("base_url    = %q", req.BaseURL))
					updated["base_url"] = true
					continue
				}
			}
		}

		out = append(out, line)
	}

	// If the section wasn't found, append it
	if !inTarget && section != "providers."+req.Name {
		// check if any section was the target
		foundSection := false
		for _, l := range lines {
			if strings.TrimSpace(l) == "[providers."+req.Name+"]" {
				foundSection = true
				break
			}
		}
		if !foundSection {
			out = append(out, "", "[providers."+req.Name+"]")
			if req.Enabled != nil {
				out = append(out, fmt.Sprintf("enabled     = %v", *req.Enabled))
			}
			if req.DeclaredCap != nil {
				out = append(out, fmt.Sprintf("declared_cap = %d", *req.DeclaredCap))
			}
			if req.Model != "" {
				out = append(out, fmt.Sprintf("model       = %q", req.Model))
			}
			if req.BaseURL != "" {
				out = append(out, fmt.Sprintf("base_url    = %q", req.BaseURL))
			}
		}
	}

	return os.WriteFile(tomlPath, []byte(strings.Join(out, "\n")), 0600)
}

// ─── /providers format for TUI ────────────────────────────────────────────────

func formatProvidersTable(details []ApiProviderDetail) string {
	var b strings.Builder
	statusIcon := map[string]string{
		ProbeAvailable:   "✓",
		ProbeNoKey:       "⚠",
		ProbeNotFound:    "✗",
		ProbeUnavailable: "~",
		ProbeManual:      "?",
	}
	b.WriteString(fmt.Sprintf("  %-12s %-6s %-10s %s\n", "PROVIDER", "ON/OFF", "STATUS", "NOTE"))
	b.WriteString(fmt.Sprintf("  %s\n", strings.Repeat("─", 64)))
	for _, d := range details {
		on := "off"
		if d.Enabled {
			on = "on"
		}
		icon := statusIcon[d.ProbeStatus]
		note := d.ProbeNote
		if note == "" {
			note = d.SetupHint
		}
		if len(note) > 46 {
			note = note[:43] + "…"
		}
		b.WriteString(fmt.Sprintf("  %-12s %-6s %s %-8s %s\n",
			d.Name, on, icon, d.ProbeStatus, note))
	}
	b.WriteString("\n  Use /enable <name> or /disable <name> to toggle.\n")
	b.WriteString("  Edit declared_cap in .relay/relay.toml for quota tuning.\n")
	return b.String()
}

// JSON encode helper for the TUI
func jsonEncode(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// runAccountLogin opens a terminal to sign in to a specific account's config dir,
// injecting the provider's config-dir env var (e.g. CLAUDE_CONFIG_DIR) so the
// login lands in that account's directory rather than the default.
func runAccountLogin(cfg *config.Config, provider, label string, emit func(tag, msg string)) error {
	meta := providerByName(provider)
	if meta == nil {
		return fmt.Errorf("unknown provider: %s", provider)
	}
	steps, ok := meta.AuthCmds[runtime.GOOS]
	if !ok || len(steps) == 0 {
		return fmt.Errorf("no login command for %s on %s", provider, runtime.GOOS)
	}
	var configDir string
	if pc := cfg.Providers[provider]; pc != nil {
		for _, a := range pc.Accounts {
			if a.Label == label {
				configDir = a.ConfigDir
				break
			}
		}
	}
	var env []string
	if v, mapped := config.AccountConfigEnvVar(provider); mapped && configDir != "" {
		_ = os.MkdirAll(configDir, 0o700)
		env = append(env, v+"="+configDir)
	}
	env = append(env, extraAccountEnv(cfg, provider, label)...)
	emit("system", fmt.Sprintf("opening terminal to sign in to %s account %q…", provider, label))
	title := fmt.Sprintf("Relay - %s login (%s)", provider, label)
	for _, step := range steps {
		if err := openInTerminal(step.cmd, step.args, env, title); err != nil {
			return err
		}
	}
	emit("system", "  complete sign-in in the new terminal · browser may also open")
	return nil
}

// extraAccountEnv returns the account's explicit Env entries (KEY=VAL).
func extraAccountEnv(cfg *config.Config, provider, label string) []string {
	pc := cfg.Providers[provider]
	if pc == nil {
		return nil
	}
	for _, a := range pc.Accounts {
		if a.Label == label {
			return a.Env
		}
	}
	return nil
}
