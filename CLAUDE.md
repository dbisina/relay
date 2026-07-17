# CLAUDE.md

Rules for any AI agent (Claude Code, Codex, OpenCode, Cline, Continue, etc.) working on this repository. Read `CODEMAP.md` for layout. Read `AGENTS.md` for the same rules in a slightly different register if Claude Code isn't the agent loading this.

## Identity

This is **Relay**: a vendor-neutral orchestrator that rotates a single coding task across multiple AI agents. The code that orchestrates is itself code we're improving. Mind the meta.

## Goals before changes

Before any file write:

1. **Read `CODEMAP.md`** to locate the relevant area.
2. **Read the surrounding 200 lines** of the file you're about to edit.
3. **Confirm the change matches existing patterns** in that package. Relay has a deliberate style.

## Style: Go

- `gofmt` formatted, no exceptions.
- Names: `Foo` not `IFoo`. Capitalised function for exported, lowercase for package-private.
- No `panic()` in library code. Return error.
- Channels for events; don't poll if a push exists.
- Comments above non-trivial functions explain *why*, not *what*. Code says what.
- Package-level docs at the top of every file (`// internal/foo/foo.go: what this file does`).
- Public functions in a package: docstring required. Private: only if non-obvious.

## Style: Rust

- `cargo fmt` formatted.
- Pass `cargo clippy -- -D warnings`.
- No `unwrap()` in non-test code unless infallible (e.g. allocation).
- All draw functions: `fn draw_<thing>(ui: &mut Ui, ...)`. Keep them short, extract sub-draws.
- HTTP only via `crate::api::send_*` helpers: never call `ureq` inside draw functions.
- Egui state that needs to outlive one frame: stash in `ui.ctx().data_mut(|m| m.insert_temp(id, value))`. Don't add fields to `RelayApp` unless cross-frame mutation is required.

## Architecture rules

- The Go daemon is the single source of truth. UIs render `/api/*` data.
- Adapters normalise to `AgentEvent`. Don't leak provider-specific types into the orchestrator.
- Every cross-boundary string passes through `internal/redact`. Don't bypass.
- New endpoints: handler in `server/server.go`, callback registered in `main.go` or `orchestrator.go`, DTO mirrored in `packages/ui/src/types.rs`.
- New TOML keys: parser arm in `config.go`, default in `Default()`, doc in `relay.toml` template.

## What NOT to do

- **Do not** commit secrets. The redactor catches the obvious ones: don't rely on it.
- **Do not** add panic recovery to hide bugs. Surface them.
- **Do not** add network calls outside `internal/{adapter,server,vision}`.
- **Do not** introduce a new heavy dependency. PR description must justify any new direct dep.
- **Do not** modify `.relay/.signing-key` handling without a security review note in PR.
- **Do not** disable the worktree check unless explicitly authorised: agents losing the user's branch is a P0.
- **Do not** introduce em dashes (Unicode U+2014) into user-facing copy. Use commas, colons, periods. (Style guide enforced in copy.)

## Specific subsystems

### Adding a provider

Follow the five-step recipe in `docs/providers.md`. Touch exactly these files:
1. `internal/adapter/interface.go`: name const
2. `internal/adapter/<name>.go`: adapter
3. `internal/adapter/registry.go`: register
4. `cmd/relay/providers.go`: metadata + probe
5. `internal/pricing/pricing.go`: pricing

### Adding a slash command (TUI)

`cmd/relay/tui.go`:
- Add entry to `slashCommands` table
- Add `case "/yourcmd":` in `executeCommand`
- Update `docs/cli-reference.md`

### Adding a slash command (desktop)

`packages/ui/src/app.rs` `draw_slash_palette`:
- Add tuple to `commands` array
- Add `id` arm in `run_palette_action`

### Touching the contract format

`internal/contract/`:
- Bump `Version` field on the struct
- Add migration in `Serializer.Serialize` if necessary
- Update `docs/architecture.md` schema example
- Add a test fixture under `internal/contract/testdata/`

### Touching the FSM

`internal/fsm/`:
- Only via `HandoffMachine` methods. Don't mutate state directly.
- Every transition must be durable (write to `.relay/fsm.json`).
- Add a property test if you add a new state.

## Testing

- Unit tests live next to code: `<file>_test.go`.
- Adapter event-parsing tests use recorded fixtures under `internal/adapter/testdata/`.
- Integration: spin up daemon, hit `/api/health`, kill. See `scripts/setup.sh` for the pattern.
- `relay eval` runs golden routing cases. Update `.relay/eval/tasks.json` when changing the matcher.

## Commit messages

Imperative mood. Under 72 chars. Reference issues. Sign if you can.

```
Add diff viewer to dashboard

Reads git diff main...relay/<sid> via /api/session/diff.
Renders +/- colourized lines in a new MainTab variant.

Closes #142
```

## When you're stuck

1. Read `CODEMAP.md` again. The answer is probably there.
2. Read `docs/architecture.md` for the moving parts.
3. Search for an existing similar pattern in the same package.
4. If you can't find a precedent, ask the user before inventing one.

## Final rule

If your change touches >5 files or >300 lines in one go, **stop and check in with the user**. Relay values incremental changes over heroic refactors. The continuation contract format is itself proof of the design value of small, signed, reviewable steps. Extend the same discipline to the code that builds it.
