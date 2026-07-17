# Changelog

All notable changes per release. Follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [SemVer](https://semver.org).

## Unreleased

### Added

- **Agent detection** (`internal/detect` + `relay detect`) — discover AI coding agents already running on the machine (process scan + JSONL / JSON / VS Code SQLite session stores) and lift each session's intent: prompt, plan, tasks left, files, skills, MCPs, token usage. Providers: Claude Code, Codex, Copilot (CLI + VS Code), Cursor, Cline, Continue, Antigravity.
- **Adopt & port** — `relay detect --adopt <id> --target <provider> --start` (or `POST /api/detect/adopt`) renders a detected session into a continuation brief and resumes it on another agent.
- **Ambient mode** (`cmd/relay/ambient.go`) — the daemon polls session stores in the background and announces new agent sessions as they appear.
- **Continuation contract v2** — rich session intent: `initialPrompt`, `plan`, `tasksRemaining`, `skills*`, `inFlightCode`. v1 contracts still serialize byte-for-byte (migration guard).
- **Accounts** — multiple logins per provider (label + isolated config dir), account-aware handoff, and `POST /api/providers/account{,/add,/remove,/login}` + in-UI account manager.
- **Quota wallet** (`GET /api/quota/wallet`) — per provider+account remaining quota, reset time, burn rate, and time-to-exhaustion ETA.
- **Predictive handoff** — hands off at a safe point *before* exhaustion using the wallet's burn-rate forecast.
- **Wait-vs-handoff engine** (`internal/retry`) — classifies usage-limit / overload / safeguard signals, parses reset times, and decides whether waiting beats burning a handoff. `[retry]` config in `relay.toml`.
- **Pipelines** — multi-agent DAGs (`.relay/pipelines.json`, desktop designer, `POST /api/pipelines/run`): per-node provider, `dependsOn` ordering, `fallback` providers, `verify` acceptance commands.
- **Verifier gate** (`internal/verify`) — acceptance commands run between handoffs and after pipeline nodes; failures block acceptance.
- **Time machine** (`GET /api/history*`) — handoff timeline from the audit log, snapshot commits, per-commit diff, and non-destructive rewind (new branch at the snapshot).
- **Project code graph endpoint** — `GET /api/graph/project?path=` one-shot codegraph scan for the desktop Graph page.
- **Daemon autostart** — the desktop app starts the daemon detached if it isn't running and leaves it up for the CLI.
- **MCP server** — `relay mcp` exposes the HTTP API as Model Context Protocol tools over stdio. Wire Relay into Claude Desktop, Cursor, Cline, Continue, Zed, or any MCP-aware LLM client. 11 tools: status / providers / run_task / handoff / retrieve / diff / cost / send_reply / list_profiles / pause / events.
- **Codebase graph builder** (`internal/codegraph`) — regex-based scanner for Go, Rust, TS/JS, Python, Java/Kotlin, Ruby. Runs once at session start, indexes symbols + modules + imports into the same SQLite graph as decisions/constraints/files. Skips vendor/node_modules/target/build/.git/.idea/.vscode.
- **FTS5 chunk retrieval** — new `chunks` + `chunks_fts` tables in `internal/graph`. `GraphStore.UpsertChunk/SearchChunks`. New endpoint `GET /api/retrieval?q=&limit=` for context-economic LLM lookups.
- **`CODEMAP.md`** — compact repo navigation guide for contributing agents.
- **`CLAUDE.md` + `AGENTS.md`** — agent rules at repo root.
- Per-session git worktree (`internal/worktree`) — agents never touch user's working tree.
- Secret redactor (`internal/redact`) with 12 patterns for common credentials.
- Per-provider circuit breaker (`internal/circuit`) — closed / open / half-open lifecycle.
- Live cost meter using `internal/pricing` — per-million-token rates per provider, displayed in dashboard footer.
- Outcome tracker (`internal/outcomes`) — JSONL of session results, success-rate boost feeds back into profile routing.
- Approval gate primitive (`internal/approval`) — generic gate for HITL on risky actions.
- Continuation contract verification on resume — signed JSON sidecar (`envelope.json`) prevents tamper.
- Vision cloud opt-in — Gemini / OpenAI / Anthropic refuse to send screenshots unless `enabled=true`.
- Inline pause via `POST /api/session/pause` — agents halt at next event boundary.
- Diff tab in dashboard with `git diff main...relay/<sid>` rendered with `+/-` colouring.
- Slash command palette via Ctrl/Cmd+K with live filter and arrow nav.
- Approval bar — top-of-screen toast for pending requests.
- `relay eval` golden-task routing regression harness.
- Lenient vision-response parsing (snake_case + camelCase) to handle Ollama / cloud-model inconsistency.
- Fallback to `npx --yes <pkg>` when CLI not yet in PATH after install (Windows post-`npm i -g` race fix).

- **GitHub Releases** — Automated cross-platform packaging (`linux`, `darwin`, `windows` for `amd64` and `arm64`) on tag push via GitHub Actions.
- **Install Scripts** — Added `curl` and `irm` automated installation scripts for UNIX and Windows platforms.
- **Demo Recording** — Added `scripts/record-demo.sh` for automating terminal UI recordings using asciinema.
- **Community Templates** — Added YAML issue forms, PR checklists, and `CODE_OF_CONDUCT.md`.

### Changed

- **README** — Complete overhaul for clarity, featuring visual architecture diagrams, installation one-liners, and a feature grid.
- `selectNextProvider` is now cost-aware — picks cheapest healthy provider when multiple eligible.
- Stdin reply handler set by orchestrator during session, restored to stub after exit.
- Vision tab Model field replaced with installed/curated picker when provider = ollama.

### Fixed

- **P0: handoff snapshots targeted the user's checkout.** The durability manager was bound to the pre-worktree working dir, so `Snapshot`/`EmergencySnapshot` committed or stashed the user's own WIP instead of the agent's changes. It now follows the per-session worktree; regression test included.
- Stale `.relay/session.json` no longer poisons later sessions: terminal or foreign-session records are discarded on load, and clean exits remove the file. (Previously a session ending in ERROR broke every subsequent handoff in that repo.)
- Handoff no longer leaks the outgoing adapter's event-pump goroutine and un-reaped child process; the event channel is drained (redacted + audited) until the adapter closes it.
- A provider registered in the adapter registry but missing from the quota registry no longer nil-panics the orchestrator (falls back to no-op quota tracking); a cross-registry consistency test makes the miss a test failure.
- TOML parser: `#` inside quoted values no longer truncates them; string arrays no longer shatter on commas inside quoted elements.
- Daemon config is no longer mutated while HTTP handler goroutines read it (copy-on-write config pointer); `activeProvider` reads from server callbacks are synchronized.
- UI poll thread's daemon autostart now spawns detached (previously tied the daemon to the UI's process group, contradicting the leave-running design).
- Full-sidebar nav icons rendered blank: the list passed unicode glyphs that matched no `paint_icon` arm. Pipeline/Wallet/History also no longer reuse other pages' icons.
- `setup.ps1` failed to parse on PowerShell 5.1 (em dashes in a BOM-less file decode as a string-terminating smart quote under ANSI).
- `install.sh`/`install.ps1` died silently when no GitHub release exists; they now explain and offer the build-from-source path, and accept `RELAY_VERSION` to pin.
- Contributor smoke test could false-pass against an already-running daemon on 4748; it now uses port 4799 with a health poll.
- Provider card hover did not trip pointer cursor on Direction A icon rail.
- Settings panel was empty due to a `SidePanel` nested inside `horizontal_centered`.
- Profile chain reorder buttons rendered as `□` boxes on systems missing geometric Unicode glyphs — replaced with painted icons.

## 0.3.0 — 2026-05-15

### Added

- Profiles for task-kind routing.
- Vision fallback subsystem (screenshot + multimodal LLM).
- CLAUDE.md / AGENTS.md / Cursor rules / `.relay/instructions.md` discovery + injection.
- Ollama bridge: pull models, launch providers via Ollama.

### Changed

- Provider page rewritten with bespoke layout (no more identical cards).
- Icons painted from SVG paths rather than rendered as font glyphs.

## 0.2.0 — 2026-04-22

### Added

- Bubble Tea TUI with slash commands.
- Auto-install and OAuth flows per provider.
- Per-provider API key entry → `.relay/.env`.

## 0.1.0 — 2026-03-10

Initial release. Multi-provider orchestration, handoff state machine, hash-chained audit log, continuation contracts.
