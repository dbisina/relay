// Accounts.tsx — the heart of the app. One place to keep several logins per
// provider and relay work between them without friction.
//
// Safety by design: Relay never creates accounts or handles passwords. "Add"
// registers an isolated profile slot; "Sign in" launches the provider's OWN
// login (browser/CLI) which the user completes. Each account is a separate,
// isolated credential store so multiple logins on one provider coexist.

import React, { useEffect, useState } from 'react'
import { Page } from '../components/Page'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { providerTitle, pct, etaLabel, quotaLabel } from '../lib/format'
import {
  Button,
  Input,
  Field,
  Badge,
  StatusDot,
  Meter,
  ProviderGlyph,
  EmptyState,
  Spinner,
} from '../components/ui'
import { Icon } from '../components/Icon'
import type { ProviderDetail, AccountDto, WalletEntry } from '../lib/types'

function probeTone(status: string): { tone: 'green' | 'yellow' | 'red' | 'neutral'; label: string } {
  switch (status) {
    case 'available':
      return { tone: 'green', label: 'Ready' }
    case 'authenticating':
    case 'installing':
      return { tone: 'yellow', label: 'Working' }
    case 'no_key':
      return { tone: 'yellow', label: 'Needs auth' }
    case 'not_found':
    case 'unavailable':
      return { tone: 'red', label: 'Unavailable' }
    default:
      return { tone: 'neutral', label: status || 'Manual' }
  }
}

function AccountRow({
  provider,
  acct,
  wallet,
  busy,
  onSwitch,
  onLogin,
  onRemove,
}: {
  provider: string
  acct: AccountDto
  wallet?: WalletEntry
  busy: boolean
  onSwitch: () => void
  onLogin: () => void
  onRemove: () => void
}) {
  const [hover, setHover] = useState(false)
  const frac = wallet ? wallet.fractionUsed : undefined
  return (
    <div
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 'var(--s3)',
        padding: '10px 12px',
        borderRadius: 'var(--r)',
        background: acct.active ? 'var(--accent-weak)' : hover ? 'var(--hover)' : 'transparent',
        border: `1px solid ${acct.active ? 'var(--accent-line)' : 'transparent'}`,
        transition: 'background 0.12s',
      }}
    >
      <button
        onClick={acct.active ? undefined : onSwitch}
        title={acct.active ? 'Active account' : 'Make active'}
        style={{
          width: 18,
          height: 18,
          borderRadius: '50%',
          border: `1.5px solid ${acct.active ? 'var(--accent)' : 'var(--border-2)'}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexShrink: 0,
          cursor: acct.active ? 'default' : 'pointer',
        }}
      >
        {acct.active && (
          <span style={{ width: 9, height: 9, borderRadius: '50%', background: 'var(--accent)' }} />
        )}
      </button>

      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontSize: 'var(--fz-md)', fontWeight: 600 }}>{acct.label}</span>
          {acct.active && <Badge tone="accent">Active</Badge>}
        </div>
        <div
          style={{
            fontSize: 'var(--fz-xs)',
            color: 'var(--tx-3)',
            fontFamily: 'var(--font-mono)',
            marginTop: 2,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            maxWidth: 320,
          }}
        >
          {acct.configDir || 'default profile'}
        </div>
      </div>

      {frac != null && (
        <div style={{ width: 120, flexShrink: 0 }}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              fontSize: 'var(--fz-xs)',
              color: 'var(--tx-2)',
              marginBottom: 4,
            }}
          >
            <span title={`${pct(frac)} used`}>{quotaLabel(frac)}</span>
            <span style={{ color: 'var(--tx-3)' }}>{etaLabel(wallet!.etaMinutes)}</span>
          </div>
          <Meter fraction={frac} />
        </div>
      )}

      <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
        <Button size="sm" variant="ghost" icon="key" loading={busy} onClick={onLogin} title="Launch provider sign-in">
          Sign in
        </Button>
        <Button size="sm" variant="ghost" icon="x" onClick={onRemove} title="Remove account" />
      </div>
    </div>
  )
}

function AddAccountForm({ provider, onDone }: { provider: string; onDone: () => void }) {
  const toast = useToast()
  const { refresh } = useStore()
  const [label, setLabel] = useState('')
  const [dir, setDir] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    if (!label.trim()) return
    setBusy(true)
    const r = await api.addAccount(provider, label.trim(), dir.trim())
    setBusy(false)
    if (r.ok) {
      toast(`Added "${label}" to ${providerTitle(provider)}. Click Sign in to authenticate.`, 'ok')
      await refresh()
      onDone()
    } else {
      toast(r.error || 'Could not add account', 'error')
    }
  }

  return (
    <div
      style={{
        marginTop: 8,
        padding: 'var(--s3)',
        borderRadius: 'var(--r)',
        background: 'var(--bg-0)',
        border: '1px solid var(--border-1)',
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--s3)',
      }}
    >
      <Field label="Account label" hint="A name you'll recognise, e.g. personal, work, pro-plan.">
        <Input value={label} onChange={setLabel} placeholder="work" autoFocus onEnter={submit} />
      </Field>
      <Field
        label="Isolated profile folder (optional)"
        hint="Keeps this login separate so several accounts on one provider don't clobber each other. Leave blank for the default."
      >
        <div style={{ display: 'flex', gap: 6 }}>
          <Input value={dir} onChange={setDir} placeholder="(default)" mono />
          <Button
            variant="default"
            icon="folder"
            onClick={async () => {
              const p = await window.relay.pickFolder()
              if (p) setDir(p)
            }}
          >
            Browse
          </Button>
        </div>
      </Field>
      <div style={{ display: 'flex', gap: 'var(--s2)' }}>
        <Button variant="primary" icon="plus" loading={busy} onClick={submit}>
          Add account
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

function ProviderAccounts({
  detail,
  wallet,
}: {
  detail: ProviderDetail
  wallet: WalletEntry[]
}) {
  const toast = useToast()
  const { refresh } = useStore()
  const [adding, setAdding] = useState(false)
  const [busyLabel, setBusyLabel] = useState<string | null>(null)
  const probe = probeTone(detail.probeStatus)

  const walletFor = (label: string) =>
    wallet.find((w) => w.provider === detail.name && w.account === label)

  const act = async (label: string, fn: () => Promise<{ ok: boolean; error?: string }>, ok: string) => {
    setBusyLabel(label)
    const r = await fn()
    setBusyLabel(null)
    if (r.ok) {
      toast(ok, 'ok')
      refresh()
    } else {
      toast(r.error || 'Action failed', 'error')
    }
  }

  return (
    <div
      style={{
        border: '1px solid var(--border-0)',
        borderRadius: 'var(--r-lg)',
        background: 'var(--bg-1)',
        marginBottom: 'var(--s3)',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--s3)',
          padding: 'var(--s3) var(--s4)',
          borderBottom: detail.accounts.length || adding ? '1px solid var(--border-0)' : 'none',
        }}
      >
        <ProviderGlyph name={detail.name} size={32} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 'var(--fz-lg)', fontWeight: 600 }}>
              {detail.displayName || providerTitle(detail.name)}
            </span>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
              <StatusDot tone={probe.tone} />
              <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)' }}>{probe.label}</span>
            </span>
          </div>
          <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 2 }}>
            {detail.accounts.length
              ? `${detail.accounts.length} account${detail.accounts.length > 1 ? 's' : ''}`
              : 'No accounts yet'}
          </div>
        </div>
        <Button size="sm" variant="default" icon="plus" onClick={() => setAdding((v) => !v)}>
          Add account
        </Button>
      </div>

      <div style={{ padding: detail.accounts.length || adding ? 'var(--s2) var(--s3) var(--s3)' : 0 }}>
        {detail.accounts.map((a) => (
          <AccountRow
            key={a.label}
            provider={detail.name}
            acct={a}
            wallet={walletFor(a.label)}
            busy={busyLabel === a.label}
            onSwitch={() =>
              act(a.label, () => api.switchAccount(detail.name, a.label), `Switched to "${a.label}"`)
            }
            onLogin={() =>
              act(
                a.label,
                () => api.loginAccount(detail.name, a.label),
                `Launched sign-in for "${a.label}". Finish it in the window that opens.`,
              )
            }
            onRemove={() =>
              act(a.label, () => api.removeAccount(detail.name, a.label), `Removed "${a.label}"`)
            }
          />
        ))}
        {adding && <AddAccountForm provider={detail.name} onDone={() => setAdding(false)} />}
      </div>
    </div>
  )
}

export function Accounts() {
  const { details, wallet, conn } = useStore()
  const [loaded, setLoaded] = useState(false)
  useEffect(() => {
    if (conn === 'up') setLoaded(true)
  }, [conn, details])

  // Show providers that can hold accounts (CLI/API auth), enabled first.
  const accountable = details
    .filter((d) => d.canOauth || d.canApiKey || d.kind === 'cli' || d.accounts.length > 0)
    .sort((a, b) => Number(b.enabled) - Number(a.enabled) || b.accounts.length - a.accounts.length)

  return (
    <Page
      title="Accounts"
      subtitle="Keep several logins per provider and relay work between them the moment one hits its limit. Relay launches each provider's own sign-in; it never sees your password."
    >
      {!loaded ? (
        <div style={{ padding: 'var(--s6)', display: 'flex', justifyContent: 'center' }}>
          <Spinner size={22} />
        </div>
      ) : accountable.length === 0 ? (
        <EmptyState
          icon="accounts"
          title="No account-capable providers yet"
          body="Enable a provider on the Providers screen first, then come back to add and sign in to your accounts."
        />
      ) : (
        <>
          <div
            style={{
              display: 'flex',
              gap: 'var(--s2)',
              padding: '10px 12px',
              borderRadius: 'var(--r)',
              background: 'var(--bg-1)',
              border: '1px solid var(--border-0)',
              marginBottom: 'var(--s4)',
              alignItems: 'flex-start',
            }}
          >
            <Icon name="handoff" size={16} style={{ color: 'var(--accent)', marginTop: 1 }} />
            <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', lineHeight: 1.5 }}>
              The <strong style={{ color: 'var(--tx-1)' }}>active</strong> account per provider is
              used first. When it runs dry, Relay signs a continuation contract and rotates to the
              next available account or provider, so a task keeps moving without a manual copy-paste.
            </div>
          </div>
          {accountable.map((d) => (
            <ProviderAccounts key={d.name} detail={d} wallet={wallet} />
          ))}
        </>
      )}
    </Page>
  )
}
