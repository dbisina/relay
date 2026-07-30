// components/onboarding/StepProviders.tsx — step 2, and the one that matters.
// Turn on the coding tools you have, then add one entry per login you own for
// each of them. Relay keeps every login in its own folder, which is how it can
// move a task from your first account to your second when the first runs out.
//
// Every write here goes straight to the daemon (nothing is staged for later),
// so each control that needs the daemon says "waiting" in place while it boots.

import React, { useEffect, useRef, useState } from 'react'
import { api } from '../../lib/api'
import { useStore } from '../../lib/store'
import { useToast } from '../../lib/toast'
import { providerTitle } from '../../lib/format'
import { Button, Input, Field, Badge, ProviderGlyph, Spinner, StatusDot } from '../ui'
import { Icon } from '../Icon'
import type { ProviderDetail } from '../../lib/types'
import { SelectRow, Note, StepHead, WaitingNote, useDaemonReady } from './wizard'

/** Probe status in words a non-developer can act on. */
function probeLine(d: ProviderDetail): string {
  switch (d.probeStatus) {
    case 'available':
      return 'Installed on this computer and ready to use.'
    case 'no_key':
      return 'Installed. It still needs you to sign in.'
    case 'not_found':
      return 'Not installed yet. You can turn it on now and install it later.'
    case 'unavailable':
      return 'Not available on this computer.'
    case 'installing':
      return 'Installing right now.'
    case 'authenticating':
      return 'Signing in right now.'
    default:
      return d.description || 'Turn this on to include it in the rotation.'
  }
}

// ── Add-account form ─────────────────────────────────────────────────────────

function AddAccount({ detail, onDone }: { detail: ProviderDetail; onDone: () => void }) {
  const toast = useToast()
  const { refresh } = useStore()
  const { ready } = useDaemonReady()
  const name = providerTitle(detail.name)
  const [label, setLabel] = useState(`Account ${detail.accounts.length + 1}`)
  const [where, setWhere] = useState<'default' | 'custom'>('default')
  const [dir, setDir] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (signIn: boolean) => {
    const clean = label.trim()
    if (!clean) {
      toast('Give this login a name first', 'error')
      return
    }
    if (where === 'custom' && !dir.trim()) {
      toast('Choose a folder, or switch back to the default', 'error')
      return
    }
    setBusy(true)
    // Empty configDir is meaningful: the daemon picks the standard folder for
    // this provider and keeps the login there. Do not invent a path here.
    const added = await api.addAccount(detail.name, clean, where === 'custom' ? dir.trim() : '')
    if (!added.ok) {
      setBusy(false)
      toast(added.error || 'Could not add this login', 'error')
      return
    }
    if (signIn) {
      const login = await api.loginAccount(detail.name, clean)
      if (!login.ok) {
        setBusy(false)
        await refresh()
        toast(login.error || `Added "${clean}", but the sign-in window did not open`, 'error')
        onDone()
        return
      }
      toast(`Added "${clean}". Finish signing in to ${name} in the window that opens.`, 'ok')
    } else {
      toast(`Added "${clean}" to ${name}.`, 'ok')
    }
    setBusy(false)
    await refresh()
    onDone()
  }

  return (
    <div
      style={{
        marginTop: 'var(--s2)',
        padding: 'var(--s3)',
        borderRadius: 'var(--r)',
        background: 'var(--bg-0)',
        border: '1px solid var(--border-1)',
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--s3)',
      }}
    >
      <Field
        label="Name this login"
        hint={`Anything you will recognise later, like Account 1, Work, or Personal. It is only a label, not your ${name} username.`}
      >
        <Input value={label} onChange={setLabel} placeholder="Account 1" autoFocus />
      </Field>

      <div>
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
          Where this login lives
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <SelectRow
            selected={where === 'default'}
            onClick={() => setWhere('default')}
            title="Let Relay handle it"
            subtitle={`Recommended. Relay keeps this ${name} login in its own folder, separate from your other ${name} logins, so signing in to one never signs you out of another.`}
          />
          <SelectRow
            selected={where === 'custom'}
            onClick={() => setWhere('custom')}
            title="Choose a folder myself"
            subtitle={`Point at a folder where you are already signed in to ${name}, or pick where this login should be kept.`}
          />
        </div>
        {where === 'custom' && (
          <div style={{ display: 'flex', gap: 6, marginTop: 'var(--s2)' }}>
            <Input value={dir} onChange={setDir} placeholder="No folder chosen yet" mono />
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
        )}
      </div>

      <div style={{ display: 'flex', gap: 'var(--s2)', alignItems: 'center', flexWrap: 'wrap' }}>
        <Button variant="primary" icon="key" loading={busy} disabled={!ready} onClick={() => submit(true)}>
          Add and sign in
        </Button>
        <Button variant="ghost" loading={busy} disabled={!ready} onClick={() => submit(false)}>
          Add without signing in
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
        <WaitingNote />
      </div>

      <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', lineHeight: 1.5 }}>
        Signing in happens in {name}&apos;s own window. Relay never sees or stores your password.
      </div>
    </div>
  )
}

// ── One provider: the switch, then its logins ────────────────────────────────

function ProviderBlock({ detail }: { detail: ProviderDetail }) {
  const toast = useToast()
  const { refresh } = useStore()
  const { ready } = useDaemonReady()
  const [pending, setPending] = useState<boolean | null>(null)
  const [busyToggle, setBusyToggle] = useState(false)
  const [busyLabel, setBusyLabel] = useState('')
  const [adding, setAdding] = useState(false)

  const name = providerTitle(detail.name)
  const on = pending ?? detail.enabled

  const toggle = async () => {
    if (!ready) return
    const next = !on
    setPending(next)
    setBusyToggle(true)
    const r = await api.saveProviderConfig({ name: detail.name, enabled: next })
    setBusyToggle(false)
    if (!r.ok) {
      setPending(null)
      toast(r.error || `Could not turn ${next ? 'on' : 'off'} ${name}`, 'error')
      return
    }
    await refresh()
    setPending(null)
    if (next) setAdding(detail.accounts.length === 0)
  }

  const signIn = async (label: string) => {
    setBusyLabel(label)
    const r = await api.loginAccount(detail.name, label)
    setBusyLabel('')
    if (r.ok) toast(`Opening the ${name} sign-in for "${label}".`, 'ok')
    else toast(r.error || 'Could not open the sign-in window', 'error')
  }

  return (
    <div style={{ marginBottom: 'var(--s2)' }}>
      <SelectRow
        mark="check"
        selected={on}
        disabled={!ready}
        onClick={toggle}
        glyph={<ProviderGlyph name={detail.name} size={30} />}
        title={detail.displayName || name}
        subtitle={probeLine(detail)}
        right={
          busyToggle ? (
            <Spinner size={14} />
          ) : on && detail.accounts.length > 0 ? (
            <Badge tone="neutral">
              {detail.accounts.length} login{detail.accounts.length > 1 ? 's' : ''}
            </Badge>
          ) : undefined
        }
      />

      {on && (
        <div
          style={{
            margin: '6px 0 var(--s3)',
            padding: 'var(--s3)',
            borderRadius: 'var(--r)',
            background: 'var(--bg-1)',
            border: '1px solid var(--border-0)',
          }}
        >
          <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', lineHeight: 1.55 }}>
            Add one entry for each {name} login you have. A second entry means your second {name}{' '}
            account, for example a personal plan and a work plan. When the first one runs out of usage,
            Relay carries the task over to the next.
          </div>

          {detail.accounts.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginTop: 'var(--s3)' }}>
              {detail.accounts.map((a) => (
                <div
                  key={a.label}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 'var(--s2)',
                    padding: '8px 10px',
                    borderRadius: 'var(--r)',
                    background: 'var(--bg-0)',
                    border: '1px solid var(--border-0)',
                  }}
                >
                  <Icon name="accounts" size={14} style={{ color: 'var(--tx-3)' }} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                      <span style={{ fontSize: 'var(--fz-md)', fontWeight: 600 }}>{a.label}</span>
                      {a.active && <Badge tone="accent">In use</Badge>}
                      {a.signedInKnown &&
                        (a.signedIn ? (
                          <span
                            style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 'var(--fz-xs)', color: 'var(--green)' }}
                          >
                            <StatusDot tone="green" /> signed in
                          </span>
                        ) : (
                          <span
                            style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}
                          >
                            <StatusDot tone="neutral" /> not signed in yet
                          </span>
                        ))}
                    </div>
                    <div
                      style={{
                        fontSize: 'var(--fz-xs)',
                        color: 'var(--tx-3)',
                        marginTop: 2,
                        fontFamily: 'var(--font-mono)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {a.configDir || "kept in Relay's own folder"}
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant="ghost"
                    icon="key"
                    loading={busyLabel === a.label}
                    disabled={!ready}
                    onClick={() => signIn(a.label)}
                    title={`Open the ${name} sign-in for this login`}
                  >
                    Sign in
                  </Button>
                </div>
              ))}
            </div>
          )}

          {adding ? (
            <AddAccount detail={detail} onDone={() => setAdding(false)} />
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--s2)', marginTop: 'var(--s3)' }}>
              <Button variant="default" icon="plus" disabled={!ready} onClick={() => setAdding(true)}>
                {detail.accounts.length === 0 ? `Add my ${name} login` : `Add another ${name} login`}
              </Button>
              <WaitingNote />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ── The step ─────────────────────────────────────────────────────────────────

export function StepProviders() {
  const { details, refresh } = useStore()
  const { ready, failed } = useDaemonReady()
  const autoRan = useRef(false)
  const [autoAdded, setAutoAdded] = useState<string[]>([])

  // Anything that can hold a login or run as a local CLI. Same filter the
  // Accounts screen uses, so the two screens never disagree about what exists.
  const usable = details.filter(
    (d) => d.canOauth || d.canApiKey || d.kind === 'cli' || d.kind === 'local' || d.accounts.length > 0,
  )
  const enabledCount = usable.filter((d) => d.enabled).length
  const accountCount = usable.reduce((n, d) => n + (d.enabled ? d.accounts.length : 0), 0)

  // A tool Relay can already see on this machine should not cost three clicks
  // before it is usable, so anything the probe finds installed gets turned on
  // automatically, once, the first time the list loads.
  //
  // This deliberately does NOT also register a first login slot. It used to:
  // addAccount(name, 'Account 1', '') with a blank configDir, which the daemon
  // resolves to a brand new, never-authenticated ~/.<provider>-Account 1
  // folder (AddUIAccount's default, correct for a deliberately isolated
  // SECOND login, wrong here). For someone who already has the CLI signed in
  // at its normal default location, which is exactly what makes it probe as
  // "available" in the first place, that silently orphaned the real,
  // already-working login behind a phantom empty one: sign-in looked like it
  // never took effect because it was writing to the wrong folder, and every
  // dispatch failed auth against the empty one. A provider with zero
  // registered accounts already runs fine against its own ambient default
  // login; only turning it on is safe to automate. Explicitly adding a login
  // slot stays a user action, on the Providers/Accounts screens, where the
  // isolated-folder tradeoff is explained.
  useEffect(() => {
    if (autoRan.current || !ready || details.length === 0) return
    const detected = usable.filter(
      (d) =>
        !d.enabled &&
        d.accounts.length === 0 &&
        (d.probeStatus === 'available' || d.probeStatus === 'no_key'),
    )
    if (detected.length === 0) return
    autoRan.current = true
    void (async () => {
      const enabled: string[] = []
      for (const d of detected) {
        const on = await api.saveProviderConfig({ name: d.name, enabled: true })
        if (on.ok) enabled.push(providerTitle(d.name))
      }
      await refresh()
      if (enabled.length > 0) setAutoAdded(enabled)
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, details.length])

  return (
    <>
      <StepHead
        eyebrow="Step 2 of 5"
        title="Your coding tools and logins"
        body="Turn on the tools you already have, then add every login you own for each one. This is what lets Relay keep working after one account hits its limit."
      />

      {autoAdded.length > 0 && (
        <Note icon="check">
          Found {autoAdded.join(', ')} already installed and turned{' '}
          {autoAdded.length > 1 ? 'them' : 'it'} on. If you're already signed in there, it will
          just work, add a login below only if you want a second, separate one.
        </Note>
      )}

      {usable.length === 0 ? (
        <Note icon={failed ? 'warning' : 'providers'}>
          {failed
            ? 'Relay could not start, so the list of tools is not available yet. You can skip this step and come back from the Providers screen once Relay is running.'
            : 'Loading the tools Relay can see on this computer.'}
        </Note>
      ) : (
        <>
          {usable.map((d) => (
            <ProviderBlock key={d.name} detail={d} />
          ))}

          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--s3)',
              marginTop: 'var(--s3)',
              fontSize: 'var(--fz-xs)',
              color: 'var(--tx-3)',
            }}
          >
            <span>
              {enabledCount} tool{enabledCount === 1 ? '' : 's'} on, {accountCount} login
              {accountCount === 1 ? '' : 's'} added
            </span>
            {!ready && <WaitingNote />}
          </div>
        </>
      )}
    </>
  )
}
