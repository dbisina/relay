// ui.tsx — shared primitives. Small, composable, token-driven. No card-grids,
// no side-stripe accents; borders and rows carry structure, the accent is rare.
//
// Semantics live here too: an icon-only control gets its name from `title`, a
// toggleable Chip reports aria-pressed, and nothing overrides the tokenised
// :focus-visible ring from global.css. Screens inherit all of it for free.

import React, { useState, useId } from 'react'
import { Icon, type IconName } from './Icon'
import { PROVIDER_MARKS } from './provider-marks'

// ── Button ───────────────────────────────────────────────────────────────────
type BtnVariant = 'primary' | 'default' | 'ghost' | 'danger'
export function Button({
  children,
  variant = 'default',
  icon,
  size = 'md',
  loading,
  disabled,
  onClick,
  title,
  style,
}: {
  children?: React.ReactNode
  variant?: BtnVariant
  icon?: IconName
  size?: 'sm' | 'md'
  loading?: boolean
  disabled?: boolean
  onClick?: () => void
  title?: string
  style?: React.CSSProperties
}) {
  const [hover, setHover] = useState(false)
  const off = disabled || loading
  const pad = size === 'sm' ? '5px 10px' : '7px 13px'
  const base: React.CSSProperties = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    padding: pad,
    borderRadius: 'var(--r)',
    fontSize: size === 'sm' ? 'var(--fz-sm)' : 'var(--fz-md)',
    fontWeight: 500,
    lineHeight: 1,
    whiteSpace: 'nowrap',
    transition: 'background 0.14s var(--ease), border-color 0.14s var(--ease), opacity 0.14s',
    opacity: off ? 0.5 : 1,
    cursor: off ? 'not-allowed' : 'pointer',
    border: '1px solid transparent',
  }
  const variants: Record<BtnVariant, React.CSSProperties> = {
    primary: {
      background: hover && !off ? 'var(--accent-hi)' : 'var(--accent)',
      color: 'var(--accent-ink)',
      fontWeight: 600,
    },
    default: {
      background: hover && !off ? 'var(--bg-4)' : 'var(--bg-3)',
      borderColor: 'var(--border-1)',
      color: 'var(--tx-0)',
    },
    ghost: {
      background: hover && !off ? 'var(--hover)' : 'transparent',
      color: 'var(--tx-1)',
    },
    danger: {
      background: hover && !off ? 'var(--red-weak)' : 'transparent',
      borderColor: 'var(--border-1)',
      color: 'var(--red)',
    },
  }
  // An icon-only button has no text node to name it, so `title` doubles as the
  // accessible name. With children present the visible label already names it,
  // and an aria-label would silently override what the user can read.
  const named = children != null && children !== false && children !== ''
  return (
    <button
      type="button"
      title={title}
      aria-label={named ? undefined : title}
      aria-disabled={off || undefined}
      aria-busy={loading || undefined}
      onClick={off ? undefined : onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{ ...base, ...variants[variant], ...style }}
    >
      {loading ? (
        <Icon name="refresh" size={14} style={{ animation: 'relay-spin 0.8s linear infinite' }} />
      ) : icon ? (
        <Icon name={icon} size={14} />
      ) : null}
      {children}
    </button>
  )
}

// ── Panel — a bordered surface. Used sparingly, never nested. ────────────────
export function Panel({
  children,
  style,
  pad = true,
}: {
  children: React.ReactNode
  style?: React.CSSProperties
  pad?: boolean
}) {
  return (
    <div
      style={{
        background: 'var(--bg-1)',
        border: '1px solid var(--border-0)',
        borderRadius: 'var(--r-lg)',
        padding: pad ? 'var(--s4)' : 0,
        ...style,
      }}
    >
      {children}
    </div>
  )
}

// ── Field label + control wrapper ────────────────────────────────────────────
export function Field({
  label,
  hint,
  children,
  style,
}: {
  label: string
  hint?: string
  children: React.ReactNode
  style?: React.CSSProperties
}) {
  return (
    <label style={{ display: 'block', ...style }}>
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
        {label}
      </div>
      {children}
      {hint && (
        <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 5, lineHeight: 1.5 }}>
          {hint}
        </div>
      )}
    </label>
  )
}

export function Input({
  value,
  onChange,
  placeholder,
  type = 'text',
  mono,
  onEnter,
  autoFocus,
  style,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  mono?: boolean
  onEnter?: () => void
  autoFocus?: boolean
  style?: React.CSSProperties
}) {
  return (
    <input
      type={type}
      value={value}
      autoFocus={autoFocus}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && onEnter) onEnter()
      }}
      className="selectable"
      style={{
        width: '100%',
        height: 34,
        padding: '0 11px',
        borderRadius: 'var(--r)',
        background: 'var(--bg-0)',
        border: '1px solid var(--border-1)',
        color: 'var(--tx-0)',
        fontFamily: mono ? 'var(--font-mono)' : 'var(--font-ui)',
        fontSize: 'var(--fz-md)',
        // No `outline: none` here: the tokenised :focus-visible ring in
        // global.css is the one focus affordance, and inline styles beat it.
        ...style,
      }}
    />
  )
}

// ── Chip — a small toggleable control for a toolbar (composer chips, filters) ─
export function Chip({
  icon,
  children,
  active,
  onClick,
  style,
}: {
  icon?: IconName
  children: React.ReactNode
  active?: boolean
  onClick?: () => void
  style?: React.CSSProperties
}) {
  const [hover, setHover] = useState(false)
  return (
    <button
      type="button"
      // `active` is what the chip communicates visually, so it is the pressed
      // state. Chips used as plain actions pass no `active` and stay unpressed.
      aria-pressed={active}
      onClick={onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        height: 28,
        padding: '0 10px',
        borderRadius: 'var(--r-pill)',
        fontSize: 'var(--fz-sm)',
        fontWeight: 500,
        border: `1px solid ${active ? 'var(--accent-line)' : 'var(--border-1)'}`,
        background: active ? 'var(--accent-weak)' : hover ? 'var(--hover)' : 'transparent',
        color: active ? 'var(--accent)' : 'var(--tx-1)',
        whiteSpace: 'nowrap',
        transition: 'background 0.12s, border-color 0.12s',
        ...style,
      }}
    >
      {icon && <Icon name={icon} size={13} />}
      {children}
    </button>
  )
}

// ── Badge / pill ─────────────────────────────────────────────────────────────
type Tone = 'neutral' | 'accent' | 'green' | 'yellow' | 'red' | 'blue'
const toneMap: Record<Tone, { fg: string; bg: string }> = {
  neutral: { fg: 'var(--tx-1)', bg: 'transparent' },
  accent: { fg: 'var(--accent)', bg: 'var(--accent-weak)' },
  green: { fg: 'var(--green)', bg: 'var(--green-weak)' },
  yellow: { fg: 'var(--yellow)', bg: 'var(--yellow-weak)' },
  red: { fg: 'var(--red)', bg: 'var(--red-weak)' },
  blue: { fg: 'var(--blue)', bg: 'var(--blue-weak)' },
}
export function Badge({
  children,
  tone = 'neutral',
  style,
}: {
  children: React.ReactNode
  tone?: Tone
  style?: React.CSSProperties
}) {
  const t = toneMap[tone]
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 5,
        padding: '2px 8px',
        borderRadius: 'var(--r-pill)',
        fontSize: 'var(--fz-xs)',
        fontWeight: 600,
        letterSpacing: '0.02em',
        color: t.fg,
        background: t.bg,
        border: `1px solid ${t.bg === 'transparent' ? 'var(--border-1)' : 'transparent'}`,
        ...style,
      }}
    >
      {children}
    </span>
  )
}

export function StatusDot({ tone = 'neutral', pulse }: { tone?: Tone; pulse?: boolean }) {
  return (
    <span
      // Decorative: the state it colours is always spelled out in adjacent text.
      aria-hidden="true"
      style={{
        width: 7,
        height: 7,
        borderRadius: '50%',
        flexShrink: 0,
        background: toneMap[tone].fg,
        animation: pulse ? 'relay-pulse 1.2s ease-in-out infinite' : undefined,
      }}
    />
  )
}

// ── Quota meter ──────────────────────────────────────────────────────────────
export function Meter({
  fraction,
  tone,
  label,
}: {
  fraction: number
  tone?: Tone
  /** Names the meter for assistive tech, e.g. "Claude quota used". */
  label?: string
}) {
  const f = Math.max(0, Math.min(1, fraction || 0))
  const auto: Tone = f >= 0.9 ? 'red' : f >= 0.7 ? 'yellow' : 'green'
  const t = tone ?? auto
  const pct = Math.round(f * 100)
  return (
    <div
      // Colour alone carries the warning bands, so the value has to be readable
      // without seeing them.
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={pct}
      aria-valuetext={`${pct}%`}
      style={{
        height: 5,
        borderRadius: 'var(--r-pill)',
        background: 'var(--bg-3)',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          height: '100%',
          width: `${f * 100}%`,
          background: toneMap[t].fg,
          borderRadius: 'var(--r-pill)',
          transition: 'width 0.4s var(--ease-quint)',
        }}
      />
    </div>
  )
}

// ── Provider glyph ───────────────────────────────────────────────────────────
// Real, official provider marks where one is published (see provider-marks.ts,
// generated from simple-icons). Providers with no published mark (OpenAI
// withdrew theirs; Antigravity and Continue have none) fall back to a lettermark
// in their brand color: honest identification rather than an invented logo.

const FALLBACK_HEX: Record<string, string> = {
  codex: '#9AE6C4', // OpenAI product, mark not redistributable
  antigravity: '#8AB4F8', // Google product blue
  continue: '#B39DFF',
}

export function ProviderGlyph({ name, size = 30 }: { name: string; size?: number }) {
  const slug = (name || '').toLowerCase()
  const mark = PROVIDER_MARKS[slug]
  const inner = Math.round(size * 0.56)
  const label = mark?.title || providerLabel(slug)
  return (
    <div
      title={label}
      // The glyph is the only thing identifying the provider in tight rows, so
      // it is content, not decoration.
      role="img"
      aria-label={label}
      style={{
        width: size,
        height: size,
        borderRadius: 'var(--r)',
        background: 'var(--bg-3)',
        border: '1px solid var(--border-1)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
      }}
    >
      {mark ? (
        <svg width={inner} height={inner} viewBox="0 0 24 24" fill={`#${mark.hex}`} aria-hidden="true">
          <path d={mark.path} />
        </svg>
      ) : (
        <span
          style={{
            fontFamily: 'var(--font-ui)',
            fontWeight: 700,
            fontSize: size * 0.4,
            lineHeight: 1,
            color: FALLBACK_HEX[slug] || 'var(--tx-1)',
            textTransform: 'uppercase',
          }}
        >
          {(name || '?').slice(0, 1)}
        </span>
      )}
    </div>
  )
}

function providerLabel(slug: string): string {
  return slug ? slug[0].toUpperCase() + slug.slice(1) : 'Unknown'
}

// ── Empty state ──────────────────────────────────────────────────────────────
export function EmptyState({
  icon,
  title,
  body,
  action,
}: {
  icon?: IconName
  title: string
  body?: string
  action?: React.ReactNode
}) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        textAlign: 'center',
        padding: 'var(--s7) var(--s4)',
        gap: 'var(--s3)',
      }}
    >
      {icon && (
        <div
          style={{
            width: 46,
            height: 46,
            borderRadius: 'var(--r-lg)',
            border: '1px solid var(--border-1)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: 'var(--tx-3)',
          }}
        >
          <Icon name={icon} size={22} />
        </div>
      )}
      <div style={{ fontSize: 'var(--fz-lg)', fontWeight: 600 }}>{title}</div>
      {body && (
        <div style={{ fontSize: 'var(--fz-md)', color: 'var(--tx-2)', maxWidth: 380, lineHeight: 1.55 }}>
          {body}
        </div>
      )}
      {action && <div style={{ marginTop: 4 }}>{action}</div>}
    </div>
  )
}

export function Spinner({ size = 16 }: { size?: number }) {
  return (
    <Icon name="refresh" size={size} style={{ animation: 'relay-spin 0.8s linear infinite', color: 'var(--tx-2)' }} />
  )
}

// ── Section header for a screen region ───────────────────────────────────────
export function SectionTitle({
  children,
  right,
}: {
  children: React.ReactNode
  right?: React.ReactNode
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: 'var(--s3)',
      }}
    >
      <div style={{ fontSize: 'var(--fz-md)', fontWeight: 600, color: 'var(--tx-1)' }}>{children}</div>
      {right}
    </div>
  )
}

// ── Modal sheet (used rarely, only when genuinely modal) ─────────────────────
export function Sheet({
  open,
  onClose,
  title,
  children,
  width = 460,
}: {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
  width?: number
}) {
  const titleId = useId()
  if (!open) return null
  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 150,
        background: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 'var(--s5)',
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        style={{
          width,
          maxWidth: '100%',
          maxHeight: '86vh',
          overflow: 'auto',
          background: 'var(--bg-1)',
          border: '1px solid var(--border-2)',
          borderRadius: 'var(--r-lg)',
          boxShadow: '0 24px 70px rgba(0,0,0,0.6)',
          animation: 'relay-rise 0.2s var(--ease) both',
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '14px var(--s4)',
            borderBottom: '1px solid var(--border-0)',
          }}
        >
          <div id={titleId} style={{ fontSize: 'var(--fz-lg)', fontWeight: 600 }}>
            {title}
          </div>
          <button
            type="button"
            onClick={onClose}
            style={{ color: 'var(--tx-2)', display: 'flex', padding: 4, borderRadius: 'var(--r)' }}
            title="Close"
            aria-label="Close"
          >
            <Icon name="x" size={16} />
          </button>
        </div>
        <div style={{ padding: 'var(--s4)' }}>{children}</div>
      </div>
    </div>
  )
}
