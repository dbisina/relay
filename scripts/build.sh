#!/usr/bin/env bash
# scripts/build.sh — release build for all targets.

set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

bold() { printf "\n\033[1m%s\033[0m\n" "$*"; }

mkdir -p dist

bold "Go (release)"
GOFLAGS="-trimpath -ldflags=-s -w" \
  go build -C packages/daemon-go -o "$REPO_ROOT/dist/relay" ./cmd/relay
echo "  → dist/relay"

bold "Rust (release)"
( cd packages/ui && cargo build --release --quiet )
cp packages/ui/target/release/relay-ui dist/ 2>/dev/null || \
  cp target/release/relay-ui dist/
echo "  → dist/relay-ui"

bold "Build artefacts"
ls -lh dist/ | tail -n +2

bold "Done."
