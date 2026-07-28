// components/onboarding/StepMainProvider.tsx — step 3. Which tool leads the
// work, which of its models, and one plain line per tool about what it is good
// at. The lines are saved with the rotation profile in the next step, so the
// router has something to match on besides provider order.

import React, { useEffect } from 'react'
import { useStore } from '../../lib/store'
import { providerTitle } from '../../lib/format'
import { Input, ProviderGlyph } from '../ui'
import { StepHead, Note, SelectRow, selectStyle, type SetupDraft } from './wizard'

/** Starting points for the "good at" line, so the step arrives already filled in. */
const HINT_SUGGESTIONS: Record<string, string> = {
  claude: 'Long refactors, tricky bugs, and work that needs the whole picture',
  codex: 'Quick edits, tests, and small self-contained changes',
  copilot: 'Small changes and boilerplate inside the editor',
  gemini: 'Reading a lot of code at once and summarising it',
  antigravity: 'Finding your way around an unfamiliar project',
  ollama: 'Simple work offline, without spending a paid account',
  opencode: 'Everyday edits from the terminal',
  cursor: 'Edits guided by the files you have open',
  continue: 'Small changes inside the editor',
  cline: 'Step by step changes you want to watch happen',
}
const DEFAULT_HINT = 'General coding work'

export function hintFor(slug: string): string {
  return HINT_SUGGESTIONS[slug] || DEFAULT_HINT
}

export function StepMainProvider({
  draft,
  setDraft,
}: {
  draft: SetupDraft
  setDraft: React.Dispatch<React.SetStateAction<SetupDraft>>
}) {
  const { details } = useStore()
  const enabled = details.filter((d) => d.enabled)
  const main = details.find((d) => d.name === draft.main)

  // Preselect the first tool that is on, its saved model (or its first one),
  // and a starting "good at" line for every tool. Runs once per new provider.
  useEffect(() => {
    if (enabled.length === 0) return
    setDraft((prev) => {
      const next = { ...prev, hints: { ...prev.hints } }
      let changed = false
      if (!next.main || !enabled.some((d) => d.name === next.main)) {
        const first = enabled[0]
        next.main = first.name
        next.model = first.model || first.availableModels[0] || ''
        changed = true
      }
      for (const d of enabled) {
        if (next.hints[d.name] == null) {
          next.hints[d.name] = hintFor(d.name)
          changed = true
        }
      }
      return changed ? next : prev
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [details])

  const pickMain = (name: string) => {
    const d = details.find((x) => x.name === name)
    setDraft((prev) => ({
      ...prev,
      main: name,
      model: d ? d.model || d.availableModels[0] || '' : '',
    }))
  }

  return (
    <>
      <StepHead
        eyebrow="Step 3 of 5"
        title="Which tool leads the work"
        body="Relay starts every task with this one and hands over to the others as it runs out. You can change it at any time later."
      />

      {enabled.length === 0 ? (
        <Note icon="providers">
          No tools are turned on yet. Go back a step to turn one on, or skip this and pick a leader later
          from the Providers screen.
        </Note>
      ) : (
        <>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {enabled.map((d) => (
              <SelectRow
                key={d.name}
                selected={draft.main === d.name}
                onClick={() => pickMain(d.name)}
                glyph={<ProviderGlyph name={d.name} size={28} />}
                title={d.displayName || providerTitle(d.name)}
                subtitle={
                  d.accounts.length > 0
                    ? `${d.accounts.length} login${d.accounts.length > 1 ? 's' : ''} ready`
                    : 'No logins added yet'
                }
              />
            ))}
          </div>

          <div style={{ marginTop: 'var(--s5)' }}>
            <div
              style={{
                fontSize: 'var(--fz-xs)',
                fontWeight: 600,
                letterSpacing: '0.06em',
                textTransform: 'uppercase',
                color: 'var(--tx-2)',
                marginBottom: 6,
              }}
            >
              Model for {main ? main.displayName || providerTitle(main.name) : 'this tool'}
            </div>
            {main && main.availableModels.length > 0 ? (
              <select
                value={draft.model}
                onChange={(e) => setDraft((prev) => ({ ...prev, model: e.target.value }))}
                style={{ ...selectStyle, maxWidth: 360 }}
              >
                {main.availableModels.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            ) : (
              <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>
                This tool does not offer a choice of models here, so Relay uses its usual one.
              </div>
            )}
          </div>

          <div style={{ marginTop: 'var(--s5)' }}>
            <div
              style={{
                fontSize: 'var(--fz-xs)',
                fontWeight: 600,
                letterSpacing: '0.06em',
                textTransform: 'uppercase',
                color: 'var(--tx-2)',
                marginBottom: 6,
              }}
            >
              What each one is best at
            </div>
            <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginBottom: 'var(--s3)', lineHeight: 1.5 }}>
              One short line each. Relay saves these with your setup and uses them to decide who gets
              which part of a job. The suggestions below are fine to leave as they are.
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s3)' }}>
              {enabled.map((d) => (
                <div key={d.name} style={{ display: 'flex', alignItems: 'center', gap: 'var(--s3)' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, width: 150, flexShrink: 0 }}>
                    <ProviderGlyph name={d.name} size={22} />
                    <span
                      style={{
                        fontSize: 'var(--fz-sm)',
                        fontWeight: 500,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {providerTitle(d.name)}
                    </span>
                  </div>
                  <Input
                    value={draft.hints[d.name] ?? ''}
                    onChange={(v) => setDraft((prev) => ({ ...prev, hints: { ...prev.hints, [d.name]: v } }))}
                    placeholder={hintFor(d.name)}
                  />
                </div>
              ))}
            </div>
          </div>
        </>
      )}
    </>
  )
}
