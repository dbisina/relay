import * as si from 'simple-icons'
import fs from 'fs'

// Providers whose OFFICIAL mark ships in simple-icons (CC0 icon data; the marks
// themselves remain their owners' trademarks, used here for identification).
const OFFICIAL = {
  claude: 'siClaudecode',
  ollama: 'siOllama',
  copilot: 'siGithubcopilot',
  cline: 'siCline',
  opencode: 'siOpencode',
}

// Brand colors for the marks we cannot ship an official path for. On a dark UI
// pure black is unusable, so a few are lightened deliberately.
const OVERRIDE_HEX = {
  ollama: 'EDEDED',
  copilot: 'D7D7DE',
  opencode: 'E5E5E5',
  cline: 'C9C9D1',
}

const out = {}
for (const [slug, key] of Object.entries(OFFICIAL)) {
  const icon = si[key]
  if (!icon) throw new Error('missing icon ' + key)
  out[slug] = {
    title: icon.title,
    hex: OVERRIDE_HEX[slug] || icon.hex,
    path: icon.path,
  }
}

const header = `// provider-marks.ts — GENERATED, do not edit by hand.
// Source: the simple-icons package (icon data is CC0). Regenerate with
// scripts/gen-provider-marks.mjs. Each mark is the provider's OFFICIAL logo path and
// remains the trademark of its owner; used here solely to identify the provider.
//
// Providers absent from this map have no official mark published in
// simple-icons (OpenAI withdrew theirs; Antigravity and Continue have none).
// Those fall back to a lettermark in brand color, which is honest identification
// rather than an invented logo.

export interface ProviderMark {
  title: string
  hex: string
  path: string
}

export const PROVIDER_MARKS: Record<string, ProviderMark> = ${JSON.stringify(out, null, 2)}
`
fs.writeFileSync(process.argv[2], header)
console.log('wrote', Object.keys(out).length, 'official marks ->', process.argv[2])
