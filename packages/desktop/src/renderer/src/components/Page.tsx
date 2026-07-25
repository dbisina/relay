// Page.tsx — consistent screen scaffold: a sticky header (title, subtitle,
// actions) over a padded, max-width body so long screens stay readable.

import React from 'react'

export function Page({
  title,
  subtitle,
  actions,
  children,
}: {
  title: string
  subtitle?: string
  actions?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div style={{ minHeight: '100%' }}>
      <header
        style={{
          position: 'sticky',
          top: 0,
          zIndex: 10,
          display: 'flex',
          alignItems: 'flex-end',
          justifyContent: 'space-between',
          gap: 'var(--s4)',
          padding: 'var(--s5) var(--s6) var(--s4)',
          background: 'linear-gradient(var(--bg-0) 72%, transparent)',
        }}
      >
        <div>
          <h1 style={{ fontSize: 'var(--fz-2xl)', fontWeight: 700, letterSpacing: '-0.01em' }}>
            {title}
          </h1>
          {subtitle && (
            <p style={{ fontSize: 'var(--fz-md)', color: 'var(--tx-2)', marginTop: 4, maxWidth: 620, lineHeight: 1.5 }}>
              {subtitle}
            </p>
          )}
        </div>
        {actions && <div style={{ display: 'flex', gap: 'var(--s2)', flexShrink: 0 }}>{actions}</div>}
      </header>
      <div style={{ padding: '0 var(--s6) var(--s7)', maxWidth: 960 }}>{children}</div>
    </div>
  )
}
