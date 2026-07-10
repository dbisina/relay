# CLI reference

The `relay` binary is both the daemon, the run command, and the TUI client. One binary, several entry points.

## Commands

### `relay init`

Initialises `.relay/` in the current directory: writes `relay.toml`, generates the signing key, appends to `.gitignore`.

### `relay run "<task>" [flags]`

Starts a task and runs the orchestrator until completion or `MaxHandoffs`.

Flags:

```
-f, --force-handoff float     handoff threshold (0.0–1.0) (default 0.85)
-n, --max-handoffs int        max handoffs (0 = unlimited)
-p, --providers strings       ordered provider list (overrides config)
    --no-ui                   disable HTTP server (no relay-ui)
-y, --yes                     skip confirmation prompt
```

### `relay daemon`

Runs the dashboard API without starting a session. Use when you want the desktop app to launch tasks via `POST /api/run` rather than running them via CLI.

Flags:

```
    --port int                dashboard API port (default from relay.toml or 4748)
```

### `relay status`

Prints the current session JSON.

### `relay handoff`

POSTs to `/api/handoff` — triggers immediate handoff in the running session.

### `relay resume`

Writes a receiver heartbeat to `.relay/`, unblocking a session stuck in `DISPATCHED`. Use after restoring from a snapshot.

### `relay audit verify`

Walks the hash chain in `.relay/audit.jsonl` and reports breakage.

### `relay graph`

Prints node + edge counts.

### `relay tui`

Opens the Bubble Tea TUI (full-screen alt-buffer). Equivalent: `relay interactive`, `relay shell`. Default when `relay` is invoked with no args inside a real terminal.

### `relay eval [--path tasks.json] [--json]`

Runs golden-task routing regression. Exits non-zero on mismatch. Wire into CI.

### `relay mcp`

Runs Relay as a Model Context Protocol server over stdio. Used by MCP-aware LLM clients (Claude Desktop, Cursor, Cline, Continue, Zed). See [docs/mcp.md](mcp.md) for client configuration.

Requires the daemon to be running on :4748. Start with `relay daemon` separately.

## TUI shortcuts

Inside `relay tui`:

| Key | Action |
|---|---|
| `/` | Open command popup |
| `Tab` | Autocomplete cycle in popup |
| `Enter` | Execute command or select popup item |
| `Esc` | Close popup |
| `↑ / ↓` | History (when popup closed) / popup nav (when open) |
| `PgUp / PgDn` | Scroll event log |
| `Ctrl+L` | Clear log |
| `Ctrl+C` | Quit |

## TUI slash commands

| Command | Alias | Effect |
|---|---|---|
| `/run <task>` | `/r` | Start a task |
| `/init` | `/i` | `relay init` here |
| `/daemon` | `/d` | Start daemon detached |
| `/handoff` | `/h` | Trigger handoff |
| `/status` | `/s` | Show session status |
| `/providers` | `/p` | Show provider table |
| `/enable <name>` | — | Enable provider |
| `/disable <name>` | — | Disable provider |
| `/audit` | — | `relay audit verify` |
| `/graph` | — | Node / edge counts |
| `/open` | `/o` | Launch `relay-ui` |
| `/clear` | `/cls` | Clear log |
| `/help` | `/?` | Show commands |
| `/exit` | `/q` | Quit |

Bare text (no leading `/`) is treated as `/run <text>`.

## Desktop app shortcuts

| Key | Action |
|---|---|
| `Ctrl/Cmd + K` | Open slash command palette |
| `↑ / ↓` (in palette) | Navigate |
| `Enter` (in palette) | Execute |
| `Esc` (in palette) | Close |

Palette commands mirror the TUI set plus tab-switch verbs (`/diff`, `/contract`, `/cost`).
