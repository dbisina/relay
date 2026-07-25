// ChainBuilder.tsx — an ordered, reorderable list of providers. Shared between
// the profile editor (Agents & skills) and the Home composer's Providers chip,
// since "pick providers and put them in order" is the same interaction in both.

import React from 'react'
import { Icon } from './Icon'
import { ProviderGlyph } from './ui'
import { providerTitle } from '../lib/format'

function swap<T>(arr: T[], a: number, b: number): T[] {
  const c = [...arr]
  ;[c[a], c[b]] = [c[b], c[a]]
  return c
}

export function ChainBuilder({
  chain,
  available,
  onChange,
  emptyHint = 'No providers yet. Add them below in the order Relay should try them.',
}: {
  chain: string[]
  available: string[]
  onChange: (c: string[]) => void
  emptyHint?: string
}) {
  const remaining = available.filter((p) => !chain.includes(p))
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {chain.length === 0 && (
        <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>{emptyHint}</div>
      )}
      {chain.map((p, i) => (
        <div
          key={p}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--s2)',
            padding: '7px 10px',
            borderRadius: 'var(--r)',
            background: 'var(--bg-0)',
            border: '1px solid var(--border-1)',
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
          <ProviderGlyph name={p} size={22} />
          <span style={{ flex: 1, fontSize: 'var(--fz-md)', fontWeight: 500 }}>{providerTitle(p)}</span>
          <button
            onClick={() => i > 0 && onChange(swap(chain, i, i - 1))}
            disabled={i === 0}
            title="Move up"
            style={{ color: i === 0 ? 'var(--tx-3)' : 'var(--tx-1)', padding: 3, opacity: i === 0 ? 0.4 : 1 }}
          >
            <Icon name="chevronDown" size={14} style={{ transform: 'rotate(180deg)' }} />
          </button>
          <button
            onClick={() => i < chain.length - 1 && onChange(swap(chain, i, i + 1))}
            disabled={i === chain.length - 1}
            title="Move down"
            style={{
              color: i === chain.length - 1 ? 'var(--tx-3)' : 'var(--tx-1)',
              padding: 3,
              opacity: i === chain.length - 1 ? 0.4 : 1,
            }}
          >
            <Icon name="chevronDown" size={14} />
          </button>
          <button onClick={() => onChange(chain.filter((x) => x !== p))} title="Remove" style={{ color: 'var(--tx-2)', padding: 3 }}>
            <Icon name="x" size={14} />
          </button>
        </div>
      ))}
      {remaining.length > 0 && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 2 }}>
          {remaining.map((p) => (
            <button
              key={p}
              onClick={() => onChange([...chain, p])}
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
              <Icon name="plus" size={12} /> {providerTitle(p)}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
