---
name: setup
description: One-time per-project setup wizard for Relay's automatic multi-agent handoff. Triggered by the relay-autopilot SessionStart hook on new software projects, or invoked manually as /relay-autopilot:setup to reconfigure.
---

# Relay autopilot setup

You're seeing this because either the SessionStart hook detected a software project with no recorded Relay decision yet (`.relay/autopilot.json` doesn't exist), or the user manually ran `/relay-autopilot:setup`. Your job: ask two short questions, then either wire up Relay or record that the user doesn't want it, so this never asks again unless they explicitly re-run the command.

Keep this brief. It's a one-time setup, not a conversation. Don't explain Relay at length, don't editorialize.

## Step 1: opt in or out

Ask, using AskUserQuestion:

> "This project doesn't have Relay set up yet. Relay hands a coding task off between AI agents automatically when one hits its usage limit, so a task keeps running instead of stopping. Want it configured for this project?"

Options:
- **Yes, set it up** — proceed to Step 2.
- **No, and don't ask again** — write the decline marker (see below) and stop. Say nothing further beyond a one-line acknowledgement.
- **Not now** — do NOT write any marker file. Just continue with whatever the user actually asked for this session; the hook will ask again next session.

**Decline marker** (only for "No, and don't ask again"):

Write `.relay/autopilot.json` at the project root (create `.relay/` if it doesn't exist yet):

```json
{
  "decision": "declined",
  "decidedAt": "<current ISO 8601 timestamp>"
}
```

## Step 2: which providers

Only reached if the user chose "Yes" in Step 1.

First check whether the `relay` binary is actually installed (`command -v relay` on macOS/Linux, `where relay` on Windows, or just try `relay --version`). If it's missing, tell the user in one line how to install it:

```bash
curl -fsSL https://raw.githubusercontent.com/dbisina/relay/main/scripts/install.sh | bash   # macOS/Linux
irm https://raw.githubusercontent.com/dbisina/relay/main/scripts/install.ps1 | iex          # Windows
```

Then stop without writing any marker (so the hook asks again once it's installed; don't record "declined" for a missing binary, that's not a decision).

If `relay` is installed, ask (AskUserQuestion, multiSelect: true):

> "Which of these do you actually have access to and want in the rotation? (Pick as many as apply.)"

Options, matching Relay's real built-in adapters (see `docs/providers.md` for detail if you need it):
- **Claude** — Claude Code CLI, OAuth or API key.
- **Codex** — OpenAI Codex CLI, API key.
- **Antigravity** — Google's coding IDE, manual auth.
- **OpenCode** — open-source terminal agent, API key.
- **Ollama** — local models, no cloud account needed.
- **Copilot** — GitHub Copilot via the `gh` extension.
- **Continue** — VS Code extension.
- **Cline** — VS Code extension.

If they pick zero, treat it as "Claude" only (Relay's own fallback default) rather than erroring or re-asking.

## Step 3: wire it up

1. If `.relay/` doesn't exist in this project yet, run `relay init` in the project root first.
2. Open `.relay/relay.toml`. For each of the 8 provider blocks (`[providers.claude]`, `[providers.codex]`, `[providers.antigravity]`, `[providers.opencode]`, `[providers.ollama]`, `[providers.copilot]`, `[providers.continue]`, `[providers.cline]`), set `enabled = true` for ones the user picked and `enabled = false` for the rest. Don't touch any other key in those blocks (`declared_cap`, `model`, `base_url` etc. already have sensible defaults from `relay init`).
3. The provider *order* Relay actually uses is a fixed built-in priority (`claude, codex, antigravity, opencode, ollama, copilot, continue, cline`), filtered down to whichever are enabled — you don't set order in `relay.toml` directly. If the user wants a specific order or per-task-kind routing (e.g. "route refactors to Claude, tests to Codex"), that's a `[profiles.*]` block with an explicit `chain = [...]`; point them at `docs/profiles.md` rather than building one yourself unless they ask for it explicitly.
4. Write the configured marker at `.relay/autopilot.json`:

```json
{
  "decision": "configured",
  "chain": "<enabled providers in Relay's default order, joined with ' -> '>",
  "configuredAt": "<current ISO 8601 timestamp>"
}
```

5. Tell the user, in one or two lines: it's configured, the chain that resulted, and that `relay run "<task>"` will now auto-handoff across that chain until the task finishes or every account's quota is exhausted (Relay's default `MaxHandoffs` is unlimited; a quota-breach handoff to the next provider is the natural stopping condition, not a fixed hop count). Mention they can change providers anytime by editing `.relay/relay.toml` or re-running `/relay-autopilot:setup`.

6. If the user's original message for this session already described a real coding task (not just idle chat), offer to kick it off immediately: `relay run "<their task>"`. Don't force this, just offer it as the natural next step.

## Notes for future edits to this skill

- This skill is the only thing that should write `.relay/autopilot.json`. Don't let other flows touch it.
- Never overwrite an existing `.relay/autopilot.json` silently. If this skill is invoked manually (`/relay-autopilot:setup`) on an already-configured project, that's an explicit reconfigure request, treat it as such (fine to overwrite), just make sure it's clear to the user that's what's happening.
- Keep the two-question shape. If you're tempted to add a third question (threshold, retry policy, etc.), don't, those have sensible defaults and are one `relay.toml` edit away for anyone who cares. The whole point is this stays fast enough to survive running on every new project.
