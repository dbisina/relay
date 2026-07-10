#!/usr/bin/env bash
# scripts/setup.sh — one-shot contributor setup for macOS / Linux.
# Re-run is safe; everything is idempotent.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

bold() { printf "\033[1m%s\033[0m\n" "$*"; }
note() { printf "  %s\n" "$*"; }
ok()   { printf "  \033[32m✓\033[0m %s\n" "$*"; }
err()  { printf "  \033[31m✗\033[0m %s\n" "$*" >&2; }

bold "Relay setup"

# ─── Required tools ──────────────────────────────────────────────────────────

# Root/sudo helper for Linux package managers (brew must not run as root).
SUDO=""
if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then SUDO="sudo"; fi

detect_pm() {
  if command -v brew    >/dev/null 2>&1; then echo brew
  elif command -v apt-get >/dev/null 2>&1; then echo apt
  elif command -v dnf     >/dev/null 2>&1; then echo dnf
  elif command -v pacman  >/dev/null 2>&1; then echo pacman
  elif command -v zypper  >/dev/null 2>&1; then echo zypper
  else echo none; fi
}

pm_install() {
  local pkg="$1"
  # Go's package name differs across distros.
  if [ "$pkg" = "go" ]; then
    case "$(detect_pm)" in
      apt) pkg="golang-go" ;;
      dnf) pkg="golang" ;;
    esac
  fi
  case "$(detect_pm)" in
    brew)   brew install "$pkg" ;;
    apt)    $SUDO apt-get update -y && $SUDO apt-get install -y "$pkg" ;;
    dnf)    $SUDO dnf install -y "$pkg" ;;
    pacman) $SUDO pacman -Sy --noconfirm "$pkg" ;;
    zypper) $SUDO zypper --non-interactive install "$pkg" ;;
    *)      return 1 ;;
  esac
}

install_rust() {
  if command -v rustup >/dev/null 2>&1; then
    rustup default stable || true
  else
    note "Installing Rust via rustup…"
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
  fi
  # shellcheck disable=SC1091
  [ -f "$HOME/.cargo/env" ] && . "$HOME/.cargo/env"
}

# Self-healing dependency check: install what's missing, then re-check.
# `pkg` is the OS package name; cargo is special-cased to rustup.
ensure() {
  local name="$1" pkg="$2" version_cmd="$3"
  if command -v "$name" >/dev/null 2>&1; then
    ok "$name: $($version_cmd 2>&1 | head -1)"; return
  fi
  note "$name not found — attempting auto-install…"
  if [ "$name" = "cargo" ]; then
    install_rust || true
  elif [ "$(detect_pm)" = "none" ]; then
    err "$name missing and no supported package manager found. Install $name and re-run."
    exit 1
  else
    pm_install "$pkg" || true
  fi
  hash -r 2>/dev/null || true
  if command -v "$name" >/dev/null 2>&1; then
    ok "$name installed: $($version_cmd 2>&1 | head -1)"
  else
    err "Could not auto-install $name. Install it manually and re-run (open a new shell for PATH)."
    exit 1
  fi
}

bold "Checking toolchain"
ensure git   git    "git --version"
ensure go    go     "go version"
ensure cargo rustup "cargo --version"

# Node is optional (needed for npm-install of some providers); warn only.
if command -v node >/dev/null 2>&1; then
  ok "node: $(node --version)"
else
  note "node not found — fine for building, but you'll need it to install Claude/Codex CLI later."
fi

# ─── Go side ─────────────────────────────────────────────────────────────────

bold "Pulling Go modules"
( cd packages/daemon-go && go mod download )
ok "go.mod resolved"

bold "Building Go daemon"
( cd packages/daemon-go && go build -o "$REPO_ROOT/bin/relay" ./cmd/relay )
ok "bin/relay"

# ─── Rust side ───────────────────────────────────────────────────────────────

bold "Fetching Rust crates"
( cd packages/ui && cargo fetch )
ok "Cargo.lock resolved"

bold "Building Rust UI"
( cd packages/ui && cargo build --release )
mkdir -p bin
cp target/release/relay-ui bin/ 2>/dev/null || \
  cp packages/ui/target/release/relay-ui bin/ 2>/dev/null || true
ok "bin/relay-ui (release)"

# ─── Smoke test ──────────────────────────────────────────────────────────────

bold "Smoke test"

# Start daemon in background, give it a moment, hit /api/health, kill it.
"$REPO_ROOT/bin/relay" daemon --port 4748 >/dev/null 2>&1 &
DAEMON_PID=$!

sleep 1.5
if curl -fsS http://127.0.0.1:4748/api/health >/dev/null 2>&1; then
  ok "/api/health reachable"
else
  err "daemon failed to start — check binary"
  kill -9 "$DAEMON_PID" 2>/dev/null || true
  exit 1
fi
kill -TERM "$DAEMON_PID" 2>/dev/null || true
wait "$DAEMON_PID" 2>/dev/null || true

# ─── Optional dev tools ──────────────────────────────────────────────────────

bold "Optional dev tools"
if ! command -v air >/dev/null 2>&1; then
  note "Installing air (Go hot-reload)…"
  go install github.com/air-verse/air@latest && ok "air"
fi
if ! cargo install --list 2>/dev/null | grep -q '^cargo-watch '; then
  note "Installing cargo-watch (Rust hot-reload)…"
  cargo install cargo-watch && ok "cargo-watch"
fi

bold "Done."
note "Next:"
note "  ./scripts/dev.sh    # dev loop (hot reload)"
note "  ./bin/relay init    # initialise a project"
note "  ./bin/relay-ui      # launch the desktop app"
