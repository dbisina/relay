# Security & privacy

What we do, what we don't do, what's on you.

## What Relay guarantees

### Per-session git worktree

Every session runs in `<repo>/.relay/sessions/<sid>/` on a dedicated branch `relay/<sid>`. Agents never write to your main working tree.

- If git is installed and the project is a repo → worktree created automatically.
- You opt in to merge or discard at session end. Default: keep the branch around for review.
- If git is not present → falls back to direct workdir writes. You'll see a `system` event saying so.

### Secret redaction

Every event content + meta passes through a redactor with twelve patterns before reaching the audit log, UI, or another provider. Detected kinds:

- `openai_key`, `anthropic_key`, `gemini_key`, `github_pat`, `slack_token`, `stripe_key`
- `aws_access_key`, `aws_session_key`
- `jwt`
- `pem_block`, `ssh_private_key`
- `url_password` (preserves URL structure, redacts only credentials)
- `auth_bearer` (Authorization headers)
- `env_secret_pair` (KEY=value with sensitive-named keys)

Matches replaced with `[REDACTED:<kind>]`. End-of-session summary logs counts: `redactions during session: openai_key:2, env_secret_pair:1`.

### Audit log integrity

`.relay/audit.jsonl` is hash-chained:

```
hash_n = SHA-256(hash_{n-1} + canonical(event_n))
```

`relay audit verify` walks the chain and exits non-zero on any tampering. Used in incident response and compliance reviews.

### Signed continuation contracts

Every contract is HMAC-SHA256 signed using `.relay/.signing-key` (32 bytes, generated at `relay init`, gitignored). On resume:

1. Orchestrator reads `envelope.json` sidecar.
2. Calls `Builder.Verify(c)`: constant-time HMAC compare.
3. Tamper → logs `SECURITY: envelope signature invalid` to audit, refuses to inject, rebuilds fresh.

If `envelope.json` is missing (legacy or hand-crafted) the `.md` is accepted with a warning event.

### Circuit breakers

Per-provider circuit breakers prevent cascading failures from a flapping API. State transitions are observable via `GET /api/circuit`.

### Approval gate

Programmatic gate (`internal/approval`) lets the orchestrator block at boundaries (large diffs, shell commands not on an allowlist) until a user resolves via `POST /api/approvals/<id>`. UI shows a yellow toast bar across the top. Default timeout 5 minutes → deny.

### Vision opt-in

Cloud vision providers (Gemini, OpenAI, Anthropic) refuse to send screenshots unless `vision.enabled = true` is explicitly set. Local Ollama always works. Privacy default is silent.

## What Relay does NOT guarantee

### Agent sandboxing

Agents run as subprocesses with full user permissions. They can read any file the user can read. They can write to the worktree (which they should) but also to anywhere else they're told to.

Mitigations:
- Worktree isolation per session means main branch is safe.
- Approval gate can intercept large diffs and non-allowlist shell commands when wired (the primitive ships; per-adapter wiring is ongoing).
- Run with `--no-ui` in a container if you want hard isolation.

Roadmap: rootless container per session.

### Network egress control

Adapters call upstream provider APIs directly. We don't proxy or filter that traffic except for the Claude quota proxy (which exists to read rate-limit headers, not to gate egress).

If you need egress control, run Relay inside a network namespace that constrains where agents can talk.

### Code execution review

When an agent decides to run `rm -rf` or `curl … | sh`, Relay shows it to you in the event stream. It doesn't pre-screen execution. The approval gate primitive exists for this; wiring it to every shell-exec site is an ongoing task.

### Vendor-side data handling

When you send a prompt to OpenAI, that prompt is governed by OpenAI's data policy. Same for any other cloud provider. Relay can't change that.

## Settings → Security tab

| Setting | Effect |
|---|---|
| HITL gate | Require confirmation before each cross-provider handoff |
| Data isolation | Never transmit one vendor's raw output to another vendor (filters meta keys) |
| At-rest encryption | Marker only: implementation underway |
| Audit log | Status of `.relay/audit.jsonl` chain |

## Secrets storage

`.relay/.env` (mode 0600) holds API keys you save via the desktop app's "Use API key" flow. Format:

```
ANTHROPIC_API_KEY="sk-ant-..."
OPENAI_API_KEY="sk-..."
GEMINI_API_KEY="AIza..."
```

Auto-appended to `.gitignore`. Loaded into `os.Environ()` at daemon start so child adapter processes inherit. **Never** sent to providers as part of prompts (redactor catches them on the way out).

Roadmap: OS keyring storage (`go-keyring`) as an option for users who don't want file-based secrets.

## Reporting a vulnerability

Email security@relay.dev with details. Don't open a public issue.

See [SECURITY.md](../SECURITY.md) at the repo root for the disclosure process.
