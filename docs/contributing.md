# Contributing

Welcome. This page covers code style, the dev loop, tests, and how to add a provider.

## Setup

One command:

```bash
./scripts/setup.sh        # macOS/Linux
./scripts/setup.ps1       # Windows
```

It will:

1. Check / install Go 1.23+, Rust 1.75+, Node 18+.
2. Pull all submodules.
3. `go mod download`, `cargo fetch`.
4. Build both binaries to `./bin/`.
5. Run a smoke test (start daemon, GET /api/health, kill).

Manual setup is also fine — see [getting-started.md](getting-started.md).

## Dev loop

```bash
./scripts/dev.sh
```

Runs three things in parallel with hot-reload:

- `go run ./packages/daemon-go/cmd/relay daemon` (watched by `air`)
- `cargo watch -x run` for `relay-ui`
- A small task-runner that re-runs `relay eval` whenever you edit a profile

Stop with Ctrl+C — kills all three cleanly.

## Tests

```bash
./scripts/test.sh
```

- `go test ./...` for the Go side
- `cargo test` for the Rust UI
- `relay eval --json` for routing regressions

CI runs the same in `.github/workflows/ci.yml`. Tests must pass before merge.

## Code style

### Go

- `gofmt` enforced. CI fails on diff.
- `golangci-lint run` — clean.
- Files end with newline.
- No `panic()` in library code; return error.
- All interfaces named `Foo` not `IFoo`.
- Stream events through channels; don't poll unless explicitly part of the API.

### Rust

- `cargo fmt`. CI fails on diff.
- `cargo clippy -- -D warnings` — clean.
- No `unwrap()` in non-test code unless impossible-to-fail (allocation, etc.).
- egui draw functions are `fn draw_<thing>(ui: &mut Ui, ...)`. Keep them short; extract sub-draws.
- Use `crate::api::send_*` helpers — never call `ureq` from inside draw functions.

### Commits

- Imperative mood. "Add diff viewer", not "Added".
- Subject ≤ 72 chars. Body wrapped at 72.
- Reference the issue: `Closes #142`.
- Sign your commits if you have a GPG key (`git commit -S`).

## Adding a provider

See [providers.md](providers.md). Five files touched:

1. `internal/adapter/interface.go` — add `ProviderName` const.
2. `internal/adapter/yourtool.go` — implement `AdapterContract`.
3. `internal/adapter/registry.go` — register.
4. `cmd/relay/providers.go` — metadata + probe arm.
5. `internal/pricing/pricing.go` — pricing.

Add a unit test for your `Run` event parsing using a recorded fixture.

## Adding a UI page

In `packages/ui/src/app.rs`:

1. Add a variant to `NavPage`.
2. Add an icon to `paint_icon` (16×16 viewBox, SVG-derived path).
3. Add a row to `IconRail` / `FullSidebar` items.
4. Write `draw_yourpage(ui, state)`.
5. Add a `match` arm in `draw_central`.

If the page needs state, mirror the existing pattern: stash via `ui.ctx().data_mut(|m| m.insert_temp(id, value))`. Don't add fields to `RelayApp` unless they need to outlive a single frame.

## PR checklist

- [ ] Code compiles on macOS, Linux, Windows
- [ ] `go test ./...` and `cargo test` pass
- [ ] `relay eval` passes against shipped golden tasks
- [ ] New API endpoints documented in `docs/api-reference.md`
- [ ] New CLI commands documented in `docs/cli-reference.md`
- [ ] User-facing changes noted in `docs/changelog.md`
- [ ] No new dependencies without a note in PR description explaining why
- [ ] No `panic` / `unwrap` in non-test code
- [ ] No `println!` / `fmt.Println` in non-debug paths — use the event log or a logger

## Areas welcoming contributions

Tagged on GitHub with `good first issue`:

- Additional secret patterns for the redactor
- More vision-model providers
- Per-provider quota source improvements
- Windows-specific install fixes
- Test recordings for adapter event streams
- Performance tuning of the graph store

Big-ticket roadmap items in [docs/architecture.md](architecture.md).

## Code of conduct

Be respectful. We follow the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).

## License

Contributions are licensed under Apache-2.0, the same as the project.
