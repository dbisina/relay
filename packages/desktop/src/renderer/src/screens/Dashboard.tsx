// Dashboard.tsx — Home. A chat-first cockpit, not a stacked dashboard: greet,
// show recent sessions you can click to reload their provider/task config, and
// a composer with chips for arranging providers/profile/threshold above it,
// quick actions below it. When a task is actually running, the scroll area's
// top becomes that session's live status + event stream instead of a welcome.

import React, { useEffect, useMemo, useRef, useState } from 'react'
import type { Route } from '../components/Sidebar'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { providerTitle, compact, relTime } from '../lib/format'
import { sessionProject, sessionIntent, contextWeight, byRecency } from '../lib/session'
import { Button, StatusDot, Chip, Spinner, ProviderGlyph } from '../components/ui'
import { ChainBuilder } from '../components/ChainBuilder'
import { Popover } from '../components/Popover'
import { SessionDetail } from '../components/SessionDetail'
import { Icon } from '../components/Icon'
import { useLocale } from '../lib/i18n'
import type { EventLine, DetectedAgent } from '../lib/types'

function Spark({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="var(--accent)">
      <path d="M12 1.5c.7 4.2 2.3 7.1 5.5 8.5-3.2 1.4-4.8 4.3-5.5 8.5-.7-4.2-2.3-7.1-5.5-8.5 3.2-1.4 4.8-4.3 5.5-8.5z" />
    </svg>
  )
}

// ── Sessions list (idle state) ───────────────────────────────────────────────
// Source is /api/detect, not /api/history: detect carries the rich, adoptable
// session (real id, model, message/file counts, skills) and is exactly what the
// Scan & adopt screen lists in full, so "See all" lands on the same data.
const HOME_SESSION_LIMIT = 2

function SessionRow({ agent, onOpen }: { agent: DetectedAgent; onOpen: () => void }) {
  const [hover, setHover] = useState(false)
  const intent = sessionIntent(agent)
  const w = contextWeight(agent)
  return (
    <button
      onClick={onOpen}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 'var(--s3)',
        width: '100%',
        padding: '12px 13px',
        borderRadius: 'var(--r)',
        background: hover ? 'var(--bg-2)' : 'var(--bg-1)',
        border: '1px solid var(--border-0)',
        textAlign: 'left',
        transition: 'background 0.12s var(--ease)',
      }}
    >
      <ProviderGlyph name={agent.provider} size={30} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontSize: 'var(--fz-md)', fontWeight: 600 }}>{sessionProject(agent)}</span>
          {agent.running && (
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: 'var(--fz-xs)', color: 'var(--green)' }}>
              <StatusDot tone="green" pulse /> running
            </span>
          )}
        </div>
        <div
          style={{
            fontSize: 'var(--fz-sm)',
            color: 'var(--tx-2)',
            marginTop: 3,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {intent || `${providerTitle(agent.provider)} session, no recorded prompt`}
        </div>
      </div>
      <div style={{ flexShrink: 0, textAlign: 'right' }}>
        <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)' }}>{relTime(agent.lastActive)}</div>
        <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', fontFamily: 'var(--font-mono)', marginTop: 3 }}>
          {w.messages > 0 ? `${compact(w.messages)} msg` : ''}
          {w.messages > 0 && w.files > 0 ? ' · ' : ''}
          {w.files > 0 ? `${w.files} files` : ''}
        </div>
      </div>
      <Icon name="chevronRight" size={14} style={{ color: 'var(--tx-3)', flexShrink: 0 }} />
    </button>
  )
}

function SessionsList({ onOpen, go }: { onOpen: (a: DetectedAgent) => void; go: (r: Route) => void }) {
  const { agents, agentsScanned, scanningAgents, scanAgents } = useStore()
  const { t } = useLocale()

  // Scan once per app session; the store caches so revisiting Home is free.
  useEffect(() => {
    if (!agentsScanned && !scanningAgents) scanAgents(24)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const sorted = useMemo(() => [...agents].sort(byRecency), [agents])
  const shown = sorted.slice(0, HOME_SESSION_LIMIT)
  const hiddenCount = sorted.length - shown.length

  const header = (
    <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--tx-3)', marginBottom: 'var(--s2)' }}>
      {t('home.sessions')}
    </div>
  )

  if (!agentsScanned && scanningAgents) {
    return (
      <div style={{ marginTop: 'var(--s5)' }}>
        {header}
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '10px 2px', color: 'var(--tx-3)', fontSize: 'var(--fz-sm)' }}>
          <Spinner size={14} /> Looking for recent agent sessions
        </div>
      </div>
    )
  }

  if (sorted.length === 0) {
    return (
      <div style={{ marginTop: 'var(--s5)' }}>
        {header}
        <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)', padding: '10px 2px' }}>{t('home.noSessions')}</div>
      </div>
    )
  }

  return (
    <div style={{ marginTop: 'var(--s5)' }}>
      {header}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        {shown.map((a) => (
          <SessionRow key={a.id} agent={a} onOpen={() => onOpen(a)} />
        ))}
      </div>
      {hiddenCount > 0 && (
        <button
          onClick={() => go('detect')}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            marginTop: 8,
            padding: '4px 2px',
            fontSize: 'var(--fz-sm)',
            color: 'var(--tx-2)',
          }}
        >
          See all {sorted.length} sessions
          <Icon name="chevronRight" size={13} />
        </button>
      )}
    </div>
  )
}

// ── Active session, compact (replaces the old full-page stat block) ─────────
function Sparkline({ data }: { data: number[] }) {
  if (!data || data.length < 2) return null
  const w = 70,
    h = 18
  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const span = max - min || 1
  const pts = data.map((v, i) => `${(i / (data.length - 1)) * w},${h - ((v - min) / span) * h}`).join(' ')
  return (
    <svg width={w} height={h}>
      <polyline points={pts} fill="none" stroke="var(--accent)" strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  )
}

function ActiveSessionStrip({ go }: { go: (r: Route) => void }) {
  const toast = useToast()
  const { session, refresh, events } = useStore()
  const [handing, setHanding] = useState(false)
  const tailRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    tailRef.current?.scrollTo({ top: tailRef.current.scrollHeight })
  }, [events])

  if (!session || !session.sessionId) return null

  const handoff = async () => {
    setHanding(true)
    const r = await api.handoff()
    setHanding(false)
    if (r.ok) {
      toast('Handoff triggered. Signing a continuation contract for the next agent.', 'ok')
      refresh()
    } else toast(r.error || 'Handoff failed', 'error')
  }

  const lines = events.slice(-80)

  return (
    <div style={{ marginBottom: 'var(--s4)' }}>
      <div
        style={{
          border: '1px solid var(--border-1)',
          borderRadius: 'var(--r-lg)',
          background: 'var(--bg-1)',
          padding: 'var(--s4)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 'var(--s4)' }}>
          <div style={{ minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <StatusDot tone="green" pulse />
              <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)', letterSpacing: '0.04em', textTransform: 'uppercase' }}>
                {session.fsmState || 'working'} on {providerTitle(session.activeProvider)}
              </span>
            </div>
            <div style={{ fontSize: 'var(--fz-lg)', fontWeight: 600, marginTop: 6 }} className="selectable">
              {session.taskGoal || 'Untitled task'}
            </div>
          </div>
          <div style={{ display: 'flex', gap: 'var(--s2)', flexShrink: 0 }}>
            <Button variant="ghost" icon="pause" onClick={() => api.pause()}>
              Pause
            </Button>
            <Button variant="primary" icon="handoff" loading={handing} onClick={handoff}>
              Hand off now
            </Button>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--s5)', marginTop: 'var(--s3)', paddingTop: 'var(--s3)', borderTop: '1px solid var(--border-0)' }}>
          <MiniStat label="Tokens" value={compact(session.tokensUsed)} />
          <MiniStat label="Handoffs" value={String(session.handoffsDone)} />
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>Fitness</span>
            <span style={{ fontSize: 'var(--fz-md)', fontWeight: 600, fontFamily: 'var(--font-mono)' }}>{session.hfsScore.toFixed(2)}</span>
            <Sparkline data={session.hfsHistory} />
          </div>
          <div style={{ flex: 1 }} />
          <button onClick={() => go('history')} style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)' }}>
            View timeline →
          </button>
        </div>
      </div>

      <div
        ref={tailRef}
        className="selectable"
        style={{
          marginTop: 'var(--s2)',
          height: 160,
          overflow: 'auto',
          padding: 'var(--s3)',
          borderRadius: 'var(--r-lg)',
          background: 'var(--bg-0)',
          border: '1px solid var(--border-0)',
          fontFamily: 'var(--font-mono)',
          fontSize: 'var(--fz-sm)',
          lineHeight: 1.7,
        }}
      >
        {lines.length === 0 ? (
          <div style={{ color: 'var(--tx-3)' }}>Waiting for agent output…</div>
        ) : (
          lines.map((e: EventLine, i) => (
            <div key={e.id ?? i} style={{ display: 'flex', gap: 10 }}>
              <span style={{ color: 'var(--tx-3)', flexShrink: 0 }}>{e.ts || ''}</span>
              <span style={{ color: tagColor(e.tag), flexShrink: 0, width: 64 }}>{e.tag || 'text'}</span>
              <span style={{ color: 'var(--tx-1)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{e.msg ?? ''}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
function MiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
      <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>{label}</span>
      <span style={{ fontSize: 'var(--fz-md)', fontWeight: 600, fontFamily: 'var(--font-mono)' }}>{value}</span>
    </div>
  )
}
function tagColor(tag?: string): string {
  switch ((tag || '').toLowerCase()) {
    case 'handoff':
      return 'var(--accent)'
    case 'quota':
      return 'var(--yellow)'
    case 'result':
      return 'var(--green)'
    case 'tooluse':
    case 'tool':
      return 'var(--blue)'
    case 'system':
      return 'var(--tx-3)'
    default:
      return 'var(--tx-2)'
  }
}

// ── Composer: chip toolbar above, textarea, quick actions + summary below ───
interface RunConfig {
  providers: string[]
  threshold: number
  maxHandoffs: number
}
type ChipKey = 'providers' | 'profile' | 'threshold' | 'maxHandoffs' | null

function Composer({
  go,
  prefill,
}: {
  go: (r: Route) => void
  prefill: { task: string; providers: string[] } | null
}) {
  const toast = useToast()
  const { t } = useLocale()
  const { details, profiles, refresh, conn } = useStore()
  const [task, setTask] = useState('')
  const [cfg, setCfg] = useState<RunConfig>(() => ({
    providers: [],
    threshold: Number(localStorage.getItem('relay.threshold')) || 0.85,
    maxHandoffs: 0,
  }))
  const [openChip, setOpenChip] = useState<ChipKey>(null)
  const [busy, setBusy] = useState(false)
  const taRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (!prefill) return
    setTask(prefill.task)
    setCfg((c) => ({ ...c, providers: prefill.providers }))
    taRef.current?.focus()
  }, [prefill])

  const availableProviders = details.filter((d) => d.enabled || d.probeStatus === 'available').map((d) => d.name)
  const effectiveChain = cfg.providers.length > 0 ? cfg.providers : availableProviders

  const start = async () => {
    if (!task.trim()) return
    setBusy(true)
    const r = await api.run(task.trim(), {
      threshold: cfg.threshold,
      maxHandoffs: cfg.maxHandoffs,
      providers: cfg.providers,
    })
    setBusy(false)
    if (r.ok) {
      toast('Task started. Relay will auto-hand off across your agents.', 'ok')
      setTask('')
      refresh()
    } else toast(r.error || 'Could not start task', 'error')
  }

  const applyProfile = (name: string) => {
    const p = profiles.find((x) => x.name === name)
    if (!p) return
    setCfg((c) => ({ ...c, providers: p.chain }))
    setOpenChip(null)
    toast(`Applied "${p.name}" chain to this run`, 'info')
  }

  return (
    <div
      style={{
        borderTop: '1px solid var(--border-0)',
        background: 'linear-gradient(var(--bg-0), var(--bg-0))',
        padding: 'var(--s4) var(--s6) var(--s5)',
      }}
    >
      <div style={{ maxWidth: 760, margin: '0 auto' }}>
        {/* Chip toolbar — arrange providers / profile / run limits before sending */}
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 'var(--s2)', position: 'relative' }}>
          <div style={{ position: 'relative' }}>
            <Chip icon="providers" active={cfg.providers.length > 0} onClick={() => setOpenChip(openChip === 'providers' ? null : 'providers')}>
              {cfg.providers.length > 0 ? `${cfg.providers.length} provider${cfg.providers.length > 1 ? 's' : ''}` : t('home.autoRotation')}
            </Chip>
            <Popover open={openChip === 'providers'} onClose={() => setOpenChip(null)} width={280}>
              <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, color: 'var(--tx-2)', marginBottom: 8 }}>
                Provider order for this run
              </div>
              <ChainBuilder
                chain={cfg.providers}
                available={availableProviders}
                onChange={(c) => setCfg((x) => ({ ...x, providers: c }))}
                emptyHint="Using Relay's default rotation. Add providers to override just this run."
              />
              {cfg.providers.length > 0 && (
                <button onClick={() => setCfg((x) => ({ ...x, providers: [] }))} style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 8 }}>
                  Reset to auto
                </button>
              )}
            </Popover>
          </div>

          <div style={{ position: 'relative' }}>
            <Chip icon="workflow" onClick={() => setOpenChip(openChip === 'profile' ? null : 'profile')}>
              Profile
            </Chip>
            <Popover open={openChip === 'profile'} onClose={() => setOpenChip(null)} width={240}>
              {profiles.length === 0 ? (
                <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>
                  No profiles yet.{' '}
                  <button onClick={() => go('workflow')} style={{ color: 'var(--accent)' }}>
                    Create one
                  </button>
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  {profiles.map((p) => (
                    <button
                      key={p.name}
                      onClick={() => applyProfile(p.name)}
                      style={{ textAlign: 'left', padding: '7px 9px', borderRadius: 'var(--r)', fontSize: 'var(--fz-sm)' }}
                    >
                      <div style={{ fontWeight: 600 }}>{p.name}</div>
                      <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>{p.chain.map(providerTitle).join(' → ') || 'no chain'}</div>
                    </button>
                  ))}
                </div>
              )}
            </Popover>
          </div>

          <div style={{ position: 'relative' }}>
            <Chip active={cfg.threshold !== 0.85} onClick={() => setOpenChip(openChip === 'threshold' ? null : 'threshold')}>
              {Math.round(cfg.threshold * 100)}% threshold
            </Chip>
            <Popover open={openChip === 'threshold'} onClose={() => setOpenChip(null)} width={220}>
              <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, color: 'var(--tx-2)', marginBottom: 8 }}>
                Hand off at this fraction used
              </div>
              <input
                type="range"
                min={0.5}
                max={0.98}
                step={0.01}
                value={cfg.threshold}
                onChange={(e) => setCfg((c) => ({ ...c, threshold: Number(e.target.value) }))}
                style={{ width: '100%' }}
              />
              <div style={{ fontSize: 'var(--fz-sm)', textAlign: 'center', fontFamily: 'var(--font-mono)', marginTop: 4 }}>
                {Math.round(cfg.threshold * 100)}%
              </div>
            </Popover>
          </div>

          <div style={{ position: 'relative' }}>
            <Chip active={cfg.maxHandoffs > 0} onClick={() => setOpenChip(openChip === 'maxHandoffs' ? null : 'maxHandoffs')}>
              {cfg.maxHandoffs > 0 ? `${cfg.maxHandoffs} max handoffs` : 'Unlimited handoffs'}
            </Chip>
            <Popover open={openChip === 'maxHandoffs'} onClose={() => setOpenChip(null)} width={220}>
              <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, color: 'var(--tx-2)', marginBottom: 8 }}>
                Stop after this many handoffs (0 = unlimited)
              </div>
              <input
                type="number"
                min={0}
                value={cfg.maxHandoffs}
                onChange={(e) => setCfg((c) => ({ ...c, maxHandoffs: Math.max(0, Number(e.target.value) || 0) }))}
                style={{
                  width: '100%',
                  height: 32,
                  padding: '0 10px',
                  borderRadius: 'var(--r)',
                  background: 'var(--bg-0)',
                  border: '1px solid var(--border-1)',
                  color: 'var(--tx-0)',
                  fontFamily: 'var(--font-mono)',
                }}
              />
            </Popover>
          </div>
        </div>

        {/* Composer pill */}
        <div
          style={{
            display: 'flex',
            alignItems: 'flex-end',
            gap: 'var(--s2)',
            padding: '10px 10px 10px 16px',
            borderRadius: 22,
            background: 'var(--bg-1)',
            border: '1px solid var(--border-1)',
          }}
        >
          <textarea
            ref={taRef}
            value={task}
            onChange={(e) => setTask(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                start()
              }
            }}
            placeholder={t('home.composerPlaceholder')}
            rows={1}
            className="selectable"
            style={{
              flex: 1,
              resize: 'none',
              background: 'transparent',
              border: 'none',
              outline: 'none',
              color: 'var(--tx-0)',
              fontFamily: 'var(--font-ui)',
              fontSize: 'var(--fz-md)',
              lineHeight: 1.5,
              maxHeight: 140,
              padding: '6px 0',
            }}
          />
          <Button variant="primary" loading={busy} onClick={start} style={{ borderRadius: 16, width: 36, height: 36, padding: 0 }}>
            <Icon name="chevronRight" size={16} style={{ transform: 'rotate(-90deg)' }} />
          </Button>
        </div>

        {/* Under the bar: quick actions left, live summary right */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 8, padding: '0 4px' }}>
          <div style={{ display: 'flex', gap: 'var(--s2)' }}>
            <button onClick={() => go('detect')} style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>
              <Icon name="detect" size={12} /> {t('home.scanAgents')}
            </button>
            <button onClick={() => go('workflow')} style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>
              <Icon name="workflow" size={12} /> {t('home.newProfile')}
            </button>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }} className="mono">
            <StatusDot tone={conn === 'up' ? 'green' : 'red'} />
            {effectiveChain.length > 0 ? effectiveChain.slice(0, 3).map(providerTitle).join(' → ') : t('home.noProvidersEnabled')}
            {effectiveChain.length > 3 ? ` +${effectiveChain.length - 3}` : ''}
          </div>
        </div>
      </div>
    </div>
  )
}

export function Dashboard({ go }: { go: (r: Route) => void }) {
  const { session, conn } = useStore()
  const { t } = useLocale()
  const [username, setUsername] = useState('')
  const [openSession, setOpenSession] = useState<DetectedAgent | null>(null)

  useEffect(() => {
    window.relay.whoami?.().then((u) => setUsername(u))
  }, [])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, overflow: 'auto', padding: 'var(--s6) var(--s6) 0' }}>
        <div style={{ maxWidth: 760, margin: '0 auto' }}>
          {session?.sessionId ? (
            <ActiveSessionStrip go={go} />
          ) : (
            <>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <Spark />
                <h1 style={{ fontSize: 'var(--fz-2xl)', fontWeight: 700, letterSpacing: '-0.01em' }}>
                  {t('home.welcome')}{username ? `, ${username}` : ''}
                </h1>
              </div>
              {conn !== 'up' ? (
                <div style={{ padding: 'var(--s6) 0', display: 'flex', justifyContent: 'center' }}>
                  <Spinner size={20} />
                </div>
              ) : (
                <SessionsList onOpen={setOpenSession} go={go} />
              )}
            </>
          )}
        </div>
      </div>
      <Composer go={go} prefill={null} />
      <SessionDetail agent={openSession} onClose={() => setOpenSession(null)} />
    </div>
  )
}
