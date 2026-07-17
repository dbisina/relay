# Vision fallback

Some providers (Continue, Cline, Antigravity, GitHub Copilot Chat) run inside an IDE and don't expose a structured event stream. For these, Relay can fall back to **screenshot-based observation**: periodically capture the target window, send it to a multimodal LLM, parse the response into a structured observation.

This is off by default and privacy-sensitive. Read this page before enabling.

## How it works

```
┌──────────────────────────────────────────────────────────┐
│ Vision loop (every PollMs):                              │
│  1. screenshot.CaptureRect(primary display)              │
│  2. base64 PNG                                           │
│  3. POST to vision model with prompt:                    │
│     "Is the agent asking the user? Return JSON:          │
│      {needsInput: bool, question, choices, summary}"     │
│  4. Parse response                                       │
│  5. If needsInput → emit 'waiting' event with Q+choices  │
│     Else → emit 'text' event with summary (deduped)      │
└──────────────────────────────────────────────────────────┘
```

The poll runs as a goroutine inside the orchestrator, cancelled when the active adapter exits.

## Configuration

```toml
[vision]
enabled       = false                                    # opt-in
provider      = "ollama"                                 # ollama | gemini | openai | anthropic
model         = "qwen2.5vl:7b"
api_key_env   = ""                                       # GEMINI_API_KEY / OPENAI_API_KEY / ANTHROPIC_API_KEY
base_url      = "http://localhost:11434"
poll_ms       = 2500
window_match  = "(?i)continue|cline|antigravity|copilot"
```

Or use **Settings → Vision** in the desktop app.

## Vision providers

| Provider | Models | Auth | Cost |
|---|---|---|---|
| **Ollama** (default) | `qwen2.5vl:7b`, `llava`, `llama3.2-vision`, `moondream`, `minicpm-v`, `gemma3` | none (local) | free |
| Google Gemini | `gemini-1.5-pro`, `gemini-2.0-flash` | `GEMINI_API_KEY` | per token |
| OpenAI | `gpt-4o`, `gpt-4o-mini` | `OPENAI_API_KEY` | per token |
| Anthropic | `claude-3-5-sonnet-...` | `ANTHROPIC_API_KEY` | per token |

Cloud providers refuse to run unless `enabled = true` is explicitly set. Default is silence.

## Recommended Ollama models

Curated list in `relay-ui` → Settings → Vision → Pull Vision Model:

- `qwen2.5vl:7b`: strong general vision + UI reading. Good default.
- `qwen2.5vl:3b`: lighter, faster on CPU.
- `llava:7b` / `llava:13b`: classic open vision.
- `llama3.2-vision:11b`: Meta's contribution.
- `moondream:1.8b`: tiny + fast.
- `minicpm-v:8b`: strong OCR.
- `gemma3:4b`: Google's compact multimodal.

Pull with the **[Pull]** button on each card. Streams progress to the dashboard event log.

## Probing once

Test your setup without committing to a poll loop. Click **Probe now** in the Vision tab. The daemon:

1. Captures the primary display.
2. Calls the configured model.
3. Returns one observation.

UI renders summary, question, choices, raw text. Useful for tuning prompts and verifying the model can read your IDE.

## Privacy

Cloud vision = sending screenshots offsite. They contain whatever is on your screen, including code, secrets, personal data. Specifically:

- Local Ollama is **safe by default**: nothing leaves your machine.
- Gemini / OpenAI / Anthropic send screenshots to those vendors. Their data policies apply.
- The redactor does **not** scrub image content. It only scrubs text.

Recommended: stay on Ollama unless you've explicitly chosen otherwise for a specific task.

## What it does today

- Captures screen, sends to model, parses, emits events. ✓
- Decision LLM step (choose answer from contract): not yet.
- Robotic input (click button, type answer): not yet, deliberately. We don't ship desktop automation by default; see [security.md](security.md).

So today, vision is **read-only**. It tells the user "the agent is asking for X"; the user replies via the inline reply bar (which routes to the active adapter's stdin if it supports it).

## Future

- Decision step: feed observation + continuation contract to a reasoner model → pick an answer.
- Cross-platform OS automation via [robotgo](https://github.com/go-vgo/robotgo) or [enigo-rs](https://github.com/enigo-rs/enigo): opt-in per session, full user prompt before activation.
- macOS screen-recording permission flow (and detection that it's missing).
- Wayland support (X11 only today on Linux).
