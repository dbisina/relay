# Contributing to Relay

Thanks for helping build Relay. This file is the short version; the full guide
lives in [docs/contributing.md](docs/contributing.md).

## Quick start

One command sets up the whole toolchain (Go, Rust, git) and builds both sides.
It is self-healing: missing tools are installed automatically where a package
manager is available (winget on Windows, Homebrew on macOS, apt/dnf/pacman/zypper
on Linux).

```bash
# macOS / Linux
./scripts/setup.sh

# Windows (PowerShell)
./scripts/setup.ps1
```

Then:

```bash
./bin/relay init          # scaffold .relay/ in a project
./bin/relay-ui            # launch the desktop app (starts the daemon for you)
```

## Ground rules

- Read [CODEMAP.md](CODEMAP.md) before touching source. It tells you where things live.
- Go: `gofmt` clean, `go vet` clean, no `panic()` in library code.
- Rust: `cargo fmt` clean, `cargo clippy -- -D warnings` clean, no `unwrap()` in non-test code.
- Keep changes small. A single change touching more than five files or three
  hundred lines should be split or discussed first.
- Every cross-boundary string passes through `internal/redact`. Never commit secrets.
- Tests live next to the code they cover (`<file>_test.go`, Rust `#[cfg(test)]`).

## Before you open a PR

```bash
cd packages/daemon-go && go build ./... && go vet ./... && go test ./... && gofmt -l .
cd ../.. && cargo build -p relay-ui && cargo clippy -p relay-ui
```

All of the above must be clean. See the [pull request template](.github/PULL_REQUEST_TEMPLATE.md)
for the checklist.

## License

By contributing you agree that your contributions are licensed under the
[Apache-2.0 License](LICENSE).
