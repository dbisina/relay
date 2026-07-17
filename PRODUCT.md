# Product

## Register

product

## Users

Developers who run more than one AI coding agent (Claude Code, Codex, Copilot, Cursor, Cline, Continue, Antigravity, OpenCode) because relying on a single subscription is expensive or limiting. They're technical, comfortable in a terminal, skeptical of vendor lock-in, and evaluating this tool during a work break or late at night, deciding in under a minute whether it's real engineering or another wrapper. The desktop app and TUI are the day-to-day surface; the docs site and README are the surface that earns the first minute of trust.

## Product Purpose

Relay is a vendor-neutral orchestrator that keeps a coding task alive across multiple AI agents. When the active agent hits its usage limit, Relay pauses it at a safe point, signs a continuation contract carrying intent, plan, and in-flight code, and resumes a different agent, account, or provider from that exact point. Success looks like: no more manual copy-paste handoffs, no more losing context when a subscription runs dry, and the confidence that comes from a signed, auditable protocol rather than a fragile shell script.

## Brand Personality

Technical, direct, unshowy. Confidence earned through precision, not hype: signed contracts, hash-chained audit logs, an FSM with named states, not "AI magic." Voice reads like a senior engineer's own README, not marketing copy. Dry humor is fine; exclamation points and growth-hacker energy are not.

## Anti-references

Generic SaaS AI-tool landing pages: the hero-metric template (big number, small label, gradient underneath), a centered icon-title-subtitle card grid, gradient text, glassmorphism decoration, gratuous emoji, gushing "supercharge your workflow" copy. Also avoid looking like a crypto/Web3 project (neon-on-black, drenched saturated color) or an enterprise SaaS (navy and beige, stock photography of people pointing at whiteboards).

## Design Principles

- **Show the mechanism, don't just claim it.** A real terminal transcript, a real state diagram, a real signed-contract JSON snippet beats an adjective every time.
- **Earn the accent color.** Orange (#e06a38) is the one committed brand color; it marks the thing that matters (the handoff, the CTA, a status), not decoration.
- **Precision over polish-for-its-own-sake.** Correct information (real repo links, real install commands, real feature list) matters more than any visual flourish sitting on top of stale or wrong content.
- **Developer-tool typography, not editorial.** Type carries the voice through code, diagrams, and structure, not through display serifs or italics.
- **No filler copy.** Every sentence states a fact about the system. No restated headings, no throat-clearing intros.

## Accessibility & Inclusion

Dark theme by default (matches the desktop app and CLI). Respect `prefers-reduced-motion`. Sufficient contrast for body text against the dark base (already using OKLCH lightness steps). No color-only signal, every status also carries an icon or text label.
