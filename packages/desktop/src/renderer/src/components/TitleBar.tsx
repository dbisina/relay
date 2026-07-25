// TitleBar.tsx — custom frameless title bar. The whole strip is a drag region
// except the interactive controls (which opt out via WebkitAppRegion: no-drag).

import React from 'react'
import { Icon } from './Icon'
import { useStore } from '../lib/store'
import { StatusDot } from './ui'
import { useLocale } from '../lib/i18n'
import logoUrl from '../assets/logo.png'

const noDrag: React.CSSProperties = { WebkitAppRegion: 'no-drag' } as React.CSSProperties

function ConnPill() {
  const { conn, connDetail, reconnect } = useStore()
  const { t } = useLocale()
  const map = {
    up: { tone: 'green' as const, label: t('conn.up') },
    checking: { tone: 'yellow' as const, label: t('conn.checking') },
    starting: { tone: 'yellow' as const, label: t('conn.starting') },
    unreachable: { tone: 'red' as const, label: t('conn.unreachable') },
  }
  const m = map[conn]
  return (
    <button
      onClick={conn === 'unreachable' ? reconnect : undefined}
      title={connDetail || m.label}
      style={{
        ...noDrag,
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        padding: '3px 9px',
        borderRadius: 'var(--r-pill)',
        border: '1px solid var(--border-1)',
        fontSize: 'var(--fz-xs)',
        color: 'var(--tx-1)',
        cursor: conn === 'unreachable' ? 'pointer' : 'default',
      }}
    >
      <StatusDot tone={m.tone} pulse={conn === 'checking' || conn === 'starting'} />
      {m.label}
    </button>
  )
}

function WinButton({ icon, onClick, danger }: { icon: 'minus' | 'maximize' | 'x'; onClick: () => void; danger?: boolean }) {
  const [hover, setHover] = React.useState(false)
  return (
    <button
      onClick={onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        ...noDrag,
        width: 44,
        height: 'var(--titlebar-h)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: hover ? (danger ? '#fff' : 'var(--tx-0)') : 'var(--tx-2)',
        background: hover ? (danger ? 'var(--red)' : 'var(--hover)') : 'transparent',
        transition: 'background 0.12s, color 0.12s',
      }}
    >
      <Icon name={icon} size={icon === 'maximize' ? 11 : 14} />
    </button>
  )
}

export function TitleBar() {
  return (
    <div
      style={
        {
          height: 'var(--titlebar-h)',
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'var(--bg-1)',
          borderBottom: '1px solid var(--border-0)',
          WebkitAppRegion: 'drag',
          paddingLeft: 78, // clear the macOS traffic lights
        } as React.CSSProperties
      }
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
          <img
            src={logoUrl}
            alt=""
            width={16}
            height={16}
            style={{ display: 'block', flexShrink: 0 }}
          />
          <span
            style={{
              fontSize: 'var(--fz-xs)',
              fontWeight: 700,
              letterSpacing: '0.28em',
              color: 'var(--tx-0)',
            }}
          >
            RELAY
          </span>
        </div>
        <ConnPill />
      </div>
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <WinButton icon="minus" onClick={() => window.relay.win.minimize()} />
        <WinButton icon="maximize" onClick={() => window.relay.win.maximizeToggle()} />
        <WinButton icon="x" onClick={() => window.relay.win.close()} danger />
      </div>
    </div>
  )
}
