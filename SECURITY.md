# Security policy

## Supported versions

| Version | Supported |
|---|:---:|
| 0.3.x | ✓ |
| 0.2.x | security fixes only until 2026-12 |
| < 0.2 | — |

## Reporting a vulnerability

**Do not open a public GitHub issue.**

Email **danbis664@gmail.com** with:

- Description of the vulnerability
- Affected version(s)
- Steps to reproduce
- Optional: a suggested fix

We acknowledge within 48 hours and aim to ship a fix within 14 days for critical issues. For lower-severity issues, expect 30 days.

We'll credit you in the changelog unless you ask not to be named.

## Threat model

What Relay protects against:

- Tampering with continuation contracts between providers (HMAC-signed).
- Leakage of common secret patterns into provider prompts (redactor).
- Cascading failures from a flapping provider (circuit breaker).
- Loss of user's main branch state (per-session worktree).
- Tampering with the audit log (hash chain).

What Relay does NOT protect against:

- Malicious agents with full user-level system access. Run in a container if this matters.
- Network exfiltration to legitimate provider APIs. We don't proxy or filter that traffic.
- Vendor-side data handling. When you send a prompt to OpenAI, OpenAI's policies apply.

See [docs/security.md](docs/security.md) for the full breakdown.

## Disclosed past issues

None to date.
