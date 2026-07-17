#!/usr/bin/env bash
# scripts/install.sh — install Relay from GitHub Releases.
# Usage: curl -fsSL https://raw.githubusercontent.com/dbisina/relay/main/scripts/install.sh | bash
#
# Installs both `relay` (daemon + CLI + TUI) and `relay-ui` (desktop app).

set -euo pipefail

REPO="dbisina/relay"
INSTALL_DIR="/usr/local/bin"
NEEDS_SUDO=false

bold()  { printf "\033[1m%s\033[0m\n" "$*"; }
green() { printf "\033[32m%s\033[0m\n" "$*"; }
red()   { printf "\033[31m%s\033[0m\n" "$*"; }

# ── Detect OS + arch ──────────────────────────────────────────────────────────

detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "$os" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      red "Unsupported OS: $os"; exit 1 ;;
    esac

    case "$arch" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)            red "Unsupported architecture: $arch"; exit 1 ;;
    esac

    TARGET="${OS}-${ARCH}"
}

# ── Find latest release ───────────────────────────────────────────────────────

no_release_help() {
    red "No release found for ${REPO}."
    red "Either no releases have been published yet, or the GitHub API request failed (rate limit / network)."
    echo ""
    bold "Build from source instead:"
    bold "  git clone https://github.com/dbisina/relay && cd relay && ./scripts/setup.sh"
    echo ""
    bold "Or pin a specific version and re-run:"
    bold "  RELAY_VERSION=v0.1.0 bash install.sh"
}

find_latest_version() {
    # RELAY_VERSION pins a version and skips the GitHub API query entirely.
    if [ -n "${RELAY_VERSION:-}" ]; then
        VERSION="$RELAY_VERSION"
        return
    fi

    local response
    if ! response="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null)"; then
        no_release_help
        exit 1
    fi

    VERSION="$(printf '%s' "$response" | grep '"tag_name"' | head -1 \
        | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/' || true)"

    if [ -z "$VERSION" ]; then
        no_release_help
        exit 1
    fi
}

# ── Install ───────────────────────────────────────────────────────────────────

install() {
    detect_platform
    find_latest_version

    bold ""
    bold "  Relay installer"
    bold "  Version:  ${VERSION}"
    bold "  Platform: ${TARGET}"
    bold ""

    local url="https://github.com/${REPO}/releases/download/${VERSION}/relay-${VERSION}-${TARGET}.tar.gz"
    local checksums_url="https://github.com/${REPO}/releases/download/${VERSION}/SHA256SUMS"
    local tmpdir
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    bold "Downloading..."
    curl -fsSL "$url" -o "${tmpdir}/relay.tar.gz"
    curl -fsSL "$checksums_url" -o "${tmpdir}/SHA256SUMS"

    bold "Verifying checksum..."
    (cd "$tmpdir" && grep "relay-${VERSION}-${TARGET}.tar.gz" SHA256SUMS | sha256sum -c --quiet 2>/dev/null) || {
        # macOS uses shasum
        local expected
        expected="$(grep "relay-${VERSION}-${TARGET}.tar.gz" "${tmpdir}/SHA256SUMS" | awk '{print $1}')"
        local actual
        actual="$(shasum -a 256 "${tmpdir}/relay.tar.gz" | awk '{print $1}')"
        if [ "$expected" != "$actual" ]; then
            red "Checksum mismatch!"
            exit 1
        fi
    }
    green "  ✓ Checksum verified"

    bold "Extracting..."
    tar xzf "${tmpdir}/relay.tar.gz" -C "$tmpdir"

    # Determine install location
    if [ ! -w "$INSTALL_DIR" ]; then
        if command -v sudo >/dev/null 2>&1; then
            NEEDS_SUDO=true
        else
            INSTALL_DIR="$HOME/.local/bin"
            mkdir -p "$INSTALL_DIR"
        fi
    fi

    bold "Installing to ${INSTALL_DIR}..."
    if [ "$NEEDS_SUDO" = true ]; then
        sudo install -m 755 "${tmpdir}/relay" "${INSTALL_DIR}/relay"
        sudo install -m 755 "${tmpdir}/relay-ui" "${INSTALL_DIR}/relay-ui"
    else
        install -m 755 "${tmpdir}/relay" "${INSTALL_DIR}/relay"
        install -m 755 "${tmpdir}/relay-ui" "${INSTALL_DIR}/relay-ui"
    fi

    green ""
    green "  ✓ relay installed to ${INSTALL_DIR}/relay"
    green "  ✓ relay-ui installed to ${INSTALL_DIR}/relay-ui"
    green ""

    # Check PATH
    if ! echo "$PATH" | tr ':' '\n' | grep -q "^${INSTALL_DIR}$"; then
        bold "  Note: ${INSTALL_DIR} is not in your PATH."
        bold "  Add it:  export PATH=\"${INSTALL_DIR}:\$PATH\""
    fi

    bold ""
    bold "  Get started:"
    bold "    cd your-project"
    bold "    relay init"
    bold "    relay run \"your task here\""
    bold ""
    bold "  Or launch the desktop app:  relay-ui"
    bold "  Docs: https://github.com/${REPO}#readme"
    bold ""
}

install
