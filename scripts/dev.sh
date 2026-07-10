#!/usr/bin/env bash
# scripts/dev.sh — dev loop with hot reload.
# Starts daemon + UI in parallel; Ctrl+C cleans both up.

set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

trap 'kill 0' INT TERM EXIT

# Air watches Go files and rebuilds the daemon on change.
( cd packages/daemon-go && \
  air -c "$REPO_ROOT/.air.toml" -- daemon 2>&1 | sed 's/^/[daemon] /' ) &

# cargo-watch reruns relay-ui on src change.
( cd packages/ui && \
  cargo watch -x 'run --quiet' 2>&1 | sed 's/^/[ui]     /' ) &

# Re-run eval when profiles change so routing regressions surface fast.
if command -v entr >/dev/null 2>&1; then
  ( find .relay/relay.toml | entr -r ./bin/relay eval 2>&1 | sed 's/^/[eval]   /' ) &
fi

wait
