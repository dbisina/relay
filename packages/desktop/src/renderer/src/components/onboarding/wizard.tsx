// components/onboarding/wizard.tsx — the shared shell for first-run setup: the
// step rail, step headings, selectable rows, and the inline "waiting for Relay"
// treatment that replaced the old full-screen connection takeover.
//
// Nothing here talks to the daemon. It reads connection state so that a control
// which genuinely needs the daemon can say so in place, instead of the whole
// window being covered while the daemon boots.

import React, { useState } from 'react'
import { Icon, type IconName } from '../Icon'
import { Spinner } from '../ui'
import { useStore } from '../../lib/store'

// ── Setup draft ──────────────────────────────────────────────────────────────
// One shared object the steps read and write. It is deliberately small: the
// daemon owns everything real, this only carries choices between steps.

/** One stop in the rotation: a provider, and which of its logins to use. */
export interface RotationEntry {
  provider: string
  account: string
}

export interface SetupDraft {
  /** Provider that leads the work. */
  main: string
  /** Model chosen for the main provider. */
  model: string
  /** provider slug -> one plain line about what it is good at. */
  hints: Record<string, string>
  rotationMode: 'default' | 'custom'
  rotation: RotationEntry[]
}

export const emptyDraft = (): SetupDraft => ({
  main: '',
  model: '',
  hints: {},
  rotationMode: 'default',
  rotation: [],
})

// ── Daemon readiness ─────────────────────────────────────────────────────────

/**
 * Whether daemon-backed actions can run right now. Steps use this to disable a
 * single control and explain why, rather than blocking the whole wizard.
 */
export function useDaemonReady(): { ready: boolean; failed: boolean; label: string } {
  const { conn } = useStore()
  if (conn === 'up') return { ready: true, failed: false, label: '' }
  if (conn === 'failed') return { ready: false, failed: true, label: 'Relay could not start' }
  return { ready: false, failed: false, label: 'Waiting for Relay to start' }
}

/** Inline, quiet: sits beside the control it is blocking. */
export function WaitingNote({ label }: { label?: string }) {
  const { ready, label: auto } = useDaemonReady()
  if (ready) return null
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        fontSize: 'var(--fz-xs)',
        color: 'var(--tx-3)',
      }}
    >
      <Spinner size={12} />
      {label || auto}
    </span>
  )
}

// ── Step rail ────────────────────────────────────────────────────────────────

export function StepRail({
  labels,
  current,
  reached,
  onJump,
}: {
  labels: string[]
  current: number
  reached: number
  onJump: (i: number) => void
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
      {labels.map((label, i) => {
        const done = i < current
        const active = i === current
        const open = i <= reached
        return (
          <React.Fragment key={label}>
            {i > 0 && (
              <span
                style={{
                  width: 16,
                  height: 1,
                  background: done || active ? 'var(--border-2)' : 'var(--border-0)',
                }}
              />
            )}
            <button
              onClick={open ? () => onJump(i) : undefined}
              title={open ? label : 'Not yet'}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                padding: '4px 10px 4px 6px',
                borderRadius: 'var(--r-pill)',
                border: `1px solid ${active ? 'var(--accent-line)' : 'transparent'}`,
                background: active ? 'var(--accent-weak)' : 'transparent',
                color: active ? 'var(--accent)' : open ? 'var(--tx-2)' : 'var(--tx-3)',
                fontSize: 'var(--fz-xs)',
                fontWeight: active ? 600 : 500,
                cursor: open ? 'pointer' : 'default',
                transition: 'background 0.14s var(--ease), color 0.14s var(--ease)',
              }}
            >
              <span
                style={{
                  width: 18,
                  height: 18,
                  borderRadius: '50%',
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  border: `1px solid ${active ? 'var(--accent)' : 'var(--border-2)'}`,
                  background: done ? 'var(--bg-3)' : 'transparent',
                  fontSize: 'var(--fz-xs)',
                  fontFamily: 'var(--font-mono)',
                  color: active ? 'var(--accent)' : 'var(--tx-2)',
                }}
              >
                {done ? <Icon name="check" size={11} /> : i + 1}
              </span>
              {label}
            </button>
          </React.Fragment>
        )
      })}
    </div>
  )
}

// ── Step heading ─────────────────────────────────────────────────────────────

export function StepHead({
  eyebrow,
  title,
  body,
}: {
  eyebrow?: string
  title: string
  body?: string
}) {
  return (
    <div style={{ marginBottom: 'var(--s5)' }}>
      {eyebrow && (
        <div
          style={{
            fontSize: 'var(--fz-xs)',
            fontWeight: 600,
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            color: 'var(--tx-3)',
            marginBottom: 6,
          }}
        >
          {eyebrow}
        </div>
      )}
      <h2 style={{ fontSize: 'var(--fz-xl)', fontWeight: 700, letterSpacing: '-0.01em' }}>{title}</h2>
      {body && (
        <p style={{ fontSize: 'var(--fz-md)', color: 'var(--tx-2)', marginTop: 6, lineHeight: 1.6 }}>
          {body}
        </p>
      )}
    </div>
  )
}

/** A quiet block of explanation. Used once per step at most. */
export function Note({ icon = 'handoff', children }: { icon?: IconName; children: React.ReactNode }) {
  return (
    <div
      style={{
        display: 'flex',
        gap: 'var(--s2)',
        alignItems: 'flex-start',
        padding: '10px 12px',
        borderRadius: 'var(--r)',
        background: 'var(--bg-1)',
        border: '1px solid var(--border-0)',
      }}
    >
      <Icon name={icon} size={15} style={{ color: 'var(--tx-2)', marginTop: 1 }} />
      <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', lineHeight: 1.55 }}>{children}</div>
    </div>
  )
}

// ── Selectable row ───────────────────────────────────────────────────────────

/**
 * One choosable line: a mark, a glyph, a title, a plain explanation, and room
 * for a control on the right. Every "pick this" surface in the wizard is this
 * row, so the whole flow reads as one thing rather than five styles of card.
 */
export function SelectRow({
  mark = 'radio',
  selected,
  disabled,
  onClick,
  glyph,
  title,
  subtitle,
  right,
}: {
  mark?: 'radio' | 'check' | 'none'
  selected: boolean
  disabled?: boolean
  onClick?: () => void
  glyph?: React.ReactNode
  title: React.ReactNode
  subtitle?: React.ReactNode
  right?: React.ReactNode
}) {
  const [hover, setHover] = useState(false)
  return (
    <div
      onClick={disabled ? undefined : onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      role={onClick ? 'button' : undefined}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 'var(--s3)',
        padding: '11px 12px',
        borderRadius: 'var(--r)',
        textAlign: 'left',
        width: '100%',
        border: `1px solid ${selected ? 'var(--accent-line)' : 'var(--border-1)'}`,
        background: selected ? 'var(--accent-weak)' : hover && !disabled ? 'var(--hover)' : 'transparent',
        opacity: disabled ? 0.55 : 1,
        cursor: onClick && !disabled ? 'pointer' : 'default',
        transition: 'background 0.14s var(--ease), border-color 0.14s var(--ease)',
      }}
    >
      {mark !== 'none' && (
        <span
          style={{
            width: 18,
            height: 18,
            flexShrink: 0,
            borderRadius: mark === 'radio' ? '50%' : 'var(--r-sm)',
            border: `1.5px solid ${selected ? 'var(--accent)' : 'var(--border-2)'}`,
            background: selected && mark === 'check' ? 'var(--accent)' : 'transparent',
            color: 'var(--accent-ink)',
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          {selected && mark === 'radio' && (
            <span style={{ width: 9, height: 9, borderRadius: '50%', background: 'var(--accent)' }} />
          )}
          {selected && mark === 'check' && <Icon name="check" size={11} />}
        </span>
      )}
      {glyph}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 'var(--fz-md)', fontWeight: 600, color: 'var(--tx-0)' }}>{title}</div>
        {subtitle && (
          <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)', marginTop: 3, lineHeight: 1.5 }}>
            {subtitle}
          </div>
        )}
      </div>
      {right && (
        <div onClick={(e) => e.stopPropagation()} style={{ flexShrink: 0 }}>
          {right}
        </div>
      )}
    </div>
  )
}

/** Native select, styled to match Input. Shared by the model and stage pickers. */
export const selectStyle: React.CSSProperties = {
  width: '100%',
  height: 34,
  padding: '0 10px',
  borderRadius: 'var(--r)',
  background: 'var(--bg-0)',
  border: '1px solid var(--border-1)',
  color: 'var(--tx-0)',
  fontSize: 'var(--fz-md)',
}
