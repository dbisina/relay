# Providers

Relay ships adapters for eight providers out of the box. Each adapter normalises that provider's native event stream into Relay's `AgentEvent`. You don't need every one — pick what you have keys/subscriptions for and disable the rest in `.relay/relay.toml`.

## Built-in adapters

| Name | Type | Auth methods | Stdin replies | Native install |
|---|---|---|:---:|:---:|
| `claude` | CLI subprocess | OAuth via `claude login`, API key | ✓ | `npm i -g @anthropic-ai/claude-code` |
| `codex` | CLI subprocess | API key (`OPENAI_API_KEY`) | — | `npm i -g @openai/codex` |
| `antigravity` | IDE | manual | — | download from antigravity.google |
| `opencode` | CLI subprocess | API key | — | `npm i -g opencode-ai` / `brew install opencode-ai/tap/opencode` |
| `ollama` | local HTTP | — (local) | — | `winget install Ollama.Ollama` / `brew install ollama` / curl install |
| `copilot` | gh extension | OAuth via `gh auth login` | — | `gh extension install github/gh-copilot` |
| `continue` | VS Code ext | extension UI | — | `code --install-extension Continue.continue` |
| `cline` | VS Code ext | extension UI | — | `code --install-extension saoudrizwan.claude-dev` |

## Auto-install + auth

From the desktop app's **Settings → Providers** tab:

- **[Install]** spawns a terminal running the per-OS install command for your platform (Windows: `winget` / `npm.cmd`; macOS: `brew`/`npm`; Linux: `apt`-friendly install scripts).
- **[Sign in with browser]** runs the provider's OAuth subcommand (`claude login`, `gh auth login --web`, etc.).
- **[Use API key]** writes the key to `.relay/.env` (gitignored) and reloads the daemon's env so spawned agents inherit it.
- **[Run via Ollama]** opens `ollama launch <tool> --model <model>` so a provider that needs cloud auth can run against a local model instead.

## Probe

Every poll cycle (~1.5s) the daemon probes:

```go
for each provider:
  check PATH + extra dirs (%APPDATA%\npm, ~/.local/bin, brew prefix, etc.)
  check provider-specific env vars
  check IDE extension dirs for VS Code providers
  → status: available | no_key | not_found | unavailable | manual
```

Status drives the colour-coded tag in the UI and the orchestrator's "which providers are eligible right now" decision.

## Quota source

Each adapter has a `quota.Adapter` paired with it. Three detection strategies:

1. **Proxy header** (Claude): a local HTTP proxy injects between the CLI and the API, parses `x-anthropic-ratelimit-tokens-remaining`. Authoritative.
2. **Session file**: provider writes usage to a known on-disk path. Polled.
3. **Declared cap** + counted requests: fallback when no API surface exists. From `.relay/relay.toml`.
4. **429 backstop**: detects exhaustion on first 429 even when no other signal works.

The UI surfaces which source is in use per provider.

## Cost

`internal/pricing/pricing.go` maps each provider to per-million-token rates (input + output). Updated by hand; PRs welcome.

```go
"claude":      {3.00, 15.00},   // Sonnet tier
"codex":       {5.00, 15.00},
"opencode":    {3.00, 15.00},
"ollama":      {0.0, 0.0},
"copilot":     {0.0, 0.0},      // flat subscription
```

The orchestrator records token usage per event (from adapter `Meta` fields) and shows live USD in the dashboard footer.

## Adding a new adapter

Five steps. Reference: `internal/adapter/claude.go`.

### 1. Add the name

```go
// internal/adapter/interface.go
const (
    ProviderClaude      ProviderName = "claude"
    // ...
    ProviderYourTool    ProviderName = "yourtool"
)
```

### 2. Implement the adapter

```go
// internal/adapter/yourtool.go
type YourToolAdapter struct {
    mu          sync.Mutex
    cmd         *exec.Cmd
    safePauseCh chan SafePoint
}

func NewYourToolAdapter() *YourToolAdapter { ... }

func (a *YourToolAdapter) Capability() ProviderCapability {
    return ProviderCapability{
        Name:                ProviderYourTool,
        InjectionSemantics:  InjectionSystemLayer,
        MaxTokensPerSession: 100_000,
        SupportsStreaming:    true,
        SupportsTools:       true,
    }
}

func (a *YourToolAdapter) Run(ctx context.Context, opts RunOptions, ch chan<- AgentEvent) error {
    // 1. Build command line, set env
    // 2. cmd.Start()
    // 3. Drain stdout, parse to AgentEvent, send on ch
    // 4. Close ch on exit
}

func (a *YourToolAdapter) AwaitSafePauseWindow(...) (SafePoint, error) { ... }
func (a *YourToolAdapter) ForceStop() error { ... }
```

Implementing `StdinReplier.SendStdin(reply)` is optional but unlocks the inline-reply UI for your adapter.

### 3. Register

```go
// internal/adapter/registry.go
func BuildAdapterRegistry(proxyPort int, ollamaURL, ollamaModel string) Registry {
    return Registry{
        // ...
        ProviderYourTool: NewYourToolAdapter(),
    }
}
```

### 4. Add metadata

```go
// cmd/relay/providers.go
{
    Name:        "yourtool",
    DisplayName: "Your Tool",
    Description: "What it does.",
    Kind:        "cli",
    CanInstall:  true,
    CanOAuth:    false,
    CanAPIKey:   true,
    APIKeyEnvVar: "YOURTOOL_API_KEY",
    APIKeyURL:    "https://yourtool.example.com/keys",
    InstallCmds: map[string][]installStep{
        "windows": {{cmd: "npm.cmd", args: []string{"install", "-g", "yourtool"}}},
        "darwin":  {{cmd: "npm", args: []string{"install", "-g", "yourtool"}}},
        "linux":   {{cmd: "npm", args: []string{"install", "-g", "yourtool"}}},
    },
},
```

### 5. Add a probe arm

```go
// cmd/relay/providers.go probeProvider
case "yourtool":
    if commandExists("yourtool") { return ProbeAvailable, "yourtool CLI found" }
    return ProbeNotFound, "yourtool CLI not found"
```

Pricing too — add to `internal/pricing/pricing.go`.

That's it. Ship it.

## Ollama as a backend

Several providers (Claude Code, Codex, OpenCode, Cline) support `ollama launch` — a built-in bridge that points them at a local Ollama model. The desktop app surfaces this as a **"Run via Ollama"** button on each provider card when `ollama` is installed.

```
relay-ui → POST /api/ollama/launch  {"provider":"claude","model":"qwen3.5"}
       → opens terminal: ollama launch claude --model qwen3.5
```

Use case: you have no Anthropic subscription but want to use Claude Code as your agent. Pick a local model.
