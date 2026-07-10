# Getting started

Relay runs as a small daemon plus optional desktop + CLI clients. Five-minute path from `git clone` to a running session.

## Installation

The easiest way to install Relay is via the automated installation scripts. This installs both the `relay` CLI daemon and the `relay-ui` desktop application.

### macOS & Linux

```bash
curl -fsSL https://raw.githubusercontent.com/dbisina/relay/main/scripts/install.sh | bash
```

### Windows

Open PowerShell and run:

```powershell
irm https://raw.githubusercontent.com/dbisina/relay/main/scripts/install.ps1 | iex
```

### Manual Download

You can also download the pre-compiled binaries directly from the [GitHub Releases page](https://github.com/dbisina/relay/releases).

---

## Setting up your project

```bash
cd my-project
relay init
```

Creates `.relay/`:

- `relay.toml` — providers, profiles, vision settings
- `.signing-key` — HMAC key for contract signing (gitignored)
- Empty `audit.jsonl`, `graph.db`

Appends `.gitignore` entries for everything ephemeral.

## Configure providers

Open Settings → Providers in the desktop app, or edit `.relay/relay.toml`. For each provider you want to use:

- **Install** button → opens a terminal running the install command
- **Sign in with browser** → OAuth flow (Claude, Antigravity, GitHub Copilot)
- **Use API key** → paste key, saved to `.relay/.env` (gitignored)
- **Run via Ollama** → bridge mode, uses a local model instead of cloud

You don't need every provider. One is enough to start.

## Run a task

```bash
relay run "add a refund flow to orders service"
```

You'll get a HITL gate:

```
  Task:      add a refund flow to orders service
  Providers: claude → codex → ollama
  Threshold: 85%

Proceed? [y/N]
```

`y` starts the session. Watch progress in the desktop app or the TUI (`relay tui`).

Skip the gate (for app-launched runs): `relay run --yes "..."`.

## The daemon

Relay's daemon is the single source of truth; the desktop app and CLI are both
clients of it. You do not have to start it by hand:

- Opening the desktop app starts the daemon automatically if it isn't already running.
- The daemon is spawned detached, so closing the app leaves it running. Any
  `relay` CLI command keeps working against the same daemon.
- The Windows installer offers a "Start the Relay daemon on login" option, so a
  fresh boot brings the daemon up before you open anything.

To run the daemon explicitly (for example on a headless machine):

```bash
relay daemon           # foreground
```

## Watch what's happening

Three surfaces show the same data:

1. **Desktop app** (`relay-ui`) — dashboard, graph, providers, profiles, settings.
2. **TUI** (`relay tui`) — terminal UI with slash commands. `Ctrl+K` palette.
3. **Audit log** — `.relay/audit.jsonl` for forensic reading.

Live cost meter in the footer. Live diff in the Diff tab. Slash palette via `Ctrl/Cmd+K`.

## When quota hits

Relay watches each provider's usage. At your configured threshold (default 85%) it triggers a handoff:

1. Asks the active provider for a safe pause window.
2. Snapshots the workdir (git commit on the session branch).
3. Builds a continuation contract (Markdown + signed JSON sidecar).
4. Dispatches to the next provider.
5. Waits for receiver heartbeat.
6. Resumes.

You see this as a cinematic overlay in the desktop app and as `handoff` events in the stream.

## What to read next

- [Architecture](architecture.md) — the moving parts in detail
- [Providers](providers.md) — adding a new provider adapter
- [Profiles](profiles.md) — task-kind routing
- [Security](security.md) — secrets, signing, sandboxing
