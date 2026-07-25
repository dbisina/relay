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

const NAV: { group: string; items: { id: Route; labelKey: string; icon: IconName }[] }[] = [
  {
    group: 'Run',
    items: [
      { id: 'dashboard', labelKey: 'nav.home', icon: 'dashboard' },
      { id: 'detect', labelKey: 'nav.detect', icon: 'detect' },
      { id: 'workflow', labelKey: 'nav.workflow', icon: 'workflow' },
      { id: 'pipelines', labelKey: 'nav.pipelines', icon: 'pipelines' },
    ],
  },
  {
    group: 'Connect',
    items: [
      { id: 'accounts', labelKey: 'nav.accounts', icon: 'accounts' },
      { id: 'providers', labelKey: 'nav.providers', icon: 'providers' },
    ],
  },
  {
    group: 'Audit',
    items: [
      { id: 'history', labelKey: 'nav.history', icon: 'history' },
      { id: 'console', labelKey: 'nav.console', icon: 'key' },
      { id: 'settings', labelKey: 'nav.settings', icon: 'settings' },
    ],
  },
]

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

  return (
    <aside
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
      {NAV.map((section) => (
        <div key={section.group} style={{ marginBottom: 'var(--s4)' }}>
          <div
            style={{
              fontSize: 'var(--fz-xs)',
              fontWeight: 600,
              letterSpacing: '0.1em',
              textTransform: 'uppercase',
              color: 'var(--tx-3)',
              padding: '0 10px',
              marginBottom: 6,
            }}
          >
            {section.group}
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {section.items.map((item) => (
              <NavItem
                key={item.id}
                item={item}
                label={t(item.labelKey)}
                active={route === item.id}
                onClick={() => onNavigate(item.id)}
                badge={item.id === 'providers' ? readyCount : undefined}
              />
            ))}
          </div>
        </div>
      ))}

      <div style={{ flex: 1 }} />

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
