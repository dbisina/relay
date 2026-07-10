# Repo map for contributing agents

Compact navigation guide. Read this before reading any source. Designed for context economy — LLMs should be able to answer "where is X?" without reading half the repo.

## Layout

```
relay/
├── packages/
│   ├── daemon-go/                Go daemon + CLI + TUI + MCP server
│   │   ├── cmd/relay/            Cobra commands, entry point
│   │   └── internal/             Library packages (no public API)
│   └── ui/                       Rust egui desktop app
│       └── src/                  RelayApp + all rendering
├── docs/                         Markdown source for the docs site
│   ├── assets/                   Images and GIFs (hero.png)
│   └── site/                     Static HTML for GitHub Pages
├── scripts/                      Contributor setup + dev + build + test
│   ├── setup.sh / setup.ps1      Self-healing one-shot setup (installs missing Go/Rust/git)
│   ├── install.sh                Curl-pipe installer for macOS/Linux
│   ├── install.ps1               PowerShell installer for Windows
│   └── record-demo.sh            Creates terminal demo GIF
├── .github/
│   ├── workflows/                CI and release (release.yml)
│   ├── ISSUE_TEMPLATE/           Bug, feature, config forms
│   └── PULL_REQUEST_TEMPLATE.md  PR checklist
├── README.md                     Project landing page
├── CLAUDE.md                     Global rules for Claude Code on this repo
├── AGENTS.md                     Same rules, agent-neutral
├── CODEMAP.md                    ← you are here
├── CONTRIBUTING.md               Short contributor guide (full guide in docs/)
├── CODE_OF_CONDUCT.md            Contributor Covenant
├── LICENSE                       Apache-2.0
└── OPEN_SOURCE.md
```

## Go side — what each package does

| Package | Where | One-line job | Public types of note |
|---|---|---|---|
| `cmd/relay` | `packages/daemon-go/cmd/relay/` | Entry point, subcommands, TUI, MCP server, provider/install metadata | `main()`, `cmdRun/cmdDaemon/cmdTUI/cmdMCP/cmdEval`, `runMCPServer`, `dispatchTool` |
| `adapter` | `internal/adapter/` | Provider-specific child-process drivers | `AdapterContract`, `StdinReplier`, `Registry`, `ClaudeAdapter`, etc. |
| `approval` | `internal/approval/` | HITL gate for risky agent actions | `Gate.Ask`, `Gate.Resolve` |
| `audit` | `internal/audit/` | Hash-chained JSONL log | `Logger.Log`, `Verify` |
| `circuit` | `internal/circuit/` | Per-provider breaker | `Registry.For`, `Breaker.Allow/RecordFailure/RecordSuccess` |
| `codegraph` | `internal/codegraph/` | Walks user repo, extracts symbols → graph nodes (Go/Rust/TS/JS/Py/JVM/Rb) | `Scan`, `Symbol`, `Module` |
| `config` | `internal/config/` | TOML parser for `.relay/relay.toml` | `Load`, `Default`, `Config`, `ProfileConfig`, `VisionConfig` |
| `contract` | `internal/contract/` | Build / sign / verify continuation contracts (v2: prompt, plan, tasks, skills, in-flight code) | `Builder.Build/Sign/Verify`, `Serializer.Serialize`, `ContinuationContract` |
| `detect` | `internal/detect/` | Discover AI agents already running (process scan + on-disk transcript & SQLite readers) and lift their intent | `Scan`, `DetectedAgent`, `SessionIntel`, `RenderHandoff`; per-store readers in `transcript.go`/`extstores.go`/`vscdb.go` |
| `fsm` | `internal/fsm/` | Handoff state machine + durability | `HandoffMachine`, `DurabilityManager` |
| `graph` | `internal/graph/` | SQLite-backed knowledge graph + FTS5 chunk retrieval | `GraphStore.UpsertNode/UpsertEdge/Stats/Recent/UpsertChunk/SearchChunks` |
| `instructions` | `internal/instructions/` | Discover CLAUDE.md/AGENTS.md/Cursor rules | `Discover`, `Composite`, `Source` |
| `orchestrator` | `internal/orchestrator/` | Main loop: FSM, handoff, routing, redaction | `Orchestrator.Run`, `SendUserReply`, `VerifierCritique` |
| `outcomes` | `internal/outcomes/` | Per-profile success log + aggregates | `Tracker.Record`, `SuccessRate`, `Aggregates` |
| `pricing` | `internal/pricing/` | Per-provider USD/Mtoken table | `Estimate`, `Format2/Format4`, `Table` |
| `process` | `internal/process/` | OS-specific subprocess setup (Job Object/SetSid) | `SetupChildProcess`, `AssignToJobObject` |
| `quota` | `internal/quota/` | Per-provider quota detection + burn-rate ledger (quota wallet, forecast for predictive handoff) | `Registry`, `Ledger`, per-adapter quota clients |
| `redact` | `internal/redact/` | Secret pattern scrubber | `Redactor.Scrub`, `DefaultRules` |
| `server` | `internal/server/` | HTTP + WebSocket API | `Server.Start`, `SetXxxHandlers`, `PushXxx` |
| `worktree` | `internal/worktree/` | Per-session git worktree | `Manager.Create/Diff/Discard` |

## Rust side

| File | Job |
|---|---|
| `src/main.rs` | eframe entry, eframe options |
| `src/app.rs` | `RelayApp` + every `draw_*` function. ~3000 lines. Search by section banner (e.g. `// ═══ Settings ═══`) |
| `src/api.rs` | HTTP poll thread + `send_*` action helpers + folder picker |
| `src/theme.rs` | Color tokens, rounding, spacing, `apply()` to egui Visuals |
| `src/types.rs` | serde DTOs mirroring the Go API exactly |

## HTTP API surface

All endpoints under `http://127.0.0.1:4748`. Implemented in `internal/server/server.go`, wired from `cmd/relay/main.go` (daemon mode) and `internal/orchestrator/orchestrator.go` (during a session).

| Path | Method | Handler | Source |
|---|---|---|---|
| `/api/health` | GET | `handleHealth` | server.go |
| `/api/status` | GET | `handleStatus` | server.go |
| `/api/providers` | GET | `handleProviders` | server.go |
| `/api/events` | GET | `handleEvents` | server.go |
| `/api/contract` | GET | `handleContract` | server.go |
| `/api/instructions` | GET | `handleInstructions` | server.go |
| `/api/graph` | GET | `handleGraph` | server.go |
| `/api/graph/detail` | GET | `handleGraphDetail` | server.go |
| `/api/retrieval` | GET | `handleRetrieval` (FTS5) | server.go |
| `/api/handoff` | POST | `handleHandoff` | server.go |
| `/api/run` | POST | `handleRun` | server.go |
| `/api/config/providers` | GET/POST | `handleConfigProviders` | server.go |
| `/api/providers/install` | POST | `handleProviderInstall` | server.go |
| `/api/providers/oauth` | POST | `handleProviderOAuth` | server.go |
| `/api/providers/api-key` | POST | `handleProviderAPIKey` | server.go |
| `/api/profiles` | GET/POST | `handleProfiles` | server.go |
| `/api/vision/config` | GET/POST | `handleVisionConfig` | server.go |
| `/api/vision/probe` | POST | `handleVisionProbe` | server.go |
| `/api/session/reply` | POST | `handleSessionReply` | server.go |
| `/api/detect` | GET | `handleDetect` (`?sinceHours=N`) | server.go |
| `/api/detect/adopt` | POST | `handleDetectAdopt` | server.go |
| `/api/providers/account` | POST | `handleSwitchAccount` (account-aware handoff) | server.go |
| `/api/quota/wallet` | GET | `handleWallet` (per-account remaining + burn ETA) | server.go |
| `/api/pipelines` | GET/POST | `handlePipelines` (multi-agent DAGs) | server.go |
| `/api/pipelines/run` | POST | `handlePipelineRun` | server.go |
| `/api/history` | GET | `handleHistory` (time machine timeline) | server.go |
| `/api/history/commits` | GET | `handleHistoryCommits` | server.go |
| `/api/history/diff` | GET | `handleHistoryDiff` | server.go |
| `/api/history/rewind` | POST | `handleHistoryRewind` | server.go |
| `/api/session/pause` | POST | `handlePause` | server.go |
| `/api/session/diff` | GET | `handleDiff` | server.go |
| `/api/session/cost` | GET | `handleCost` | server.go |
| `/api/approvals` | GET | `handleApprovals` | server.go |
| `/api/approvals/<id>` | POST | `handleApprovalResolve` | server.go |
| `/api/circuit` | GET | `handleCircuit` | server.go |
| `/api/outcomes` | GET | `handleOutcomes` | server.go |
| `/api/redactions` | GET | `handleRedactions` | server.go |
| `/api/ollama/models` | GET | `handleOllamaModels` | server.go |
| `/api/ollama/pull` | POST | `handleOllamaPull` | server.go |
| `/api/ollama/launch` | POST | `handleOllamaLaunch` | server.go |
| `/ws` | WS | `handleWS` | server.go |

## Key control-flow paths

### Starting a session

```
relay run "task"            (main.go cmdRun)
  → runSession              (main.go)
    → orchestrator.New      (orchestrator.go)
       → worktree.New + Create
       → fsm.NewHandoffMachine
       → matchProfile       (orchestrator.go bottom)
    → orchestrator.Run      (orchestrator.go)
       → runOneProvider     (orchestrator.go)
          → adapter.Run     (adapter/claude.go etc.)
          → buildSystemPrompt → instructions.Discover
          → drain event channel, call handleEvent
       → if quota breach → doHandoff
          → adapter.AwaitSafePauseWindow
          → durability.Snapshot
          → contract.Builder.Build/Sign
          → contract.WriteJSON sidecar
          → adapter.ForceStop
          → fsm transitions through SNAPSHOTTED → ENVELOPE_BUILT → DISPATCHED
          → durability.PollHeartbeat
          → fsm.ExecuteResume
```

### Event flow

```
adapter sends AgentEvent on channel
  → orchestrator.handleEvent
     → check paused → block on pauseCh
     → redactor.Scrub on Content + Meta
     → if EventToolResult and Meta has tokens_in/tokens_out → addUsage → pricing.Estimate
     → server.PushEvent → broadcast WebSocket
     → auditLog.Log
     → graphStore.UpsertNode + UpsertEdge
     → extractGraphFacts (regex for decision/constraint/file paths)
     → pushStatus (refreshes /api/status state)
```

### Provider auto-install

```
UI clicks "Install"
  → POST /api/providers/install  {name}
  → installProviderCB (set in main.go cmdDaemon)
  → runInstall in providers.go
  → resolveCommandForTerminal (uses npx fallback if not in PATH)
  → openInTerminal (wt.exe/cmd.exe/osascript/x-terminal-emulator)
  → user watches install in spawned window
  → next probe cycle (1.5s) finds the new binary via extraSearchDirs
```

## Naming conventions

- Server callbacks: `xxxCB func(...)`. Set via `Server.SetXxxHandlers(...)`. main.go wires them at startup.
- Anything user-facing in the event log: `emit("system", "...")` / `emit("result", ...)`.
- Anything for the graph: `o.graphStore.UpsertNode/UpsertEdge` from orchestrator.
- Anything provider-specific: under `internal/adapter/<name>.go`.

## Tests

Growing. Go unit tests live next to code (`<file>_test.go`) — e.g. adapter event
parsing, contract, config/accounts, pipeline, quota ledger, detect stores, and
`internal/graph/store_test.go` (recent-neighborhood edge behaviour). Lint is
enforced via `.golangci.yml` (govet, ineffassign, staticcheck). Adding tests is
a great first contribution. See `docs/contributing.md`.

### CLI note

`relay init` scaffolds `.relay/relay.toml`, a signing key, and a starter
`.relay/eval/tasks.json`, so `relay eval` runs immediately after init.

### Desktop note

Opening `relay-ui` calls `api::ensure_daemon_running`, which starts the daemon
(detached) if it is not already up; closing the window leaves it running for the
CLI. See `packages/ui/src/api.rs`.

## When changing X, touch Y

| Change | Files to update |
|---|---|
| Add a provider | `interface.go` (const), `<name>.go` (adapter), `registry.go`, `cmd/relay/providers.go` (metadata + probe), `internal/pricing/pricing.go` |
| Detect a new agent's sessions | add a `scan<Name>` reader (`internal/detect/transcript.go` JSONL, `extstores.go` JSON, or `vscdb.go` SQLite), register it on the provider's `signature` in `signatures.go`, add a fixture test. Great first contribution. |
| Add an API endpoint | `server/server.go` (handler + register), `main.go` or `orchestrator.go` (wire CB), `packages/ui/src/api.rs` (Rust client) + `types.rs` (DTO) |
| Add a CLI command | `cmd/relay/*.go` (new file recommended), register in `main.go` `root.AddCommand` |
| Add a setting | `internal/config/config.go` (struct + parser + default TOML), surface via `relay.toml` |
| Add a UI page | `packages/ui/src/app.rs` (NavPage variant + draw_xxx + icon in paint_icon), `docs/architecture.md` |

## Roadmap markers

Search the codebase for `// TODO` and `// roadmap:` to find tracked deferrals. Don't add new TODOs without a linked issue.

Known-unfinished items intentionally scaffolded:

- `VerifierCritique` — peer-review pass. Frame ships, full spawn loop pending refactor decoupling adapter from session ownership.
- Embedding-backed retrieval — schema designed, implementation pending. See `internal/codegraph` for code-graph foundation that retrieval will build on.
- OS automation in vision loop — observation works, action does not (deliberate; opt-in robotgo/enigo path pending).
