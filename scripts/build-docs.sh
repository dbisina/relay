#!/usr/bin/env bash
# scripts/build-docs.sh — convert docs/*.md → docs/site/*.html
# Uses pandoc with a custom HTML template that wraps each doc in our shell.

set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v pandoc >/dev/null 2>&1; then
  echo "pandoc not installed. Install with:"
  echo "  macOS:   brew install pandoc"
  echo "  Debian:  sudo apt install pandoc"
  echo "  Windows: choco install pandoc"
  exit 1
fi

TEMPLATE="$REPO_ROOT/docs/site/_template.html"
SRC_DIR="$REPO_ROOT/docs"
OUT_DIR="$REPO_ROOT/docs/site"

# Pages to convert
PAGES=(
  "architecture"
  "providers"
  "profiles"
  "vision"
  "security"
  "api-reference"
  "cli-reference"
  "mcp"
  "contributing"
  "changelog"
)

for page in "${PAGES[@]}"; do
  src="$SRC_DIR/$page.md"
  out="$OUT_DIR/$page.html"
  if [ ! -f "$src" ]; then continue; fi
  pandoc "$src" \
    --from gfm \
    --to html5 \
    --template "$TEMPLATE" \
    --metadata title="$(head -1 "$src" | sed 's/^#\s*//')" \
    --metadata pagename="$page" \
    --output "$out"
  echo "  → $out"
done

echo "Built ${#PAGES[@]} pages → docs/site/"
