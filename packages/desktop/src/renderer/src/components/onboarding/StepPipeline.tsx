// components/onboarding/StepPipeline.tsx — the last, optional step.
//
// A pipeline splits one job across several tools that run in order, each
// picking up where the last stopped. Most people never need this, so the step
// opens closed: it says what a pipeline is, offers to skip, and only shows the
// builder if asked. Nothing here is required to finish setup.

import React, { useState } from 'react'
import { StepHead } from './wizard'
import { Button, Input, Badge, ProviderGlyph } from '../ui'
import { Icon } from '../Icon'
import { providerTitle } from '../../lib/format'
import type { ProviderDetail, PipelineNode } from '../../lib/types'

export interface PipelineDraft {
  /** False until the user opts in. Finish does nothing when this is false. */
  enabled: boolean
  name: string
  stages: { provider: string; task: string }[]
}

export const emptyPipeline = (): PipelineDraft => ({
  enabled: false,
  name: 'my-pipeline',
  stages: [],
})

/** Turn the friendly draft into the node shape the daemon stores. */
export function toPipelineNodes(draft: PipelineDraft): PipelineNode[] {
  return draft.stages
    .filter((s) => s.provider && s.task.trim())
    .map((s, i) => ({
      id: `stage-${i + 1}`,
      provider: s.provider,
      task: s.task.trim(),
      dependsOn: i === 0 ? [] : [`stage-${i}`],
      fallback: [],
      verify: [],
    }))
}

export function StepPipeline({
  details,
  draft,
  setDraft,
}: {
  details: ProviderDetail[]
  draft: PipelineDraft
  setDraft: React.Dispatch<React.SetStateAction<PipelineDraft>>
}) {
  const enabled = details.filter((d) => d.enabled)
  const [addOpen, setAddOpen] = useState(false)

  const addStage = (provider: string) => {
    setDraft((d) => ({ ...d, stages: [...d.stages, { provider, task: '' }] }))
    setAddOpen(false)
  }
  const setTask = (i: number, task: string) =>
    setDraft((d) => ({ ...d, stages: d.stages.map((s, j) => (j === i ? { ...s, task } : s)) }))
  const removeStage = (i: number) =>
    setDraft((d) => ({ ...d, stages: d.stages.filter((_, j) => j !== i) }))

  return (
    <>
      <StepHead
        eyebrow="Optional"
        title="Split a job across tools"
        body="A pipeline hands one job through several tools in order, for example one writes the code and the next reviews it. You can skip this and set it up later, most people do."
      />

      {!draft.enabled ? (
        <div
          style={{
            border: '1px solid var(--border-1)',
            borderRadius: 'var(--r-lg)',
            background: 'var(--bg-1)',
            padding: 'var(--s4)',
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--s3)',
          }}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 'var(--fz-md)', fontWeight: 600 }}>No pipeline for now</div>
            <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', marginTop: 3, lineHeight: 1.5 }}>
              Relay will still rotate through your accounts on its own. You can build a pipeline
              anytime from the Pipelines screen.
            </div>
          </div>
          <Button variant="default" icon="pipelines" onClick={() => setDraft((d) => ({ ...d, enabled: true }))}>
            Set one up
          </Button>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s3)' }}>
          <div>
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
              Name
            </div>
            <Input value={draft.name} onChange={(v) => setDraft((d) => ({ ...d, name: v }))} placeholder="my-pipeline" />
          </div>

          {draft.stages.map((s, i) => (
            <div
              key={i}
              style={{
                border: '1px solid var(--border-1)',
                borderRadius: 'var(--r)',
                background: 'var(--bg-0)',
                padding: 'var(--s3)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--s2)', marginBottom: 8 }}>
                <Badge tone="neutral">Step {i + 1}</Badge>
                <ProviderGlyph name={s.provider} size={20} />
                <span style={{ flex: 1, fontSize: 'var(--fz-sm)', fontWeight: 600 }}>
                  {providerTitle(s.provider)}
                </span>
                <button
                  onClick={() => removeStage(i)}
                  aria-label={`Remove step ${i + 1}`}
                  style={{ color: 'var(--tx-2)', padding: 3, display: 'flex' }}
                >
                  <Icon name="x" size={13} />
                </button>
              </div>
              <Input
                value={s.task}
                onChange={(v) => setTask(i, v)}
                placeholder="What should this tool do?"
              />
            </div>
          ))}

          {addOpen ? (
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              {enabled.map((d) => (
                <button
                  key={d.name}
                  onClick={() => addStage(d.name)}
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    padding: '6px 11px',
                    borderRadius: 'var(--r-pill)',
                    border: '1px solid var(--border-1)',
                    color: 'var(--tx-1)',
                    fontSize: 'var(--fz-sm)',
                  }}
                >
                  <ProviderGlyph name={d.name} size={18} />
                  {providerTitle(d.name)}
                </button>
              ))}
              <Button variant="ghost" size="sm" onClick={() => setAddOpen(false)}>
                Cancel
              </Button>
            </div>
          ) : (
            <div style={{ display: 'flex', gap: 'var(--s2)' }}>
              <Button variant="default" icon="plus" onClick={() => setAddOpen(true)} disabled={enabled.length === 0}>
                Add a step
              </Button>
              <Button variant="ghost" onClick={() => setDraft(emptyPipeline())}>
                Never mind
              </Button>
            </div>
          )}

          {enabled.length === 0 && (
            <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>
              Turn on at least one tool first, then you can add steps here.
            </div>
          )}
        </div>
      )}
    </>
  )
}
