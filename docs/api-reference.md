# API reference

The daemon serves a JSON HTTP API on `127.0.0.1:4748` (configurable). Authentication: none — the API is loopback-only. UIs and integrations talk to this.

## Health & status

### `GET /api/health`

```json
{"ok": true, "port": 4748}
```

### `GET /api/status`

```json
{
  "sessionId":      "f3a8c2…",
  "taskId":         "t-f3a8c2-1748906412",
  "taskGoal":       "add refund flow to orders service",
  "role":           "developer",
  "activeProvider": "claude",
  "tokensUsed":     74200,
  "graphNodes":     342,
  "handoffsDone":   2,
  "hfsScore":       0.94,
  "hfsHistory":     [0.40, 0.50, …],
  "fsmState":       "RUNNING"
}
```

## Providers

### `GET /api/providers`

Live runtime status per provider (active/standby/exhausted/error).

### `GET /api/config/providers`

Full metadata + install/auth capability flags + probe result.

### `POST /api/config/providers`

Body:

```json
{
  "name":        "claude",
  "enabled":     true,
  "declaredCap": 40000,
  "model":       "qwen2.5-coder:32b",
  "baseUrl":     "http://localhost:11434"
}
```

Patches `[providers.<name>]` in `relay.toml` and reloads.

### `POST /api/providers/install`  `{name}`

Spawns a terminal running the per-OS install command.

### `POST /api/providers/oauth`  `{name}`

Spawns the provider's OAuth subcommand (e.g. `claude login`).

### `POST /api/providers/api-key`  `{name, value}`

Writes key to `.relay/.env`, reloads daemon env.

## Profiles

### `GET /api/profiles`

```json
[
  {
    "name":        "backend",
    "chain":       ["claude", "codex"],
    "kinds":       ["api", "service"],
    "skills":      ["go", "ts"],
    "contextHint": "production backend services"
  }
]
```

### `POST /api/profiles`

```json
{
  "name":        "backend",
  "chain":       ["claude", "codex"],
  "kinds":       ["api", "service"],
  "skills":      ["go", "ts"],
  "contextHint": "production backend services",
  "delete":      false
}
```

Upserts (or deletes when `"delete": true`).

## Session control

### `POST /api/run`  `{task, threshold, maxHandoffs}`

Starts a session in the running daemon. Single-session: 409 if one is already active.

### `POST /api/handoff`

Trigger immediate handoff.

### `POST /api/session/pause`  `{pause}`

Toggle agent execution (cooperative; honoured at event boundaries).

### `POST /api/session/reply`  `{reply}`

Send a stdin reply to the active adapter (if it implements `StdinReplier`).

### `GET /api/session/diff`

```json
{
  "summary": " orders/refund.go | 42 +++\n orders/routes.go | 18 +-",
  "diff":    "diff --git a/orders/refund.go …"
}
```

Returns `git diff main...relay/<sid>` against the user's main branch.

### `GET /api/session/cost`

```json
{
  "sessionUsd": 0.0042,
  "tokensIn":   1820,
  "tokensOut":  411,
  "provider":   "claude"
}
```

## Approvals

### `GET /api/approvals`

Pending requests waiting for user resolution.

```json
[
  {
    "id":        "abc123",
    "action":    "write 250 lines to orders/refund.go",
    "reason":    "exceeds 200-line threshold",
    "severity":  "warn",
    "createdAt": "2026-06-03T15:41:03Z"
  }
]
```

### `POST /api/approvals/<id>`  `{approved, note}`

Resolve a pending request.

## Events stream

### `GET /api/events?since=<id>`

Pull events newer than the given ID:

```json
[
  {"id": 412, "ts": "09:41:27", "tag": "quota",  "msg": "claude: 74% used"},
  {"id": 413, "ts": "09:41:28", "tag": "tool_use", "msg": "Edit orders/routes.go"}
]
```

Tag values: `tool_use`, `tool_result`, `text`, `system`, `quota`, `handoff`, `waiting`, `error`.

### `WS /ws`

WebSocket push of the same events as they happen. Reduces latency from 1500ms (poll) to ~50ms.

## Graph & retrieval

### `GET /api/graph`

`{"nodes": 342, "edges": 418}`

### `GET /api/graph/detail`

Full node + edge list (~200 most recent). Includes `module` and `symbol` nodes from the codebase scan plus session-specific `decision`, `constraint`, `file`, `do_not_redo` nodes from agent activity.

### `GET /api/retrieval?q=<text>&limit=<n>`

FTS5 full-text search over indexed code chunks. Default limit 20.

```json
[
  {
    "ID":        "chunk:orders/refund.go:1-40",
    "Path":      "orders/refund.go",
    "Lang":      "go",
    "StartLine": 1,
    "EndLine":   40,
    "Body":      "package orders\n\nimport ...\n\nfunc (s *RefundService) execute(...) error {\n  ...\n}",
    "SHA":       "a3f8c2d4",
    "Dim":       0
  }
]
```

Used by the MCP `relay_retrieve` tool. Chunks are populated by the codegraph scanner at session start; future embedding-based ranking will use the `embedding` blob column.

## Audit & contract

### `GET /api/contract`

The current continuation contract as JSON.

### `GET /api/instructions`

Discovered CLAUDE.md / AGENTS.md / Cursor rules + profile skills.

## Vision

### `GET /api/vision/config`
### `POST /api/vision/config`
### `POST /api/vision/probe`

See [Vision fallback](vision.md).

## Ollama bridge

### `GET /api/ollama/models?baseUrl=...`

Installed models + curated vision pull list.

### `POST /api/ollama/pull`  `{baseUrl, tag}`

Stream-pulls a model from ollama.com. Progress emitted to event log.

### `POST /api/ollama/launch`  `{provider, model}`

Spawns `ollama launch <tool> --model <model>` in a new terminal.

## Observability

### `GET /api/circuit`

Per-provider circuit breaker snapshots.

```json
[
  {"name": "claude", "state": "closed",    "failures": 0},
  {"name": "codex",  "state": "open",      "failures": 3, "openedAt": "..."},
  {"name": "ollama", "state": "half-open", "failures": 0}
]
```

### `GET /api/outcomes`

Profile success aggregates.

```json
[
  {
    "profile":    "backend",
    "runs":       12,
    "successes":  10,
    "successRate": 0.833,
    "avgTokens":  18420,
    "avgCostUsd": 0.087,
    "lastRun":    "2026-06-03T15:41:03Z"
  }
]
```

### `GET /api/redactions`

Counts of secrets scrubbed this session.

```json
{"summary": "openai_key:2, env_secret_pair:1"}
```
