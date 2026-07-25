// format.ts — small display helpers. No dependencies.

export function pct(fraction: number): string {
  return `${Math.round(Math.max(0, Math.min(1, fraction)) * 100)}%`
}

export function compact(n: number): string {
  if (n == null || !isFinite(n)) return '0'
  const abs = Math.abs(n)
  if (abs >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (abs >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return `${Math.round(n)}`
}

export function relTime(unixSecondsOrIso: number | string | null | undefined): string {
  if (unixSecondsOrIso == null) return ''
  const ms =
    typeof unixSecondsOrIso === 'number'
      ? unixSecondsOrIso * (unixSecondsOrIso < 1e12 ? 1000 : 1)
      : Date.parse(unixSecondsOrIso)
  if (!isFinite(ms)) return typeof unixSecondsOrIso === 'string' ? unixSecondsOrIso : ''
  const diff = Date.now() - ms
  const s = Math.round(diff / 1000)
  if (s < 0) return 'just now'
  if (s < 60) return `${s}s ago`
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.round(h / 24)
  return `${d}d ago`
}

export function etaLabel(minutes: number): string {
  if (!isFinite(minutes) || minutes <= 0) return 'steady'
  if (minutes < 1) return '<1 min'
  if (minutes < 60) return `~${Math.round(minutes)} min`
  const h = minutes / 60
  if (h < 24) return `~${h.toFixed(1)} h`
  return `~${Math.round(h / 24)} d`
}

export type EventTone = 'accent' | 'green' | 'red' | 'blue' | 'neutral'

/** Shared history/timeline event -> badge tone mapping (History screen, Home sessions list). */
export function eventTone(ev: string | undefined): EventTone {
  const e = (ev || '').toLowerCase()
  if (e.includes('handoff')) return 'accent'
  if (e.includes('complete') || e.includes('done')) return 'green'
  if (e.includes('error') || e.includes('fail')) return 'red'
  if (e.includes('start') || e.includes('run')) return 'blue'
  return 'neutral'
}

/** Human title for a provider slug: "opencode" -> "OpenCode". */
const PROVIDER_TITLES: Record<string, string> = {
  claude: 'Claude',
  codex: 'Codex',
  copilot: 'Copilot',
  antigravity: 'Antigravity',
  opencode: 'OpenCode',
  ollama: 'Ollama',
  continue: 'Continue',
  cline: 'Cline',
  cursor: 'Cursor',
  gemini: 'Gemini',
}
export function providerTitle(slug: string): string {
  return PROVIDER_TITLES[slug?.toLowerCase()] ?? (slug ? slug[0].toUpperCase() + slug.slice(1) : slug)
}
