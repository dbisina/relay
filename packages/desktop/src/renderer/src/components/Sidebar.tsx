// Sidebar.tsx — primary navigation. Grouped, quiet, one accent marker on the
// active item. Shows a live handoff-ready count so the point of the app (relay
// work when an account runs dry) is always one glance away.

import React from 'react'
import { Icon, type IconName } from './Icon'
import { useStore } from '../lib/store'
import { providerTitle } from '../lib/format'
import { useLocale } from '../lib/i18n'

export type Route =
  | 'dashboard'
  | 'accounts'
  | 'detect'
  | 'workflow'
  | 'providers'
  | 'pipelines'
  | 'history'
  | 'console'
  | 'settings'

type NavEntry = { id: Route; labelKey: string; icon: IconName }

// The everyday four. A first-time, non-technical user sees only these: run a
// task, connect logins and providers, change settings. Everything else is real
// but rarely needed on day one, so it hides until asked for.
const ESSENTIALS: NavEntry[] = [
  { id: 'dashboard', labelKey: 'nav.home', icon: 'dashboard' },
  { id: 'accounts', labelKey: 'nav.accounts', icon: 'accounts' },
  { id: 'providers', labelKey: 'nav.providers', icon: 'providers' },
]

// The power-user screens, collapsed behind one disclosure. Nothing is removed,
// it is one click away, but it no longer greets a newcomer with jargon.
const ADVANCED: NavEntry[] = [
  { id: 'detect', labelKey: 'nav.detect', icon: 'detect' },
  { id: 'workflow', labelKey: 'nav.workflow', icon: 'workflow' },
  { id: 'pipelines', labelKey: 'nav.pipelines', icon: 'pipelines' },
  { id: 'history', labelKey: 'nav.history', icon: 'history' },
  { id: 'console', labelKey: 'nav.console', icon: 'key' },
]

const SETTINGS: NavEntry = { id: 'settings', labelKey: 'nav.settings', icon: 'settings' }

function NavItem({
  item,
  label,
  active,
  onClick,
  badge,
}: {
  item: { id: Route; labelKey: string; icon: IconName }
  label: string
  active: boolean
  onClick: () => void
  badge?: number
}) {
  const [hover, setHover] = React.useState(false)
  return (
    <button
      onClick={onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      aria-current={active ? 'page' : undefined}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        width: '100%',
        padding: '7px 10px',
        borderRadius: 'var(--r)',
        fontSize: 'var(--fz-md)',
        fontWeight: active ? 600 : 500,
        color: active ? 'var(--tx-0)' : hover ? 'var(--tx-1)' : 'var(--tx-2)',
        background: active ? 'var(--active)' : hover ? 'var(--hover)' : 'transparent',
        transition: 'background 0.12s, color 0.12s',
        position: 'relative',
      }}
    >
      <Icon name={item.icon} size={16} style={{ color: active ? 'var(--accent)' : 'inherit' }} />
      <span style={{ flex: 1, textAlign: 'left' }}>{label}</span>
      {badge != null && badge > 0 && (
        <span
          style={{
            fontSize: 'var(--fz-xs)',
            fontWeight: 700,
            fontFamily: 'var(--font-mono)',
            color: 'var(--accent)',
            background: 'var(--accent-weak)',
            borderRadius: 'var(--r-pill)',
            padding: '1px 6px',
          }}
        >
          {badge}
        </span>
      )}
    </button>
  )
}

export function Sidebar({ route, onNavigate }: { route: Route; onNavigate: (r: Route) => void }) {
  const { session, providers } = useStore()
  const { t } = useLocale()
  // Accounts/providers that still have headroom = places work can relay to.
  const readyCount = providers.filter((p) => p.state === 'standby' || p.state === 'active').length

  const routeIsAdvanced = ADVANCED.some((i) => i.id === route)
  const [advancedOpen, setAdvancedOpen] = React.useState(
    () => localStorage.getItem('relay.nav.advanced') === '1',
  )
  // Following a link into an advanced screen expands the section, so the active
  // item is never hidden from the person looking straight at it.
  const showAdvanced = advancedOpen || routeIsAdvanced

  const item = (entry: NavEntry) => (
    <NavItem
      key={entry.id}
      item={entry}
      label={t(entry.labelKey)}
      active={route === entry.id}
      onClick={() => onNavigate(entry.id)}
      badge={entry.id === 'providers' ? readyCount : undefined}
    />
  )

  return (
    <aside
      aria-label="Primary"
      style={{
        width: 'var(--sidebar-w)',
        flexShrink: 0,
        background: 'var(--bg-1)',
        borderRight: '1px solid var(--border-0)',
        display: 'flex',
        flexDirection: 'column',
        padding: 'var(--s3) var(--s3) var(--s2)',
        overflow: 'auto',
      }}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>{ESSENTIALS.map(item)}</div>

      <button
        onClick={() => {
          const next = !showAdvanced
          setAdvancedOpen(next)
          localStorage.setItem('relay.nav.advanced', next ? '1' : '0')
        }}
        aria-expanded={showAdvanced}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          width: '100%',
          marginTop: 'var(--s4)',
          padding: '6px 10px',
          borderRadius: 'var(--r)',
          fontSize: 'var(--fz-xs)',
          fontWeight: 600,
          letterSpacing: '0.1em',
          textTransform: 'uppercase',
          color: 'var(--tx-3)',
        }}
      >
        <Icon
          name="chevronRight"
          size={12}
          style={{ transform: showAdvanced ? 'rotate(90deg)' : 'none', transition: 'transform 0.15s var(--ease)' }}
        />
        Advanced
      </button>

      {showAdvanced && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2, marginTop: 4 }}>
          {ADVANCED.map(item)}
        </div>
      )}

      <div style={{ flex: 1, minHeight: 'var(--s4)' }} />

      <div style={{ display: 'flex', flexDirection: 'column', gap: 2, marginBottom: 'var(--s3)' }}>
        {item(SETTINGS)}
      </div>

      {session && (
        <div
          style={{
            padding: '10px 11px',
            borderRadius: 'var(--r)',
            border: '1px solid var(--border-0)',
            background: 'var(--bg-0)',
          }}
        >
          <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginBottom: 4 }}>
            Active session
          </div>
          <div
            style={{
              fontSize: 'var(--fz-sm)',
              fontWeight: 600,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {providerTitle(session.activeProvider) || 'idle'}
          </div>
          <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)', marginTop: 2 }}>
            {session.fsmState || 'ready'} · {session.handoffsDone} handoffs
          </div>
        </div>
      )}
    </aside>
  )
}
