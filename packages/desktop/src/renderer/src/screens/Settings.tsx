// Settings.tsx — daemon status, run defaults, and honest pointers out. Nothing
// here holds secrets; the daemon owns config. Defaults are stored per-window.

import React, { useState } from 'react'
import { Page } from '../components/Page'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { Button, Field, Input, Badge, StatusDot } from '../components/ui'
import { Icon } from '../components/Icon'
import { ACCENT_PRESETS, saveAccent } from '../lib/theme'
import { LOCALES, useLocale, setLocale } from '../lib/i18n'

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 0', borderBottom: '1px solid var(--border-0)' }}>
      <span style={{ fontSize: 'var(--fz-md)', color: 'var(--tx-1)' }}>{label}</span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>{children}</div>
    </div>
  )
}

export function Settings() {
  const toast = useToast()
  const { locale } = useLocale()
  const { conn, providers, details } = useStore()
  const [threshold, setThreshold] = useState(() => localStorage.getItem('relay.threshold') || '0.85')
  const [accent, setAccent] = useState(() => localStorage.getItem('relay.accent') || 'orange')

  const saveThreshold = () => {
    const n = Number(threshold)
    if (!isFinite(n) || n <= 0 || n > 1) {
      toast('Threshold must be between 0 and 1', 'error')
      return
    }
    localStorage.setItem('relay.threshold', String(n))
    toast('Default handoff threshold saved', 'ok')
  }

  return (
    <Page title="Settings" subtitle="The daemon is the source of truth. These are window defaults and pointers out.">
      <div style={{ maxWidth: 560 }}>
        <SectionLabel>Daemon</SectionLabel>
        <Row label="Connection">
          <StatusDot tone={conn === 'up' ? 'green' : conn === 'failed' ? 'red' : 'yellow'} />
          <span style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)' }}>{conn === 'up' ? 'Connected' : conn}</span>
        </Row>
        <Row label="Endpoint">
          <code style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-1)' }}>127.0.0.1:4748</code>
        </Row>
        <Row label="Providers enabled">
          <Badge tone="neutral">{details.filter((d) => d.enabled).length} / {details.length}</Badge>
        </Row>
        <Row label="Available now">
          <Badge tone="green">{providers.filter((p) => p.state !== 'exhausted' && p.state !== 'error').length}</Badge>
        </Row>

        <div style={{ height: 'var(--s5)' }} />
        <SectionLabel>Run defaults</SectionLabel>
        <Field label="Handoff threshold" hint="Hand off automatically when the active account crosses this fraction of its limit (0 to 1).">
          <div style={{ display: 'flex', gap: 6, maxWidth: 220 }}>
            <Input value={threshold} onChange={setThreshold} onEnter={saveThreshold} mono />
            <Button variant="primary" onClick={saveThreshold}>Save</Button>
          </div>
        </Field>

        <div style={{ height: 'var(--s5)' }} />
        <SectionLabel>Appearance</SectionLabel>
        <Field label="Accent color" style={{ padding: '10px 0' }}>
          <div style={{ display: 'flex', gap: 8 }}>
            {ACCENT_PRESETS.map((p) => (
              <button
                key={p.id}
                onClick={() => {
                  setAccent(p.id)
                  saveAccent(p.id)
                }}
                title={p.label}
                style={{
                  width: 30,
                  height: 30,
                  borderRadius: '50%',
                  background: p.accent,
                  border: accent === p.id ? '2px solid var(--tx-0)' : '2px solid transparent',
                  boxShadow: accent === p.id ? '0 0 0 2px var(--bg-0)' : 'none',
                }}
              />
            ))}
          </div>
        </Field>
        <Field label="Language" style={{ padding: '10px 0' }}>
          <select
            value={locale}
            onChange={(e) => setLocale(e.target.value as (typeof LOCALES)[number]['code'])}
            style={{ height: 34, padding: '0 10px', borderRadius: 'var(--r)', background: 'var(--bg-0)', border: '1px solid var(--border-1)', color: 'var(--tx-0)', fontSize: 'var(--fz-md)' }}
          >
            {LOCALES.map((l) => (
              <option key={l.code} value={l.code}>
                {l.label}
              </option>
            ))}
          </select>
        </Field>

        <div style={{ height: 'var(--s5)' }} />
        <SectionLabel>First run</SectionLabel>
        <Row label="Setup walkthrough">
          <Button
            variant="ghost"
            icon="refresh"
            onClick={() => {
              localStorage.removeItem('relay.onboarded')
              toast('Onboarding will show again next launch', 'info')
            }}
          >
            Replay onboarding
          </Button>
        </Row>

        <div style={{ height: 'var(--s5)' }} />
        <SectionLabel>About</SectionLabel>
        <div style={{ display: 'flex', gap: 'var(--s2)', flexWrap: 'wrap', marginTop: 'var(--s2)' }}>
          <Button variant="ghost" icon="external" onClick={() => window.relay.openExternal('https://dbisina.github.io/relay')}>Documentation</Button>
          <Button variant="ghost" icon="external" onClick={() => window.relay.openExternal('https://github.com/dbisina/relay')}>Source</Button>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 'var(--s4)', color: 'var(--tx-3)', fontSize: 'var(--fz-xs)' }}>
          <Icon name="handoff" size={13} />
          Relay desktop · a vendor-neutral cockpit for multi-agent coding handoff.
        </div>
      </div>
    </Page>
  )
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--tx-3)', marginBottom: 4 }}>
      {children}
    </div>
  )
}
