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

# packages/ui (eframe + rfd) needs GTK3, xkbcommon, and XCB dev headers on Linux.
if [ "$(uname -s)" = "Linux" ]; then
  if command -v apt-get >/dev/null 2>&1; then
    note "Installing Linux GUI build deps (GTK3, xkbcommon, XCB)…"
    if $SUDO apt-get update -y >/dev/null 2>&1 \
      && $SUDO apt-get install -y libgtk-3-dev libxkbcommon-dev \
           libxcb-render0-dev libxcb-shape0-dev libxcb-xfixes0-dev >/dev/null 2>&1; then
      ok "GUI build deps installed"
    else
      note "Could not install GUI build deps automatically; the UI build may fail without them."
      note "Install manually: sudo apt-get install libgtk-3-dev libxkbcommon-dev libxcb-render0-dev libxcb-shape0-dev libxcb-xfixes0-dev"
    fi
  else
    note "Non-apt distro detected: if the UI build fails, install your distro's GTK3, xkbcommon, and XCB dev packages"
    note "(for example on dnf: gtk3-devel libxkbcommon-devel libxcb-devel)."
  fi
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

# Start the daemon in the background, poll /api/health, kill it.
# Use a non-default port: Relay leaves a daemon running by design, so an
# already-running daemon on 4748 would answer for the freshly built binary
# and turn a bind failure into a false pass.
SMOKE_PORT=4799

"$REPO_ROOT/bin/relay" daemon --port "$SMOKE_PORT" >/dev/null 2>&1 &
DAEMON_PID=$!

HEALTH_OK=false
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fsS "http://127.0.0.1:${SMOKE_PORT}/api/health" >/dev/null 2>&1; then
    HEALTH_OK=true
    break
  fi
  sleep 0.5
done

if [ "$HEALTH_OK" = true ]; then
  ok "/api/health reachable on port ${SMOKE_PORT}"
else
  err "daemon failed to start on port ${SMOKE_PORT}: check the binary"
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
