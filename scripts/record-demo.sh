#!/usr/bin/env bash
# scripts/record-demo.sh — record a demo GIF of Relay in action.
# Requires: ffmpeg, relay (built), a test project.
#
# Usage: ./scripts/record-demo.sh [output.gif]
#
# This script:
# 1. Starts the Relay daemon
# 2. Runs a sample task
# 3. Captures terminal output (VHS preferred; script/asciinema fallback)
# 4. Converts to GIF using ffmpeg or agg
#
# For the desktop UI demo, run relay-ui manually and use OBS/ScreenToGif.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT="${1:-${REPO_ROOT}/docs/assets/demo.gif}"
DEMO_DIR=$(mktemp -d)
trap 'rm -rf "$DEMO_DIR"' EXIT

bold() { printf "\033[1m%s\033[0m\n" "$*"; }

# Check tools
for cmd in ffmpeg; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Error: $cmd is required. Install it and re-run."
        exit 1
    fi
done

# Preferred path: VHS renders a clean, deterministic GIF from scripts/demo.tape.
# It is the canonical demo definition; everything below is a fallback.
if command -v vhs >/dev/null 2>&1; then
    bold "Rendering demo via VHS (scripts/demo.tape)..."
    ( cd "$REPO_ROOT" && vhs scripts/demo.tape )
    if [ -f "$REPO_ROOT/docs/assets/demo.gif" ]; then
        bold "GIF written to docs/assets/demo.gif"
        ls -lh "$REPO_ROOT/docs/assets/demo.gif"
        exit 0
    fi
    bold "VHS ran but produced no GIF — check ttyd/ffmpeg. Falling back."
fi

bold "Setting up demo project..."
mkdir -p "$DEMO_DIR/demo-project"
cd "$DEMO_DIR/demo-project"
git init -q
echo '# Demo' > README.md
git add . && git commit -q -m "init"

bold "Initialising Relay..."
"$REPO_ROOT/dist/relay" init 2>/dev/null || "$REPO_ROOT/bin/relay" init 2>/dev/null || relay init

bold "Recording terminal session..."
# Use script command to capture terminal output
RECORDING="$DEMO_DIR/recording.txt"

if command -v asciinema >/dev/null 2>&1; then
    # Best option: asciinema + agg
    CAST="$DEMO_DIR/demo.cast"
    asciinema rec "$CAST" -c "bash -c '
        echo \"$ relay run \\\"add error handling to the API routes\\\"\"
        sleep 1
        relay run \"add error handling to the API routes\" --yes 2>&1 | head -50
        sleep 2
    '" --cols 100 --rows 30 --overwrite

    if command -v agg >/dev/null 2>&1; then
        agg "$CAST" "$OUTPUT" --theme monokai --speed 2
    else
        bold "asciinema recorded to $CAST"
        bold "Install agg (https://github.com/asciinema/agg) to convert to GIF"
        bold "  agg $CAST $OUTPUT --theme monokai --speed 2"
    fi
else
    # Fallback: script + ffmpeg
    bold "Tip: install asciinema + agg for better results"
    bold "  brew install asciinema && cargo install agg"

    script -q "$RECORDING" -c "bash -c '
        echo \"$ relay run \\\"add error handling to the API routes\\\"\"
        sleep 1
        relay run \"add error handling to the API routes\" --yes 2>&1 | head -50
        sleep 2
    '" || true

    bold "Terminal output saved to $RECORDING"
    bold "Convert manually using ScreenToGif, Peek, or similar tool"
fi

bold ""
bold "Demo recording complete!"
if [ -f "$OUTPUT" ]; then
    bold "  GIF saved to: $OUTPUT"
    ls -lh "$OUTPUT"
fi
