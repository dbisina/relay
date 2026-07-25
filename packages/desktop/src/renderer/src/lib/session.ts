// session.ts — deriving honest, human-readable identity from a DetectedAgent.
//
// Verified against a live daemon (19 real sessions): `initialPrompt` is NOT a
// usable title. In practice it is always harness boilerplate, either
// "Base directory for this skill: C:\..." or a run of slash commands
// ("/caveman/compress/graphify-windows/..."), which is byte-identical across
// unrelated sessions. Using it as a title would render every row the same.
//
// What IS reliable:
//   workDir      always present, and its basename is the project the work is in
//   lastPrompt   often the real user intent, but sometimes harness noise
//   filesTouched / messageCount / tokens / model  reliably populated
//   plan / tasksRemaining  frequently EMPTY, never rely on them

import type { DetectedAgent } from './types'

/** Last path segment of a Windows or POSIX path. */
export function baseName(p: string): string {
  if (!p) return ''
  const parts = p.split(/[\\/]/).filter(Boolean)
  return parts[parts.length - 1] || p
}

// Text that is the harness talking, not the user. Rejected as intent.
const BOILERPLATE = [
  /^base directory for this skill:/i,
  /^\s*\/[a-z0-9-]+(\s*\/[a-z0-9-]+)+/i, // "/caveman/compress/graphify-..."
  /^\[request interrupted/i,
  /^this session is being continued from/i,
  /^<system-reminder/i,
  /^caveat: the messages below were generated/i,
  /^<local-command/i,
  /^\s*$/,
]

function isBoilerplate(text: string): boolean {
  const t = (text || '').trim()
  if (t.length < 3) return true
  return BOILERPLATE.some((re) => re.test(t))
}

/** Collapse whitespace and trim to a single readable line. */
function oneLine(text: string, max = 160): string {
  const t = (text || '').replace(/\s+/g, ' ').trim()
  return t.length > max ? `${t.slice(0, max - 1)}…` : t
}

/**
 * The best available one-line description of what this session was doing.
 * Returns '' when nothing trustworthy exists, callers must handle that rather
 * than substituting invented text.
 */
export function sessionIntent(agent: DetectedAgent): string {
  const s = agent.session
  if (!s) return ''
  for (const candidate of [s.lastPrompt, s.initialPrompt]) {
    if (candidate && !isBoilerplate(candidate)) return oneLine(candidate)
  }
  return ''
}

/** The project a session is working in. Always real, used as primary identity. */
export function sessionProject(agent: DetectedAgent): string {
  return baseName(agent.workDir) || agent.displayName || agent.provider
}

/** Newest first, by the daemon's real epoch-ms lastActive. */
export function byRecency(a: DetectedAgent, b: DetectedAgent): number {
  return (b.lastActive || 0) - (a.lastActive || 0)
}

/**
 * How much context a handoff would have to carry. This is the number that
 * makes a handoff feel real rather than magical, so it is computed only from
 * fields that are actually populated.
 */
export function contextWeight(agent: DetectedAgent): {
  messages: number
  files: number
  tokens: number
  skills: number
  mcps: number
} {
  const s = agent.session
  return {
    messages: s?.messageCount ?? 0,
    files: s?.filesTouched?.length ?? 0,
    tokens: (s?.tokensIn ?? 0) + (s?.tokensOut ?? 0),
    skills: s?.skills?.length ?? 0,
    mcps: s?.mcps?.length ?? 0,
  }
}
