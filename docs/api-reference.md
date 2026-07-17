# API reference

The daemon serves a JSON HTTP API on `127.0.0.1:4748` (configurable). Authentication: none. The API is loopback-only. UIs and integrations talk to this.

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

## Accounts

Multiple logins per provider (account-aware handoff). Each account is a label plus an isolated config dir; the adapter is pointed at the active account's dir.

### `POST /api/providers/account`  `{provider, label}`

Switch the active account for a provider. The next session (or handoff) uses it.

### `POST /api/providers/account/add`  `{provider, label, configDir}`

Register a login. `configDir` optional, defaults to an isolated per-label dir.

### `POST /api/providers/account/remove`  `{provider, label}`

Remove a login.

### `POST /api/providers/account/login`  `{provider, label}`

Spawns a terminal running the provider's login flow against that account's config dir.

Accounts are returned inside `GET /api/config/providers` as `accounts: [{label, active, configDir}]` + `activeAccount`.

## Detection & adoption

### `GET /api/detect?sinceHours=<n>`

Scan for AI coding agents already running on this machine (process scan + on-disk transcript stores). Default window 24h.

```json
[
  {
    "id":          "claude_6d569909",
    "provider":    "claude",
    "displayName": "Claude Code",
    "surface":     "cli",
    "pid":         14032,
    "running":     true,
    "workDir":     "C:/dev/my-project",
    "account":     "personal",
    "detectedVia": ["process", "transcript"],
    "lastActive":  1752666000,
    "session": {
      "sessionId":      "6d569909…",
      "model":          "claude-sonnet-5",
      "initialPrompt":  "add a refund flow…",
      "plan":           ["…"],
      "tasksRemaining": ["…"],
      "filesTouched":   ["orders/refund.go"],
      "skills":         ["backend-patterns"],
      "mcps":           ["github"],
      "tokensIn":       84210,
      "tokensOut":      12045,
      "messageCount":   37
    }
  }
]
```

### `POST /api/detect/adopt`  `{id, target, start}`

Lift a detected session's intent into a continuation brief (persisted under `.relay/adopted/`), targeting provider `target`. With `"start": true` also launches a Relay session that continues the work.

## Quota wallet

### `GET /api/quota/wallet`

Per provider+account: remaining quota, reset time, live burn rate, and forecast ETA (feeds predictive handoff).

```json
[
  {
    "provider":     "claude",
    "account":      "personal",
    "remaining":    12400,
    "total":        40000,
    "fractionUsed": 0.69,
    "resetsAt":     "2026-07-16T21:00:00Z",
    "source":       "transcript",
    "burnPerMin":   310.5,
    "etaMinutes":   39.9
  }
]
```

## Pipelines

Multi-agent DAGs: one provider+task per node, explicit `dependsOn` ordering, `fallback` providers on failure, `verify` acceptance commands (verifier gate) per node.

### `GET /api/pipelines`

```json
[
  {
    "name": "feature",
    "nodes": [
      {
        "id":        "build",
        "provider":  "claude",
        "task":      "implement the endpoint",
        "dependsOn": [],
        "fallback":  ["codex"],
        "verify":    ["go build ./...", "go test ./..."]
      }
    ]
  }
]
```

### `POST /api/pipelines`

Saves the full pipeline list (same shape as GET response). Persisted to `.relay/pipelines.json`.

### `POST /api/pipelines/run`  `{name}`

Executes the pipeline's nodes in dependency order. Honours the single-session guard: errors with `"session already running"` if one is active.

## Time machine

### `GET /api/history`

The handoff timeline, read from the audit log.

```json
[
  {"seq": 3, "ts": "2026-07-16T09:41:03Z", "event": "handoff", "provider": "claude", "summary": "quota breach → codex"}
]
```

### `GET /api/history/commits`

Snapshot commits on the session branch: `[{sha, short, subject, when}]`.

### `GET /api/history/diff?sha=<sha>`

The diff for one snapshot commit.

### `POST /api/history/rewind`  `{sha}`

Non-destructive rewind: creates a new branch at the snapshot so current work is never lost.

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

### `GET /api/graph/project?path=<repo>`

One-shot codegraph scan of an arbitrary repo path (no session needed): returns `{nodes, edges}` of `module` / `symbol` nodes. Used by the desktop Graph page's project picker.

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
