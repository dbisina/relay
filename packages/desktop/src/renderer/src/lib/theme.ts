// theme.ts — accent customization. The neutral ramp (backgrounds, text,
// borders) is fixed by the design system; the one committed accent color is
// the single thing a user can retheme, applied as a runtime CSS variable
// override on :root so every token consumer (buttons, chips, badges) updates
// together without touching component code.

export interface AccentPreset {
  id: string
  label: string
  accent: string
  accentHi: string
  accentInk: string
}

export const ACCENT_PRESETS: AccentPreset[] = [
  { id: 'orange', label: 'Orange', accent: 'oklch(0.685 0.16 47)', accentHi: 'oklch(0.75 0.15 52)', accentInk: 'oklch(0.2 0.03 55)' },
  { id: 'blue', label: 'Blue', accent: 'oklch(0.66 0.14 245)', accentHi: 'oklch(0.72 0.13 248)', accentInk: 'oklch(0.15 0.03 250)' },
  { id: 'green', label: 'Green', accent: 'oklch(0.7 0.14 158)', accentHi: 'oklch(0.76 0.13 160)', accentInk: 'oklch(0.14 0.03 158)' },
  { id: 'violet', label: 'Violet', accent: 'oklch(0.66 0.16 300)', accentHi: 'oklch(0.72 0.15 300)', accentInk: 'oklch(0.16 0.04 300)' },
  { id: 'rose', label: 'Rose', accent: 'oklch(0.68 0.17 15)', accentHi: 'oklch(0.74 0.16 18)', accentInk: 'oklch(0.16 0.03 15)' },
]

const STORAGE_KEY = 'relay.accent'

export function applyAccent(id: string): void {
  const p = ACCENT_PRESETS.find((x) => x.id === id) ?? ACCENT_PRESETS[0]
  const root = document.documentElement.style
  root.setProperty('--accent', p.accent)
  root.setProperty('--accent-hi', p.accentHi)
  root.setProperty('--accent-ink', p.accentInk)
  root.setProperty('--accent-weak', `color-mix(in oklab, ${p.accent} 14%, transparent)`)
  root.setProperty('--accent-line', `color-mix(in oklab, ${p.accent} 40%, transparent)`)
}

/** Call once at startup: applies the saved accent (or default) before first paint. */
export function loadSavedAccent(): string {
  let id = 'orange'
  try {
    id = localStorage.getItem(STORAGE_KEY) || 'orange'
  } catch {
    /* ignore */
  }
  applyAccent(id)
  return id
}

export function saveAccent(id: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, id)
  } catch {
    /* ignore */
  }
  applyAccent(id)
}
