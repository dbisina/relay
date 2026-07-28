# Terminal UI

The `relay` TUI is a full-screen terminal cockpit for running and watching a
handoff without leaving your shell. It talks to the same daemon as the desktop
app, so anything you do in one shows up in the other.

If you like living in a terminal, this is the fastest way to use Relay.

## Start it

```bash
relay tui
```

`relay interactive` and `relay shell` do the same thing, and plain `relay` with
no arguments opens the TUI when you are in a real terminal. It runs in the
alternate screen buffer, so your scrollback is untouched when you quit.

The TUI starts its own daemon if one is not already running, so you do not need
to start `relay daemon` first.

## What you are looking at

From top to bottom:

- **Title bar** shows `relay agent orchestrator` and the version.
- **Event log** fills the middle. Every provider probe, handoff, quota warning,
  and line of agent output streams here as it happens, newest at the bottom.
- **Status bar** shows the active session: task id, current provider, tokens
  used, the FSM state, and the handoff fitness score.
- **Input line** at the bottom is where you type.
- **Hint line** reminds you that `/` opens commands.

When you press `/`, a command popup appears just above the input with the
matching commands and their descriptions.

## The one thing to know

Type a task and press Enter. That is the whole happy path.

```
add a search endpoint to the orders service
```

Anything you type that does not start with `/` is treated as a task to run, the
same as `/run <task>`. Relay routes it to a profile, starts the first agent, and
you watch the work stream in the log. When that agent nears its limit, the
handoff happens in front of you and the status bar switches to the next
provider.

## Commands

Press `/` to open the command popup, then keep typing to filter, `Tab` to cycle,
and `Enter` to run. Or just type the whole command and press Enter.

| Command | Alias | Effect |
|---|---|---|
| `/run <task>` | `/r` | Start a task |
| `/init` | `/i` | Set up Relay in the current folder |
| `/daemon` | `/d` | Start the daemon detached |
| `/handoff` | `/h` | Hand off now, before the limit |
| `/status` | `/s` | Show the current session |
| `/providers` | `/p` | Show the provider table |
| `/enable <name>` | | Turn a provider on |
| `/disable <name>` | | Turn a provider off |
| `/audit` | | Verify the audit log |
| `/graph` | | Node and edge counts |
| `/open` | `/o` | Launch the desktop app |
| `/banner` | | Reprint the banner |
| `/clear` | `/cls` | Clear the log |
| `/help` | `/?` | List commands |
| `/exit` | `/q` | Quit |

## Keys

| Key | Action |
|---|---|
| `/` | Open the command popup |
| `Tab` | Cycle matches in the popup |
| `Enter` | Run the command, or select the highlighted item |
| `Esc` | Close the popup |
| `Up` / `Down` | Command history when the popup is closed, navigation when it is open |
| `PgUp` / `PgDn` | Scroll the event log |
| `Ctrl+L` | Clear the log |
| `Ctrl+C` | Quit |

## A first run, end to end

```bash
cd my-project
relay tui
```

Then, inside the TUI:

1. `/init` once, to create `.relay/` in this project.
2. `/providers` to see which agents Relay found and which are ready.
3. `/enable claude` and `/enable codex`, or whichever you have.
4. Type your task and press Enter.
5. Watch. When you see a quota warning, either wait for the automatic handoff or
   press `/handoff` to move now.

To switch to the desktop app at any point, run `/open`. It attaches to the same
running daemon, so the session you are watching carries straight over.

## When something looks wrong

- **No providers listed:** run `/providers`. If it is empty, none were detected.
  Install an agent CLI (for example `npm i -g @anthropic-ai/claude-code`) and
  sign in, then reopen the TUI.
- **A task will not start:** check the status bar. If a session is already
  running, Relay runs one at a time by design. Let it finish or quit and
  restart.
- **The log stops updating:** the daemon may have stopped. Run `/daemon` to
  start it again, or quit and reopen.

## See also

- [CLI reference](cli-reference.md) for every `relay` subcommand and flag.
- [Getting started](getting-started.md) for install and first-run setup.
- [Providers](providers.md) for how each agent is detected and authenticated.
