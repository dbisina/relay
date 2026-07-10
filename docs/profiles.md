# Profiles & routing

Profiles map task kinds + skills to ordered provider chains. The orchestrator picks the best-matching profile at session start and uses its chain instead of the global fallback.

## Anatomy

```toml
[profiles.backend]
chain        = ["claude", "codex", "ollama"]
kinds        = ["api", "service", "refactor", "test"]
skills       = ["go", "ts", "rust"]
context_hint = "production backend services"
```

- **chain** — providers in handoff priority. First entry is the primary.
- **kinds** — keywords scored 3× for matching against the task goal.
- **skills** — keywords scored 1× (drop `"any"` if you don't want it to match everything).
- **context_hint** — free text. Each space-separated word ≥4 chars scores 1×.

## Matching

```
score("backend", "add refund flow to orders service") =
  contains("api")        → +3
  contains("service")    → +3
  contains("refactor")   → 0
  contains("test")       → 0
  contains("go")         → 0
  contains("backend")    → 0   (hint word, but not in goal)
  contains("services")   → +1  (hint word)
  total: 7
```

Best score wins. Tie-break by alphabetical order. Score 0 → no match, falls back to the global `provider_priority` order.

## Outcome boost

After enough sessions, the matcher factors in historical success rate from `.relay/outcomes.jsonl`:

```
boosted = baseScore × (0.5 + successRate)
```

So a profile that historically lands 80% of its tasks gets `× 1.3`; one at 20% gets `× 0.7`. Brand-new profiles default to 0.5 success (neutral).

## Editing in the app

Profiles page in `relay-ui`:

- Cards show chain (▲▼ reorder, ✕ remove)
- `+ Add provider` dropdown lists available providers (filtered to enabled)
- Kind / skill pills (display-only; edit by hand in `.relay/relay.toml` for now)
- `Delete` button removes the section
- `+ New profile` creates an empty one

Writes back to `.relay/relay.toml` immediately; daemon reloads.

## Editing the TOML

```toml
[profiles.frontend]
chain        = ["claude", "codex"]
kinds        = ["component", "style", "a11y", "react"]
skills       = ["react", "css", "tailwind"]
context_hint = "user interface and accessibility"

[profiles.database]
chain        = ["codex", "claude"]
kinds        = ["migration", "query", "schema"]
skills       = ["sql", "postgres", "sqlite"]
context_hint = "schema design and migrations"

[profiles.reviewer]
chain        = ["claude"]
kinds        = ["review"]
skills       = ["any"]
context_hint = "code review only — no writes"
```

## Cost-aware tie-break

When multiple providers in the chain are eligible (quota OK, circuit closed), the orchestrator picks the **cheapest** by blended price:

```
blend = inputPerMTokens + 3 × outputPerMTokens
```

So even if Claude is first in your chain, Ollama (zero cost) will win when both are eligible. Pin order: list cheaper providers earlier in `chain`.

## Defaults shipped

`relay init` writes four profiles:

- `backend` — Go/TS services
- `frontend` — React/CSS UI work
- `database` — schema + queries (Codex first since it's strong at SQL)
- `reviewer` — Claude-only review pass

Edit, replace, delete — the orchestrator only cares about what's in the file.

## Eval harness

Validate your routing assumptions:

```bash
# .relay/eval/tasks.json
[
  {"task": "add refund flow to orders service",
   "expectProfile": "backend",
   "expectChainHead": "claude",
   "mustIncludeFiles": ["CLAUDE.md"]},
  {"task": "migrate users table to add email_verified column",
   "expectProfile": "database",
   "expectChainHead": "codex"}
]

# Run
relay eval
```

Exits non-zero on regression. Wire into CI.
