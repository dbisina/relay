// SessionDetail.tsx — the one detail surface for a detected session, opened
// from both Home and Scan & adopt so the app has exactly one of these.
//
// Design premise: Relay's credibility is that a handoff moves REAL state, not
// magic. So the body is a manifest of what would actually cross the wire, and
// every number in it comes from the daemon. Where the daemon gives nothing
// (plan[] and tasksRemaining[] are usually empty in practice) the row says so
// plainly rather than being hidden or filled with an invented placeholder.
//
// The handoff control is pinned in the footer, never scrolls away.

import React, { useMemo, useState } from 'react'
import { Drawer } from './Drawer'
import { Button, Badge, StatusDot, ProviderGlyph, Spinner } from './ui'
import { Icon } from './Icon'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { providerTitle, compact, relTime } from '../lib/format'
import { sessionProject, sessionIntent, contextWeight } from '../lib/session'
import type { DetectedAgent, ProviderDetail } from '../lib/types'

// ── small building blocks ────────────────────────────────────────────────────
function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        fontSize: 'var(--fz-xs)',
        fontWeight: 600,
        letterSpacing: '0.06em',
        textTransform: 'uppercase',
        color: 'var(--tx-2)',
        marginBottom: 'var(--s2)',
      }}
    >
      {children}
    </div>
  )
}

/** A prompt shown verbatim, clamped until asked to expand. */
function PromptWell({ label, text }: { label: string; text: string }) {
  const [open, setOpen] = useState(false)
  const long = text.length > 260
  return (
    <div style={{ marginBottom: 'var(--s3)' }}>
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginBottom: 5 }}>
        {label}
      </div>
      <div
        className="selectable"
        style={{
          background: 'var(--bg-0)',
          border: '1px solid var(--border-0)',
          borderRadius: 'var(--r)',
          padding: '10px 12px',
          fontSize: 'var(--fz-sm)',
          color: 'var(--tx-1)',
          lineHeight: 1.55,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          maxHeight: open ? undefined : 92,
          overflow: 'hidden',
        }}
      >
        {text}
      </div>
      {long && (
        <button onClick={() => setOpen((v) => !v)} style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)', marginTop: 5 }}>
          {open ? 'show less' : 'show all'}
        </button>
      )}
    </div>
  )
}

/** One row of the carry manifest. Always rendered, including when the count is 0. */
function ManifestRow({ item, count, note }: { item: string; count: string; note?: string }) {
  const empty = count === '0' || count === 'none'
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '1fr auto',
        gap: 'var(--s3)',
        alignItems: 'baseline',
        padding: '7px 0',
        borderTop: '1px solid var(--border-0)',
        fontSize: 'var(--fz-sm)',
      }}
    >
      <div style={{ minWidth: 0 }}>
        <span style={{ color: empty ? 'var(--tx-3)' : 'var(--tx-1)' }}>{item}</span>
        {note && (
          <div
            style={{
              fontSize: 'var(--fz-xs)',
              color: 'var(--tx-3)',
              marginTop: 2,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {note}
          </div>
        )}
      </div>
      <span
        style={{
          fontFamily: 'var(--font-mono)',
          fontVariantNumeric: 'tabular-nums',
          color: empty ? 'var(--tx-3)' : 'var(--tx-0)',
        }}
      >
        {count}
      </span>
    </div>
  )
}

// ── account picker ───────────────────────────────────────────────────────────
// Accounts are how one provider holds several logins: each carries its own
// config dir (the daemon maps claude -> CLAUDE_CONFIG_DIR, codex -> CODEX_HOME),
// so switching accounts switches credentials. They are user-registered, not
// auto-discovered, which is why a fresh install has none. Rather than dead-end
// with "no accounts", let one be added right here, then picked.
function AccountPicker({
  provider,
  selected,
  onSelect,
}: {
  provider: ProviderDetail
  selected: string
  onSelect: (label: string) => void
}) {
  const toast = useToast()
  const { refresh } = useStore()
  const [adding, setAdding] = useState(false)
  const [label, setLabel] = useState('')
  const [dir, setDir] = useState('')
  const [busy, setBusy] = useState(false)

  const accounts = provider.accounts ?? []
  const active = accounts.find((a) => a.active)?.label || ''
  const effective = selected || active

  const add = async () => {
    const l = label.trim()
    if (!l) return
    setBusy(true)
    const r = await api.addAccount(provider.name, l, dir.trim())
    setBusy(false)
    if (!r.ok) {
      toast(r.error || 'Could not add account', 'error')
      return
    }
    toast(`Added "${l}" to ${providerTitle(provider.name)}`, 'ok')
    onSelect(l)
    setLabel('')
    setDir('')
    setAdding(false)
    refresh()
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
        <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>account</span>
        {accounts.map((a) => (
          <button
            key={a.label}
            onClick={() => onSelect(a.label)}
            style={{
              padding: '3px 9px',
              borderRadius: 'var(--r-pill)',
              fontSize: 'var(--fz-xs)',
              border: `1px solid ${effective === a.label ? 'var(--accent-line)' : 'var(--border-1)'}`,
              background: effective === a.label ? 'var(--accent-weak)' : 'transparent',
              color: effective === a.label ? 'var(--accent)' : 'var(--tx-2)',
            }}
          >
            {a.label}
            {a.active ? ' · active' : ''}
          </button>
        ))}
        {accounts.length === 0 && (
          <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>
            none registered, default login will be used
          </span>
        )}
        <button
          onClick={() => setAdding((v) => !v)}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 4,
            padding: '3px 9px',
            borderRadius: 'var(--r-pill)',
            fontSize: 'var(--fz-xs)',
            border: '1px dashed var(--border-2)',
            color: 'var(--tx-2)',
          }}
        >
          <Icon name="plus" size={10} /> add
        </button>
      </div>

      {adding && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 6,
            padding: 'var(--s2)',
            borderRadius: 'var(--r)',
            background: 'var(--bg-0)',
            border: '1px solid var(--border-1)',
          }}
        >
          <input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="label, e.g. work"
            autoFocus
            onKeyDown={(e) => e.key === 'Enter' && add()}
            style={inputStyle}
          />
          <div style={{ display: 'flex', gap: 6 }}>
            <input
              value={dir}
              onChange={(e) => setDir(e.target.value)}
              placeholder="config folder (optional, keeps logins separate)"
              style={{ ...inputStyle, fontFamily: 'var(--font-mono)', flex: 1 }}
            />
            <Button
              size="sm"
              variant="default"
              icon="folder"
              onClick={async () => {
                const p = await window.relay.pickFolder()
                if (p) setDir(p)
              }}
            />
          </div>
          <div style={{ display: 'flex', gap: 6 }}>
            <Button size="sm" variant="primary" loading={busy} onClick={add}>
              Add account
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setAdding(false)}>
              Cancel
            </Button>
          </div>
          <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', lineHeight: 1.5 }}>
            Adding registers the account. Sign in to it from the Accounts screen, Relay launches{' '}
            {providerTitle(provider.name)}'s own login and never sees your password.
          </div>
        </div>
      )}
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  height: 30,
  padding: '0 9px',
  borderRadius: 'var(--r)',
  background: 'var(--bg-1)',
  border: '1px solid var(--border-1)',
  color: 'var(--tx-0)',
  fontSize: 'var(--fz-sm)',
  outline: 'none',
}

// ── the handoff control (pinned footer) ──────────────────────────────────────
function HandoffBar({
  agent,
  targets,
  onDone,
}: {
  agent: DetectedAgent
  targets: ProviderDetail[]
  onDone: () => void
}) {
  const toast = useToast()
  const { refresh, scanAgents } = useStore()
  const [target, setTarget] = useState<string>(() => targets.find((t) => t.name !== agent.provider)?.name || '')
  const [account, setAccount] = useState<string>('')
  const [start, setStart] = useState(true)
  const [busy, setBusy] = useState(false)

  const chosen = targets.find((t) => t.name === target)

  const run = async () => {
    if (!target) return
    setBusy(true)
    const r = await api.handOffSession(agent.id, target, { account: account || undefined, start })
    setBusy(false)
    if (r.ok) {
      toast(`Handed off to ${providerTitle(target)}${start ? ', continuing now' : ''}.`, 'ok')
      refresh()
      scanAgents(24)
      onDone()
    } else {
      toast(r.error || 'Handoff failed', 'error')
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s3)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--s2)' }}>
        <Icon name="handoff" size={15} style={{ color: 'var(--accent)', flexShrink: 0 }} />
        <span style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)' }}>Continue this session on</span>
      </div>

      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {targets.map((p) => {
          const on = target === p.name
          const isSource = p.name === agent.provider
          return (
            <button
              key={p.name}
              onClick={() => {
                setTarget(p.name)
                setAccount('')
              }}
              title={isSource ? 'The provider this session is already on' : undefined}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                padding: '5px 10px',
                borderRadius: 'var(--r-pill)',
                fontSize: 'var(--fz-sm)',
                fontWeight: 500,
                border: `1px solid ${on ? 'var(--accent-line)' : 'var(--border-1)'}`,
                background: on ? 'var(--accent-weak)' : 'transparent',
                color: on ? 'var(--accent)' : 'var(--tx-1)',
              }}
            >
              <ProviderGlyph name={p.name} size={18} />
              {providerTitle(p.name)}
              {isSource && <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>same</span>}
            </button>
          )
        })}
      </div>

      {chosen && <AccountPicker provider={chosen} selected={account} onSelect={setAccount} />}

      <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', cursor: 'pointer' }}>
        <input type="checkbox" checked={start} onChange={(e) => setStart(e.target.checked)} />
        Start running it immediately
      </label>

      <Button variant="primary" icon="handoff" loading={busy} disabled={!target} onClick={run}>
        {target ? `Hand off to ${providerTitle(target)}` : 'Pick a provider'}
      </Button>
    </div>
  )
}

// ── the drawer ───────────────────────────────────────────────────────────────
export function SessionDetail({ agent, onClose }: { agent: DetectedAgent | null; onClose: () => void }) {
  const { details } = useStore()
  const [filesOpen, setFilesOpen] = useState(false)

  const targets = useMemo(
    () => details.filter((d) => d.enabled && d.probeStatus === 'available'),
    [details],
  )

  if (!agent) return null
  const s = agent.session
  const w = contextWeight(agent)
  const intent = sessionIntent(agent)
  const files = s?.filesTouched ?? []

  const header = (
    <>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--s3)' }}>
        <ProviderGlyph name={agent.provider} size={30} />
        <div style={{ minWidth: 0 }}>
          <div style={{ fontSize: 'var(--fz-lg)', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {sessionProject(agent)}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 3 }}>
            <span style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)' }}>
              {agent.displayName || providerTitle(agent.provider)}
            </span>
            {agent.running ? (
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: 'var(--fz-xs)', color: 'var(--green)' }}>
                <StatusDot tone="green" pulse /> running{agent.pid ? ` · pid ${agent.pid}` : ''}
              </span>
            ) : (
              <Badge tone="neutral">on disk</Badge>
            )}
            {agent.surface && <Badge tone="neutral">{agent.surface}</Badge>}
          </div>
        </div>
      </div>
      <div
        className="selectable"
        style={{
          marginTop: 'var(--s3)',
          fontFamily: 'var(--font-mono)',
          fontSize: 'var(--fz-xs)',
          color: 'var(--tx-3)',
          wordBreak: 'break-all',
          lineHeight: 1.6,
        }}
      >
        {agent.id}
        <br />
        {agent.workDir}
      </div>
      <div style={{ marginTop: 6, fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>
        last active {relTime(agent.lastActive)}
        {agent.detectedVia?.length > 0 ? ` · detected via ${agent.detectedVia.join(', ')}` : ''}
      </div>
    </>
  )

  return (
    <Drawer
      open={!!agent}
      onClose={onClose}
      title={header}
      footer={
        targets.length > 0 ? (
          <HandoffBar agent={agent} targets={targets} onDone={onClose} />
        ) : (
          <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)' }}>
            No provider is both enabled and available right now, so this session cannot be handed off yet.
          </div>
        )
      }
    >
      {!s ? (
        <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>
          Relay detected this agent but could not read a session transcript for it, so there is nothing to
          carry across yet.
        </div>
      ) : (
        <>
          <SectionLabel>Intent</SectionLabel>
          {intent ? (
            <>
              {s.lastPrompt && <PromptWell label="latest instruction" text={s.lastPrompt} />}
              {s.initialPrompt && s.initialPrompt !== s.lastPrompt && (
                <PromptWell label="first instruction" text={s.initialPrompt} />
              )}
            </>
          ) : (
            <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)', marginBottom: 'var(--s3)' }}>
              This transcript records no readable user instruction, only harness setup.
            </div>
          )}

          <div style={{ height: 'var(--s4)' }} />
          <SectionLabel>Carried in the contract</SectionLabel>
          <div style={{ borderBottom: '1px solid var(--border-0)' }}>
            <ManifestRow item="model" count={s.model || 'unknown'} />
            <ManifestRow item="messages" count={compact(w.messages)} />
            <ManifestRow item="tokens" count={compact(w.tokens)} note={`${compact(s.tokensIn)} in · ${compact(s.tokensOut)} out`} />
            <ManifestRow item="files touched" count={String(w.files)} note={w.files > 0 ? 'listed below' : undefined} />
            <ManifestRow item="skills" count={String(w.skills)} note={s.skills?.join(', ') || undefined} />
            <ManifestRow item="mcp servers" count={String(w.mcps)} note={s.mcps?.join(', ') || undefined} />
            <ManifestRow
              item="plan steps"
              count={String(s.plan?.length ?? 0)}
              note={(s.plan?.length ?? 0) === 0 ? 'not exposed by this transcript' : undefined}
            />
            <ManifestRow
              item="tasks remaining"
              count={String(s.tasksRemaining?.length ?? 0)}
              note={(s.tasksRemaining?.length ?? 0) === 0 ? 'not exposed by this transcript' : undefined}
            />
          </div>

          {files.length > 0 && (
            <>
              <div style={{ height: 'var(--s4)' }} />
              <button
                onClick={() => setFilesOpen((v) => !v)}
                style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--tx-2)' }}
              >
                <Icon name="chevronDown" size={13} style={{ transform: filesOpen ? 'none' : 'rotate(-90deg)', transition: 'transform 0.14s' }} />
                Files touched ({files.length})
              </button>
              {filesOpen && (
                <div
                  className="selectable"
                  style={{
                    marginTop: 'var(--s2)',
                    maxHeight: 220,
                    overflow: 'auto',
                    background: 'var(--bg-0)',
                    border: '1px solid var(--border-0)',
                    borderRadius: 'var(--r)',
                    padding: '8px 10px',
                    fontFamily: 'var(--font-mono)',
                    fontSize: 'var(--fz-xs)',
                    color: 'var(--tx-2)',
                    lineHeight: 1.75,
                  }}
                >
                  {files.map((f, i) => (
                    <div key={i} style={{ wordBreak: 'break-all' }}>
                      {f}
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </>
      )}
    </Drawer>
  )
}
