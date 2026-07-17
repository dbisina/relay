# MCP server

Relay runs as a Model Context Protocol server via `relay mcp`. Any MCP-aware LLM client can use it as a tool source. Examples: **Claude Desktop**, **Cursor**, **Cline**, **Continue**, **Zed**, **Codex**.

This means you don't need to leave your existing LLM client to drive Relay. Just give your LLM the `relay_*` tools and prompt accordingly.

## Why

Three use cases:

1. **You like your current LLM client.** Add Relay alongside; let the LLM call `relay_handoff` when it hits a limit.
2. **You're already using Claude Desktop / Cursor.** Wire Relay in; ask "use Codex for the refactor and Claude for the review" in plain prose.
3. **You want a programmable orchestrator.** Build agents that call the MCP tools to compose multi-provider workflows.

## Install

Relay must be in your PATH. The daemon must be running (`relay daemon` or just open `relay-ui`).

## Client config

### Claude Desktop

`~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)
`%APPDATA%\Claude\claude_desktop_config.json` (Windows)

```json
{
  "mcpServers": {
    "relay": {
      "command": "relay",
      "args": ["mcp"]
    }
  }
}
```

Restart Claude Desktop. Look for the Relay hammer icon.

### Cursor

Cursor settings → MCP → Add server:

```json
{
  "name": "relay",
  "command": "relay",
  "args": ["mcp"]
}
```

### Cline / Continue

Both support MCP server configs. Same shape as above.

### Any client supporting MCP stdio

```
command: relay
args:    ["mcp"]
```

The server speaks JSON-RPC 2.0 line-delimited over stdio. Spec: https://modelcontextprotocol.io

## Tools

Each tool is a thin wrapper over a Relay HTTP endpoint.

| Tool | Maps to | Use when |
|---|---|---|
| `relay_status` | GET /api/status | LLM asks "is a Relay session running?" |
| `relay_providers` | GET /api/providers | LLM picks which provider to hand off to |
| `relay_run_task` | POST /api/run `{task, threshold}` | LLM wants to start a Relay task |
| `relay_handoff` | POST /api/handoff | LLM is about to exhaust its quota |
| `relay_retrieve` | GET /api/retrieval?q=&limit= | LLM needs code context (top-K snippets) |
| `relay_diff` | GET /api/session/diff | LLM wants to know what the agent has changed |
| `relay_cost` | GET /api/session/cost | LLM checks budget before suggesting expensive work |
| `relay_send_reply` | POST /api/session/reply | LLM forwards user input to the active adapter |
| `relay_list_profiles` | GET /api/profiles | LLM examines routing rules |
| `relay_pause` | POST /api/session/pause | LLM pauses the agent to deliberate |
| `relay_events` | GET /api/events?since= | LLM streams recent activity for context |

## Example prompts

> "Use `relay_retrieve` to find anything about idempotency in this codebase, then propose a refund flow."

> "When you reach 80% of your context budget, call `relay_handoff` and have Codex finish."

> "Read `relay_diff` and tell me what risk this change carries."

> "Call `relay_status`. If a session is running, append to it; otherwise call `relay_run_task` with the goal I just described."

## Failure mode

If the daemon isn't running, every tool returns:

> daemon not reachable on http://127.0.0.1:4748: start it with `relay daemon`

Open the desktop app (`relay-ui`) or run `relay daemon` in a terminal first.

## Security

The MCP server is a thin pipe. It only calls the loopback API on `127.0.0.1:4748`. No new network surface, no new credentials.

But: an MCP client effectively gets every Relay capability. Treat tool-grant permissions as "give this LLM control of my Relay session". The LLM can:

- Start tasks (`relay_run_task`)
- Force handoffs (`relay_handoff`)
- Pause/resume (`relay_pause`)
- Send arbitrary stdin to the active adapter (`relay_send_reply`)

If your client supports per-tool approvals, lean on them. Claude Desktop, for example, prompts before each tool call by default.
