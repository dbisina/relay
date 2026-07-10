#!/usr/bin/env bash
# scripts/test.sh — run the full test matrix.

set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

bold() { printf "\n\033[1m%s\033[0m\n" "$*"; }

bold "Go tests"
( cd packages/daemon-go && go test ./... )

bold "Go lint"
if command -v golangci-lint >/dev/null 2>&1; then
  ( cd packages/daemon-go && golangci-lint run )
else
  echo "  (golangci-lint not installed — skipping)"
fi

bold "Rust tests"
( cd packages/ui && cargo test --quiet )

bold "Rust clippy"
( cd packages/ui && cargo clippy --quiet -- -D warnings )

bold "Eval (golden routing)"
if [ -f .relay/eval/tasks.json ]; then
  ./bin/relay eval
else
  echo "  (.relay/eval/tasks.json not present — skipping)"
fi

bold "All clean."
