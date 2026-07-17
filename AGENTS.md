# AGENTS.md

Instructions for AI coding agents (Codex, OpenCode, Cline, Continue, Cursor, etc.) working on this repository.

Agent-neutral mirror of `CLAUDE.md`. If you're Claude Code, prefer `CLAUDE.md`. Other agents: this is yours.

## What this repo is

Relay: Go daemon + Rust desktop app + Go TUI that rotates one coding task across multiple AI agents. You may be running *inside* Relay right now (orchestrated by it). If so, your output will be parsed and contributed to the next agent's context. Be precise.

## Read first

1. **`CODEMAP.md`**: repo layout, package responsibilities, key control-flow paths.
2. **`docs/architecture.md`**: the moving parts in detail.
3. **The 200 lines around any file you're about to edit.**

Do not write before you have these three contexts.

## Working rules

| Rule | Why |
|---|---|
| `gofmt` + `golangci-lint` pass | CI fails otherwise |
| `cargo fmt` + `cargo clippy -- -D warnings` pass | CI fails otherwise |
| No new direct dependencies without justification | Supply-chain budget |
| Every cross-boundary string runs through `internal/redact` | Privacy guarantee |
| Worktree-aware: write to the session worktree only | User's main branch is sacred |
| No `panic()` / `unwrap()` in non-test code | Surface errors |
| Commit messages: imperative, under 72 chars | Repo convention |

## Hot paths to know

- Adding a provider → 5 files (`docs/providers.md` has the recipe)
- Adding an API endpoint → 4 files (handler, callback wire, Rust DTO, Rust send-helper)
- Adding a CLI command → 2 files (new file in `cmd/relay/`, register in `main.go`)
- Adding a TUI slash command → 1 file (`cmd/relay/tui.go`)

## When unsure

Ask before inventing. Search for an existing similar pattern in the same package first. Relay has deliberate, consistent style.

## Style

- No em dashes in user-facing copy. Use commas, periods, colons.
- Public function: docstring required.
- Package file header: `// internal/foo/foo.go: what this file does`.
- Names: `Foo`, `FooBuilder`, never `IFoo`/`AbstractFoo`.

## Critical do-nots

- Do not modify `internal/contract/` signing flow without security review note in PR.
- Do not bypass worktree isolation.
- Do not commit `.relay/.env` or `.relay/.signing-key` (they are gitignored).
- Do not introduce panic recovery to hide bugs.
- Do not add network calls outside `internal/{adapter,server,vision}`.

## Done definition

A change is done when:

1. Code compiles on all CI targets (`go build ./...`, `cargo build --release`).
2. Tests pass (`./scripts/test.sh`).
3. `relay eval` passes if you touched routing.
4. User-facing changes noted in `docs/changelog.md` under `## Unreleased`.
5. API additions noted in `docs/api-reference.md`.
6. CLI additions noted in `docs/cli-reference.md`.
7. `CLAUDE.md` / `AGENTS.md` / `CODEMAP.md` updated if architecture shifted.

## Big-ticket rule

If your change touches more than 5 files or 300 lines, stop and ask before continuing. Incremental beats heroic. Relay's value proposition (signed continuation contracts, small reviewable steps) applies to the code that builds it.
