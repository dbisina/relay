#!/bin/bash
# plugins/relay-autopilot/scripts/session-start.sh
#
# SessionStart hook. Decides whether to speak up at all; the actual Q&A and
# relay.toml wiring logic lives in skills/setup/SKILL.md, not here. This
# script's only two jobs: (1) recognise a real software project so we never
# prompt on a notes folder or an essay, (2) remember what the user already
# decided so we never ask twice.

set -u

# --- 1. Read cwd from stdin, falling back to env if parsing fails ----------
input="$(cat)"
cwd="$(printf '%s' "$input" | grep -o '"cwd"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*: *"//; s/"$//')"
if [ -z "$cwd" ]; then
  cwd="${CLAUDE_PROJECT_DIR:-$PWD}"
fi
[ -d "$cwd" ] || exit 0

# --- 2. Walk up to find a project root: .git dir or a known manifest -------
manifests=(package.json go.mod Cargo.toml pyproject.toml pom.xml build.gradle Gemfile composer.json)
dir="$cwd"
root=""
for _ in 1 2 3 4 5 6 7 8; do
  if [ -d "$dir/.git" ]; then
    root="$dir"
    break
  fi
  for m in "${manifests[@]}"; do
    if [ -f "$dir/$m" ]; then
      root="$dir"
      break 2
    fi
  done
  parent="$(dirname "$dir")"
  [ "$parent" = "$dir" ] && break
  dir="$parent"
done

# Not a recognisable software project. Say nothing, ask nothing.
[ -z "$root" ] && exit 0

marker="$root/.relay/autopilot.json"

# --- 3. Respect a prior decision ---------------------------------------------
if [ -f "$marker" ]; then
  if grep -q '"decision"[[:space:]]*:[[:space:]]*"declined"' "$marker" 2>/dev/null; then
    exit 0
  fi
  if grep -q '"decision"[[:space:]]*:[[:space:]]*"configured"' "$marker" 2>/dev/null; then
    chain="$(grep -o '"chain"[[:space:]]*:[[:space:]]*"[^"]*"' "$marker" | head -1 | sed 's/.*: *"//; s/"$//')"
    ctx="Relay autopilot is already configured for this project (chain: ${chain:-see .relay/relay.toml}). Use \`relay run \"<task>\"\` for work that should auto-handoff across agents; don't ask the provider-setup questions again."
    printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":%s}}' "$(printf '%s' "$ctx" | sed 's/\\/\\\\/g; s/"/\\"/g' | sed 's/^/"/; s/$/"/')"
    exit 0
  fi
  # Unknown/corrupt marker content: treat as not-yet-decided rather than loop forever.
fi

# --- 4. First time seeing this project: hand off to the setup skill ---------
ctx='A new software project was detected with no prior relay-autopilot decision recorded (.relay/autopilot.json does not exist at the project root). Before doing anything else the user asked for, load the relay-autopilot:setup skill and follow it now: it asks a short one-time question about whether to configure Relay'"'"'s automatic multi-agent handoff for this project, and if declined, records that decision so this never asks again. Keep it brief; do not block on this if the user'"'"'s first message already makes clear they just want to get straight to work.'
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":%s}}' "$(printf '%s' "$ctx" | sed 's/\\/\\\\/g; s/"/\\"/g' | sed 's/^/"/; s/$/"/')"
exit 0
