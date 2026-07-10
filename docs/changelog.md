# Changelog

All notable changes per release. Follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [SemVer](https://semver.org).

## Unreleased

### Added

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
