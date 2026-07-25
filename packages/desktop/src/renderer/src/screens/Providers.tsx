// Providers.tsx — enable the agents Relay can rotate through, and get each one
// authenticated. Install a CLI, launch an OAuth sign-in, or paste an API key you
// own. No credentials are stored by this window; auth happens in the provider.

import React, { useState } from 'react'
import { Page } from '../components/Page'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { providerTitle } from '../lib/format'
import { Button, Input, Badge, StatusDot, ProviderGlyph, EmptyState, Spinner } from '../components/ui'
import { Icon } from '../components/Icon'
import type { ProviderDetail } from '../lib/types'

function Toggle({ on, onClick }: { on: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      role="switch"
      aria-checked={on}
      style={{
        width: 38,
        height: 22,
        borderRadius: 'var(--r-pill)',
        background: on ? 'var(--accent)' : 'var(--bg-3)',
        border: '1px solid ' + (on ? 'transparent' : 'var(--border-2)'),
        position: 'relative',
        transition: 'background 0.16s var(--ease)',
        flexShrink: 0,
      }}
    >
      <span
        style={{
          position: 'absolute',
          top: 2,
          left: on ? 18 : 2,
          width: 16,
          height: 16,
          borderRadius: '50%',
          background: on ? 'var(--accent-ink)' : 'var(--tx-2)',
          transition: 'left 0.16s var(--ease)',
        }}
      />
    </button>
  )
}

function probeBadge(d: ProviderDetail) {
  const m: Record<string, { tone: 'green' | 'yellow' | 'red' | 'neutral'; label: string }> = {
    available: { tone: 'green', label: 'Ready' },
    no_key: { tone: 'yellow', label: 'Needs auth' },
    authenticating: { tone: 'yellow', label: 'Authenticating' },
    installing: { tone: 'yellow', label: 'Installing' },
    not_found: { tone: 'red', label: 'Not installed' },
    unavailable: { tone: 'red', label: 'Unavailable' },
    manual: { tone: 'neutral', label: 'Manual' },
  }
  const x = m[d.probeStatus] || { tone: 'neutral' as const, label: d.probeStatus }
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
      <StatusDot tone={x.tone} />
      <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)' }}>{x.label}</span>
    </span>
  )
}

function ProviderRow({ d }: { d: ProviderDetail }) {
  const toast = useToast()
  const { refresh } = useStore()
  const [open, setOpen] = useState(false)
  const [keyVal, setKeyVal] = useState('')
  const [busy, setBusy] = useState<string | null>(null)

  const run = async (tag: string, fn: () => Promise<{ ok: boolean; error?: string }>, okMsg: string) => {
    setBusy(tag)
    const r = await fn()
    setBusy(null)
    if (r.ok) {
      toast(okMsg, 'ok')
      setTimeout(refresh, 400)
    } else toast(r.error || 'Action failed', 'error')
  }

  const toggle = () =>
    run('toggle', () => api.saveProviderConfig({ name: d.name, enabled: !d.enabled }), `${providerTitle(d.name)} ${d.enabled ? 'disabled' : 'enabled'}`)

  return (
    <div
      style={{
        border: '1px solid var(--border-0)',
        borderRadius: 'var(--r-lg)',
        background: 'var(--bg-1)',
        marginBottom: 'var(--s3)',
        opacity: d.enabled ? 1 : 0.72,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--s3)', padding: 'var(--s3) var(--s4)' }}>
        <ProviderGlyph name={d.name} size={34} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 'var(--fz-lg)', fontWeight: 600 }}>{d.displayName || providerTitle(d.name)}</span>
            <Badge tone="neutral">{d.kind}</Badge>
            {probeBadge(d)}
          </div>
          <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 460 }}>
            {d.description || d.setupHint}
          </div>
        </div>
        <button onClick={() => setOpen((v) => !v)} title="Configure" style={{ color: 'var(--tx-2)', padding: 6, display: 'flex' }}>
          <Icon name="chevronDown" size={16} style={{ transform: open ? 'rotate(180deg)' : 'none', transition: 'transform 0.16s' }} />
        </button>
        <Toggle on={d.enabled} onClick={toggle} />
      </div>

      {open && (
        <div style={{ padding: '0 var(--s4) var(--s4)', display: 'flex', flexDirection: 'column', gap: 'var(--s3)' }}>
          <div style={{ height: 1, background: 'var(--border-0)' }} />

          <div style={{ display: 'flex', gap: 'var(--s2)', flexWrap: 'wrap' }}>
            {d.canInstall && d.probeStatus === 'not_found' && (
              <Button variant="default" icon="download" loading={busy === 'install'} onClick={() => run('install', () => api.installProvider(d.name), `Installing ${providerTitle(d.name)}…`)}>
                Install CLI
              </Button>
            )}
            {d.canOauth && (
              <Button variant="default" icon="key" loading={busy === 'oauth'} onClick={() => run('oauth', () => api.oauthProvider(d.name), `Launched sign-in for ${providerTitle(d.name)}. Finish it in the browser.`)}>
                Sign in with OAuth
              </Button>
            )}
            {d.setupUrl && (
              <Button variant="ghost" icon="external" onClick={() => window.relay.openExternal(d.setupUrl)}>
                Setup guide
              </Button>
            )}
          </div>

          {d.canApiKey && (
            <div>
              <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--tx-2)', marginBottom: 6 }}>
                API key{d.apiKeyEnvVar ? ` (${d.apiKeyEnvVar})` : ''}
                {d.apiKeySet && <Badge tone="green" style={{ marginLeft: 8 }}>Set</Badge>}
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                <Input value={keyVal} onChange={setKeyVal} placeholder={d.apiKeySet ? 'Replace stored key…' : 'Paste your API key'} type="password" mono />
                <Button
                  variant="primary"
                  loading={busy === 'key'}
                  onClick={() => {
                    if (!keyVal.trim()) return
                    run('key', () => api.apiKeyProvider(d.name, keyVal.trim()), `Saved API key for ${providerTitle(d.name)}`).then(() => setKeyVal(''))
                  }}
                >
                  Save
                </Button>
                {d.apiKeyUrl && (
                  <Button variant="ghost" icon="external" onClick={() => window.relay.openExternal(d.apiKeyUrl!)} title="Get a key" />
                )}
              </div>
              <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 5 }}>
                Stored by the daemon in your local config. This window never reads it back.
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export function Providers() {
  const { details, conn } = useStore()
  if (conn !== 'up' && details.length === 0) {
    return (
      <Page title="Providers">
        <div style={{ padding: 'var(--s6)', display: 'flex', justifyContent: 'center' }}>
          <Spinner size={22} />
        </div>
      </Page>
    )
  }
  const enabled = details.filter((d) => d.enabled)
  const rest = details.filter((d) => !d.enabled)
  return (
    <Page
      title="Providers"
      subtitle="Turn on the agents Relay can rotate through and get each authenticated. Order comes from Relay's built-in priority, filtered to whichever are enabled."
    >
      {details.length === 0 ? (
        <EmptyState icon="providers" title="No providers reported" body="The daemon hasn't returned any provider metadata yet." />
      ) : (
        <>
          {enabled.length > 0 && (
            <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--tx-3)', marginBottom: 'var(--s3)' }}>
              Enabled
            </div>
          )}
          {enabled.map((d) => (
            <ProviderRow key={d.name} d={d} />
          ))}
          {rest.length > 0 && (
            <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--tx-3)', margin: 'var(--s4) 0 var(--s3)' }}>
              Available
            </div>
          )}
          {rest.map((d) => (
            <ProviderRow key={d.name} d={d} />
          ))}
        </>
      )}
    </Page>
  )
}
