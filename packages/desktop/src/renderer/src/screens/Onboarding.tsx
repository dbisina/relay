// Onboarding.tsx — first run, as a wizard rather than a wall.
//
// Shape of the flow: pick a language, turn on the tools you have and sign in to
// however many accounts you own, say which tool leads, decide the order Relay
// walks through them, and optionally split a job across several. Every step can
// be skipped, and the whole thing can be skipped.
//
// The daemon is NOT a gate here. The first two steps are useful before it is
// up, so the wizard renders immediately and individual controls that genuinely
// need the daemon say so in place. See useDaemonReady in ./wizard.

import React, { useState } from 'react'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { useLocale } from '../lib/i18n'
import { Button } from '../components/ui'
import {
  StepRail,
  emptyDraft,
  useDaemonReady,
  type SetupDraft,
  type RotationEntry,
} from '../components/onboarding/wizard'
import { StepLanguage } from '../components/onboarding/StepLanguage'
import { StepProviders } from '../components/onboarding/StepProviders'
import { StepMainProvider } from '../components/onboarding/StepMainProvider'
import { StepRotation } from '../components/onboarding/StepRotation'
import {
  StepPipeline,
  emptyPipeline,
  toPipelineNodes,
  type PipelineDraft,
} from '../components/onboarding/StepPipeline'

const STEPS = ['Language', 'Your tools', 'Lead tool', 'Order', 'Pipeline']

/** Rotation entries collapse to the provider chain a profile stores. */
function chainFrom(rotation: RotationEntry[]): string[] {
  const seen: string[] = []
  for (const e of rotation) if (e.provider && !seen.includes(e.provider)) seen.push(e.provider)
  return seen
}

export function Onboarding({ onDone }: { onDone: () => void }) {
  const toast = useToast()
  const { t } = useLocale()
  const { details, refresh } = useStore()
  const { ready } = useDaemonReady()

  const [step, setStep] = useState(0)
  const [reached, setReached] = useState(0)
  const [draft, setDraft] = useState<SetupDraft>(emptyDraft)
  const [pipeline, setPipeline] = useState<PipelineDraft>(emptyPipeline)
  const [saving, setSaving] = useState(false)

  const go = (i: number) => {
    const next = Math.max(0, Math.min(STEPS.length - 1, i))
    setStep(next)
    setReached((r) => Math.max(r, next))
  }

  /**
   * Persist the choices. Everything here is best-effort: setup finishing is
   * more important than any single save, so a failure is reported and the user
   * still lands in the app rather than being trapped in the wizard.
   */
  const finish = async () => {
    setSaving(true)
    const problems: string[] = []

    // The rotation, stored as a profile so the router can use it.
    const rotation =
      draft.rotationMode === 'default'
        ? details.filter((d) => d.enabled).map((d) => ({ provider: d.name, account: '' }))
        : draft.rotation
    const chain = chainFrom(rotation)
    if (chain.length > 0) {
      const r = await api.saveProfile({
        name: 'default',
        chain,
        kinds: [],
        skills: [],
        contextHint: draft.hints[draft.main] || '',
      })
      if (!r.ok) problems.push(`rotation: ${r.error}`)
    }

    if (pipeline.enabled) {
      const nodes = toPipelineNodes(pipeline)
      if (nodes.length > 0) {
        const r = await api.savePipeline({ name: pipeline.name.trim() || 'my-pipeline', nodes })
        if (!r.ok) problems.push(`pipeline: ${r.error}`)
      }
    }

    setSaving(false)
    if (problems.length > 0) toast(`Saved with problems, ${problems.join('; ')}`, 'error')
    else toast('Setup saved', 'ok')
    refresh()
    onDone()
  }

  const isLast = step === STEPS.length - 1

  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        display: 'flex',
        flexDirection: 'column',
        background: 'radial-gradient(1000px 600px at 50% 12%, var(--bg-1), var(--bg-0) 70%)',
        overflow: 'auto',
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: 620,
          margin: '0 auto',
          padding: 'var(--s6) var(--s5) var(--s7)',
          display: 'flex',
          flexDirection: 'column',
          minHeight: '100%',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--s5)' }}>
          <StepRail labels={STEPS} current={step} reached={reached} onJump={go} />
          <button
            onClick={onDone}
            style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', padding: 4 }}
          >
            {t('onboarding.skip')}
          </button>
        </div>

        <div style={{ flex: 1 }}>
          {step === 0 && <StepLanguage />}
          {step === 1 && <StepProviders />}
          {step === 2 && <StepMainProvider draft={draft} setDraft={setDraft} />}
          {step === 3 && <StepRotation details={details} draft={draft} setDraft={setDraft} />}
          {step === 4 && <StepPipeline details={details} draft={pipeline} setDraft={setPipeline} />}
        </div>

        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 'var(--s3)',
            marginTop: 'var(--s6)',
            paddingTop: 'var(--s4)',
            borderTop: '1px solid var(--border-0)',
          }}
        >
          <div>
            {step > 0 && (
              <Button variant="ghost" onClick={() => go(step - 1)}>
                {t('onboarding.back')}
              </Button>
            )}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--s2)' }}>
            {!isLast && (
              <Button variant="ghost" onClick={() => go(step + 1)}>
                {t('onboarding.skip')} this step
              </Button>
            )}
            {isLast ? (
              <Button variant="primary" icon="check" loading={saving} onClick={finish}>
                {t('onboarding.finish')}
              </Button>
            ) : (
              <Button variant="primary" onClick={() => go(step + 1)}>
                {step === 0 ? t('onboarding.getStarted') : t('onboarding.next')}
              </Button>
            )}
          </div>
        </div>

        {!ready && (
          <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 'var(--s3)', textAlign: 'center' }}>
            Relay is starting in the background. You can keep going, anything that needs it will
            wait its turn.
          </div>
        )}
      </div>
    </div>
  )
}
