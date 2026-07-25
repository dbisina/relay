// Drawer.tsx — a right-side panel for content you READ rather than decide on.
//
// Distinct from Sheet (centered modal) on purpose: Sheet is capped at 86vh and
// sized for a short form, which is the wrong container for a tall scannable
// document (a session manifest can carry ~100 file paths and two long prompts).
// The drawer runs full height, keeps the app visible behind a scrim so the
// session list stays in context, and pins a header and footer around a
// scrolling body so the primary action never scrolls out of reach.

import React, { useEffect, useRef } from 'react'
import { Icon } from './Icon'

export function Drawer({
  open,
  onClose,
  title,
  subtitle,
  footer,
  width = 520,
  children,
}: {
  open: boolean
  onClose: () => void
  title: React.ReactNode
  subtitle?: React.ReactNode
  footer?: React.ReactNode
  width?: number
  children: React.ReactNode
}) {
  const panelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      onMouseDown={(e) => {
        // Scrim click closes; clicks that start inside the panel do not.
        if (!panelRef.current?.contains(e.target as Node)) onClose()
      }}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 160,
        background: 'rgba(0,0,0,0.42)',
        display: 'flex',
        justifyContent: 'flex-end',
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        style={{
          width,
          maxWidth: '92vw',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          background: 'var(--bg-1)',
          borderLeft: '1px solid var(--border-1)',
          boxShadow: '-24px 0 60px rgba(0,0,0,0.45)',
          animation: 'relay-slide-in 0.18s var(--ease) both',
        }}
      >
        <header
          style={{
            flexShrink: 0,
            padding: 'var(--s4)',
            borderBottom: '1px solid var(--border-0)',
            display: 'flex',
            alignItems: 'flex-start',
            gap: 'var(--s3)',
          }}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            {title}
            {subtitle}
          </div>
          <button
            onClick={onClose}
            title="Close"
            style={{ color: 'var(--tx-2)', padding: 4, borderRadius: 'var(--r)', flexShrink: 0 }}
          >
            <Icon name="x" size={16} />
          </button>
        </header>

        <div style={{ flex: 1, overflow: 'auto', padding: 'var(--s4)' }}>{children}</div>

        {footer && (
          <footer
            style={{
              flexShrink: 0,
              padding: 'var(--s4)',
              borderTop: '1px solid var(--border-1)',
              background: 'var(--bg-2)',
            }}
          >
            {footer}
          </footer>
        )}
      </div>
    </div>
  )
}
