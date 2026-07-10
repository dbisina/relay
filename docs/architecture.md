# Architecture

Relay is split into a long-running Go daemon, a Rust desktop app, and a Go TUI. All three talk to the daemon via HTTP + WebSocket. There is no separate API server — the daemon serves the API.

## Process model

```
┌──────────────────────────────────────────────────────────────────────┐
│                       relay (Go binary)                              │
│                                                                      │
│  cmd `relay run "task"`  →  process: daemon + orchestrator           │
│  cmd `relay daemon`      →  process: daemon only (no session)        │
│  cmd `relay tui`         →  process: Bubble Tea client → HTTP        │
│                                                                      │
│  HTTP server: 127.0.0.1:4748                                         │
│  WebSocket:   127.0.0.1:4748/ws                                      │
└─────────────────────────┬────────────────────────────────────────────┘
                          │
              ┌───────────┴────────────┐
              │                        │
     ┌────────▼────────┐      ┌────────▼─────────┐
     │   relay-ui      │      │   relay tui      │
     │   (Rust egui)   │      │   (Bubble Tea)   │
     └─────────────────┘      └──────────────────┘
```

Single source of truth is the daemon. UIs are pure renderers that poll HTTP and POST actions back.

## Package layout

```
packages/daemon-go/
├── cmd/relay/                 cobra-based CLI + TUI + entry point
│   ├── main.go                root command, run/daemon wiring
│   ├── tui.go                 Bubble Tea TUI
│   ├── providers.go           probe + install + auth metadata
│   ├── profiles.go            profile CRUD
│   ├── vision.go              screenshot + vision LLM
│   ├── eval.go                relay eval golden suite
│   └── *_{windows,unix}.go    OS-specific helpers
└── internal/
    ├── adapter/               provider adapters (Run / Capability / SafePause)
    │   ├── interface.go       AdapterContract + StdinReplier
    │   ├── claude.go          NDJSON streaming, stdin replies
    │   ├── ollama.go, codex.go, ...
    │   └── registry.go
    ├── codegraph/             scan user repo → symbol + module nodes
    ├── orchestrator/          main loop (FSM, handoff, routing, redaction)
    ├── quota/                 per-provider quota tracking + claude proxy
    ├── contract/              build / sign / verify continuation contracts
    ├── fsm/                   handoff state machine + durability
    ├── graph/                 SQLite knowledge graph
    ├── audit/                 hash-chained JSONL log
    ├── server/                HTTP + WebSocket
    ├── instructions/          CLAUDE.md / AGENTS.md discovery
    ├── worktree/              per-session git worktree
    ├── redact/                secret scrubber
    ├── circuit/               per-provider circuit breaker
    ├── pricing/               cost estimation
    ├── outcomes/              profile success tracker
    └── approval/              human-in-the-loop gate

packages/ui/                    Rust egui desktop client
├── src/
│   ├── main.rs                 eframe entry
│   ├── app.rs                  RelayApp + all draw_* functions
│   ├── api.rs                  HTTP poller + send helpers
│   ├── theme.rs                palette + tokens
│   └── types.rs                serde DTOs mirroring the Go API
└── assets/relay.png
```

## Handoff state machine

```
       ┌──────────────┐
       │   RUNNING    │◀────────────────────────────┐
       └──────┬───────┘                             │
              │ quota breach / manual handoff       │
              ▼                                     │
       ┌──────────────┐                             │
       │   PAUSING    │  waits for safe-pause       │
       └──────┬───────┘  (gate, not predicate)      │
              │                                     │
              ▼                                     │
       ┌──────────────┐                             │
       │ SNAPSHOTTED  │  git commit on session      │
       └──────┬───────┘  branch                     │
              │                                     │
              ▼                                     │
       ┌──────────────────┐                         │
       │ ENVELOPE_BUILT   │  contract Markdown      │
       └──────┬───────────┘  + signed JSON sidecar  │
              │                                     │
              ▼                                     │
       ┌──────────────┐                             │
       │ DISPATCHED   │  next adapter spawned       │
       └──────┬───────┘                             │
              │                                     │
              ▼                                     │
       ┌──────────────┐                             │
       │  RESUMING    │  receiver heartbeat ────────┘
       └──────────────┘
```

Each transition is durable: written to `.relay/fsm.json` before the next state is entered. Crash mid-transition? Restart picks up where it stopped.

## Continuation contract

Markdown-first for human readability, JSON-signed for tamper detection.

```
.relay/envelope.md       human-readable, injected as --system-prompt-file
.relay/envelope.json     canonical JSON with HMAC-SHA256 signature
```

Schema (abridged):

```json
{
  "contractId": "c-abc123",
  "sessionId":  "s-...",
  "taskId":     "t-...",
  "version":    2,
  "taskGoal":   "add refund flow",
  "nextAction": "Wire POST /orders/:id/refund",
  "doNotRedo":  ["migration 0042 applied"],

  "initialPrompt":  "add a refund flow to the orders service",
  "plan":           ["migration 0042 applied", "Wire POST /orders/:id/refund", "tests"],
  "tasksRemaining": ["Wire POST /orders/:id/refund", "tests"],
  "skillsLoaded":   ["backend-patterns"],
  "skillsInUse":    ["go-concurrency-patterns"],
  "skillsToUse":    ["test-automator"],
  "inFlightCode":   [{"path": "orders/refund.go", "snippet": "func Refund(... // truncated"}],

  "acceptanceAssertions": [
    "idempotent", "emits OrderRefunded", "unit tests pass"
  ],
  "fileManifest": [
    {"path": "orders/refund.go", "sha256": "...", "modified": true}
  ],
  "decisions":   [...],
  "constraints": [...],
  "snapshotCommitSHA": "a3f8c2d",
  "signature": "hex(HMAC-SHA256(canonical JSON, signing-key))"
}
```

Schema **v2** adds the rich session-intent block (`initialPrompt`, `plan`,
`tasksRemaining`, `skills*`, `inFlightCode`). These are populated either live by
the orchestrator (from graph nodes) or lifted from a detected agent's transcript
by `internal/detect` during adoption. v1 contracts omit the block entirely and
serialise byte-for-byte as before (migration guard in `Serializer.Serialize`).

Verifier path: orchestrator reads `envelope.json` on resume, runs `Builder.Verify`, refuses to inject if signature fails.

## Adapter contract

```go
type AdapterContract interface {
    Capability() ProviderCapability
    Run(ctx, opts RunOptions, ch chan<- AgentEvent) error
    AwaitSafePauseWindow(ctx, breachReason string) (SafePoint, error)
    ForceStop() error
}

// Optional — adapters that can accept user replies during a session.
type StdinReplier interface {
    SendStdin(reply string) error
}
```

Streaming: each adapter normalises its native event format to `AgentEvent {Type, Content, Meta}`. The orchestrator doesn't care whether the source is NDJSON, SSE, or stdout text.

## Knowledge graph

SQLite. Three tables:

```sql
CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,
  node_type   TEXT,        -- task | module | symbol | decision | constraint | file | do_not_redo | tool_use | ...
  payload     JSON,
  session_id  TEXT,
  weight      REAL,
  created_at  TIMESTAMP,
  updated_at  TIMESTAMP
);
CREATE TABLE edges (
  from_id     TEXT,
  to_id       TEXT,
  edge_type   TEXT,        -- defines | imports | touches | mentions | has_fact | extracted | emitted | scope
  weight      REAL
);
-- Retrieval support: code chunks + FTS5 index
CREATE TABLE chunks (
  id TEXT PRIMARY KEY, path TEXT, lang TEXT,
  startLine INT, endLine INT, body TEXT, sha TEXT,
  embedding BLOB, dim INT
);
CREATE VIRTUAL TABLE chunks_fts USING fts5(body, path, content='chunks', content_rowid='rowid');
```

Populated by two sources:

1. **Code-graph scanner** (`internal/codegraph`) — runs once at session start. Walks the user repo, parses Go / Rust / TS / JS / Python / Java / Ruby for top-level symbols, writes `module` and `symbol` nodes.
2. **Agent activity** — orchestrator extracts file paths, decisions, constraints, do-not-redo items from `tool_use` / `tool_result` / `text` events.

Retrieval: `GET /api/retrieval?q=...&limit=20` runs FTS5 over chunk bodies. The MCP `relay_retrieve` tool wraps this for LLM clients. Embedding column is reserved for future vector-based ranking.

## Audit log

Append-only JSONL. Each line:

```json
{
  "ts":       "2026-06-03T15:41:03.123Z",
  "prevHash": "0000…",
  "hash":     "a3f8c2d…",
  "event":    "agent_event",
  "data":     { ... }
}
```

`hash = SHA-256(prevHash + canonical(event + data))`. `relay audit verify` walks the chain and reports breakage.

## What runs where

| Component | Process | Owns |
|---|---|---|
| HTTP server | `relay daemon` / `relay run` | API endpoints |
| Orchestrator | inside `relay run` (or `/api/run` handler) | FSM, contract, handoff |
| Adapter | child subprocess of orchestrator | one provider session |
| Quota tracker | inside daemon | one tracker per adapter |
| Graph store | inside daemon | SQLite handle |
| relay-ui | separate process | renders /api/* state |
| relay tui | separate process | Bubble Tea over /api/* |
| Vision probe | inside orchestrator goroutine | screenshot + LLM call |
| Worktree | external (`git`) | per-session branch |

## Why split daemon + UI?

So you can `relay run` headless on a server, or live-pair with a teammate by both pointing UIs at the same daemon, or run the TUI over SSH. The UI is a thin client. Killing it never interrupts a session.

## MCP server

`relay mcp` reads JSON-RPC 2.0 frames from stdin, writes responses to stdout. Each tool call is a thin wrapper over the same loopback HTTP API the UIs use. Lets any MCP-aware LLM client (Claude Desktop, Cursor, Cline, Continue, Zed) drive Relay without the CLI:

```
Claude Desktop ──stdio──→ relay mcp ──HTTP──→ relay daemon
                          (JSON-RPC)         (127.0.0.1:4748)
```

The MCP server is a stateless adapter. The daemon owns the session. See [docs/mcp.md](mcp.md) for tool list + client setup.
