// components/onboarding/StepRotation.tsx — step 4. The order Relay walks your
// logins. The recommended order uses up every login on one tool before moving
// to the next tool, which is what most people mean by "use my second account
// before you switch to something else".
//
// The custom order is an account-level version of components/ChainBuilder:
// same idea (an ordered list plus a pool of things to add), one level deeper,
// so ChainBuilder itself stays a provider-only control.

import React, { useState } from 'react'
import { providerTitle } from '../../lib/format'
import { ProviderGlyph } from '../ui'
import { Icon } from '../Icon'
import type { ProviderDetail } from '../../lib/types'
import { StepHead, Note, SelectRow, type RotationEntry, type SetupDraft } from './wizard'

const keyOf = (e: RotationEntry) => `${e.provider}::${e.account}`

/** Every login Relay could use, provider by provider, main provider first. */
export function allEntries(details: ProviderDetail[], main: string): RotationEntry[] {
  const enabled = details.filter((d) => d.enabled)
  const ordered = [
    ...enabled.filter((d) => d.name === main),
    ...enabled.filter((d) => d.name !== main),
  ]
  return ordered.flatMap((d) =>
    d.accounts.length > 0
      ? d.accounts.map((a) => ({ provider: d.name, account: a.label }))
      : [{ provider: d.name, account: '' }],
  )
}

/** The recommended order: all of one tool's logins, then the next tool. */
export function buildDefaultRotation(details: ProviderDetail[], main: string): RotationEntry[] {
  return allEntries(details, main)
}

function move<T>(arr: T[], from: number, to: number): T[] {
  if (from === to || from < 0 || to < 0 || from >= arr.length || to >= arr.length) return arr
  const copy = [...arr]
  const [item] = copy.splice(from, 1)
  copy.splice(to, 0, item)
  return copy
}

// ── Account-level chain ──────────────────────────────────────────────────────

function AccountChain({
  entries,
  pool,
  onChange,
}: {
  entries: RotationEntry[]
  pool: RotationEntry[]
  onChange: (next: RotationEntry[]) => void
}) {
  const [dragging, setDragging] = useState<number | null>(null)
  const remaining = pool.filter((p) => !entries.some((e) => keyOf(e) === keyOf(p)))

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {entries.length === 0 && (
        <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>
          Nothing in the order yet. Add logins from the list below.
        </div>
      )}

      {entries.map((e, i) => (
        <div
          key={keyOf(e)}
          draggable
          onDragStart={() => setDragging(i)}
          onDragOver={(ev) => ev.preventDefault()}
          onDrop={() => {
            if (dragging != null) onChange(move(entries, dragging, i))
            setDragging(null)
          }}
          onDragEnd={() => setDragging(null)}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--s2)',
            padding: '8px 10px',
            borderRadius: 'var(--r)',
            background: 'var(--bg-0)',
            border: '1px solid var(--border-1)',
            opacity: dragging === i ? 0.5 : 1,
            cursor: 'grab',
          }}
        >
          <span
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--fz-xs)',
              color: 'var(--tx-3)',
              width: 16,
            }}
          >
            {i + 1}
          </span>
          <ProviderGlyph name={e.provider} size={22} />
          <span style={{ flex: 1, minWidth: 0, fontSize: 'var(--fz-md)', fontWeight: 500 }}>
            {providerTitle(e.provider)}
            {e.account && (
              <span style={{ color: 'var(--tx-2)', fontWeight: 400 }}>{`, ${e.account}`}</span>
            )}
          </span>
          <button
            onClick={() => onChange(move(entries, i, i - 1))}
            disabled={i === 0}
            title="Move up"
            style={{ color: 'var(--tx-1)', padding: 3, opacity: i === 0 ? 0.35 : 1 }}
          >
            <Icon name="chevronDown" size={14} style={{ transform: 'rotate(180deg)' }} />
          </button>
          <button
            onClick={() => onChange(move(entries, i, i + 1))}
            disabled={i === entries.length - 1}
            title="Move down"
            style={{ color: 'var(--tx-1)', padding: 3, opacity: i === entries.length - 1 ? 0.35 : 1 }}
          >
            <Icon name="chevronDown" size={14} />
          </button>
          <button
            onClick={() => onChange(entries.filter((_, j) => j !== i))}
            title="Take out of the order"
            style={{ color: 'var(--tx-2)', padding: 3 }}
          >
            <Icon name="x" size={14} />
          </button>
        </div>
      ))}

      {remaining.length > 0 && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 2 }}>
          {remaining.map((p) => (
            <button
              key={keyOf(p)}
              onClick={() => onChange([...entries, p])}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                padding: '5px 10px',
                borderRadius: 'var(--r-pill)',
                border: '1px dashed var(--border-2)',
                color: 'var(--tx-1)',
                fontSize: 'var(--fz-sm)',
              }}
            >
              <Icon name="plus" size={12} /> {providerTitle(p.provider)}
              {p.account ? `, ${p.account}` : ''}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ── The step ─────────────────────────────────────────────────────────────────

export function StepRotation({
  details,
  draft,
  setDraft,
}: {
  details: ProviderDetail[]
  draft: SetupDraft
  setDraft: React.Dispatch<React.SetStateAction<SetupDraft>>
}) {
  const pool = allEntries(details, draft.main)
  const recommended = buildDefaultRotation(details, draft.main)
  const shown = draft.rotationMode === 'default' ? recommended : draft.rotation

  const preview = shown.map((e) => `${providerTitle(e.provider)}${e.account ? ` (${e.account})` : ''}`)

  return (
    <>
      <StepHead
        eyebrow="Step 4 of 5"
        title="The order Relay works through them"
        body="When one login runs out of usage, Relay pauses the task at a safe point and picks up on the next one in this list."
      />

      {pool.length === 0 ? (
        <Note icon="handoff">
          There is nothing to put in order yet. Go back and turn on a tool, or skip this and set the order
          later on the Agents and skills screen.
        </Note>
      ) : (
        <>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <SelectRow
              selected={draft.rotationMode === 'default'}
              onClick={() => setDraft((p) => ({ ...p, rotationMode: 'default' }))}
              title="Use up every login on one tool first (recommended)"
              subtitle="Relay finishes all of your Claude logins before it moves to the next tool, and so on down the list. This keeps a task on the same tool for as long as possible."
            />
            <SelectRow
              selected={draft.rotationMode === 'custom'}
              onClick={() =>
                setDraft((p) => ({
                  ...p,
                  rotationMode: 'custom',
                  rotation: p.rotation.length > 0 ? p.rotation : recommended,
                }))
              }
              title="Set my own order"
              subtitle="Drag the logins into the order you want, or use the arrows."
            />
          </div>

          <div style={{ marginTop: 'var(--s4)' }}>
            {draft.rotationMode === 'custom' ? (
              <AccountChain
                entries={draft.rotation}
                pool={pool}
                onChange={(next) => setDraft((p) => ({ ...p, rotation: next }))}
              />
            ) : (
              <div
                style={{
                  padding: 'var(--s3)',
                  borderRadius: 'var(--r)',
                  background: 'var(--bg-1)',
                  border: '1px solid var(--border-0)',
                  display: 'flex',
                  flexWrap: 'wrap',
                  alignItems: 'center',
                  gap: 8,
                }}
              >
                {shown.map((e, i) => (
                  <React.Fragment key={keyOf(e)}>
                    {i > 0 && <Icon name="chevronRight" size={12} style={{ color: 'var(--tx-3)' }} />}
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                      <ProviderGlyph name={e.provider} size={20} />
                      <span style={{ fontSize: 'var(--fz-sm)' }}>
                        {providerTitle(e.provider)}
                        {e.account && <span style={{ color: 'var(--tx-2)' }}> , {e.account}</span>}
                      </span>
                    </span>
                  </React.Fragment>
                ))}
              </div>
            )}
          </div>

          <div style={{ marginTop: 'var(--s4)' }}>
            <Note icon="handoff">
              Relay will work through {preview.join(', then ') || 'nothing yet'}. Handing over is
              automatic: the task, the plan, and the files in progress travel with it.
            </Note>
          </div>
        </>
      )}
    </>
  )
}
