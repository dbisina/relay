// Detect.tsx — find AI coding agents already running on this machine (or with a
// recent on-disk session) and adopt one into Relay so it can be continued
// elsewhere. This is the "I already started work, don't make me restart it" path.

import React, { useEffect, useState } from 'react'
import { Page } from '../components/Page'
import { useStore } from '../lib/store'
import { providerTitle, relTime, compact } from '../lib/format'
import { sessionProject, sessionIntent, byRecency } from '../lib/session'
import { Button, Badge, StatusDot, ProviderGlyph, EmptyState, Spinner } from '../components/ui'
import { SessionDetail } from '../components/SessionDetail'
import { Icon } from '../components/Icon'
import type { DetectedAgent } from '../lib/types'

function AgentCard({ agent, onAdopt }: { agent: DetectedAgent; onAdopt: (a: DetectedAgent) => void }) {
  const s = agent.session
  const intent = sessionIntent(agent)
  return (
    <div
      style={{
        border: '1px solid var(--border-0)',
        borderRadius: 'var(--r-lg)',
        background: 'var(--bg-1)',
        padding: 'var(--s4)',
        marginBottom: 'var(--s3)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 'var(--s3)' }}>
        <ProviderGlyph name={agent.provider} size={34} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            {/* Lead with the project, not the provider: every Claude session
                would otherwise be titled "Claude Code" and be unidentifiable. */}
            <span style={{ fontSize: 'var(--fz-lg)', fontWeight: 600 }}>{sessionProject(agent)}</span>
            <span style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)' }}>
              {agent.displayName || providerTitle(agent.provider)}
            </span>
            {agent.running ? (
              <Badge tone="green">
                <StatusDot tone="green" /> running{agent.pid ? ` · pid ${agent.pid}` : ''}
              </Badge>
            ) : (
              <Badge tone="neutral">recent session</Badge>
            )}
            {agent.surface && <Badge tone="neutral">{agent.surface}</Badge>}
            {agent.account && <Badge tone="blue">{agent.account}</Badge>}
          </div>
          {agent.workDir && (
            <div
              className="selectable"
              style={{
                fontFamily: 'var(--font-mono)',
                fontSize: 'var(--fz-xs)',
                color: 'var(--tx-3)',
                marginTop: 4,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {agent.workDir}
            </div>
          )}
        </div>
        <Button variant="default" icon="chevronRight" onClick={() => onAdopt(agent)}>
          Open
        </Button>
      </div>

      {s && (intent || s.messageCount > 0 || s.filesTouched?.length > 0) && (
        <div style={{ marginTop: 'var(--s3)', paddingTop: 'var(--s3)', borderTop: '1px solid var(--border-0)' }}>
          {intent && (
            <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-1)', lineHeight: 1.5 }} className="selectable">
              {intent}
            </div>
          )}
          <div style={{ display: 'flex', gap: 'var(--s4)', marginTop: 'var(--s2)', flexWrap: 'wrap' }}>
            {s.model && <Meta label="model" value={s.model} />}
            {s.messageCount > 0 && <Meta label="messages" value={String(s.messageCount)} />}
            {(s.tokensIn > 0 || s.tokensOut > 0) && (
              <Meta label="tokens" value={`${compact(s.tokensIn)} in · ${compact(s.tokensOut)} out`} />
            )}
            {s.filesTouched?.length > 0 && <Meta label="files" value={String(s.filesTouched.length)} />}
            {s.tasksRemaining?.length > 0 && <Meta label="tasks left" value={String(s.tasksRemaining.length)} />}
            {/* lastActive (epoch ms on the agent) is the real timestamp.
                session.lastActivity is the last message TEXT, not a time. */}
            {agent.lastActive > 0 && <Meta label="active" value={relTime(agent.lastActive)} />}
          </div>
          {s.skills?.length > 0 && (
            <div style={{ display: 'flex', gap: 6, marginTop: 'var(--s2)', flexWrap: 'wrap' }}>
              {s.skills.slice(0, 8).map((sk) => (
                <Badge key={sk} tone="accent">
                  <Icon name="skill" size={10} /> {sk}
                </Badge>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ fontSize: 'var(--fz-xs)' }}>
      <span style={{ color: 'var(--tx-3)' }}>{label} </span>
      <span style={{ color: 'var(--tx-1)', fontFamily: 'var(--font-mono)' }}>{value}</span>
    </div>
  )
}

export function Detect() {
  const { agents, agentsScanned, scanningAgents, scanAgents } = useStore()
  // One detail surface for the whole app: the same drawer Home opens.
  const [opened, setOpened] = useState<DetectedAgent | null>(null)

  // The store owns the cache: scan once, ever, per app session. Switching to
  // this tab and back must NOT re-trigger a scan — only the Rescan button, or
  // the very first visit, does.
  useEffect(() => {
    if (!agentsScanned && !scanningAgents) scanAgents(24)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <Page
      title="Scan & adopt"
      subtitle="Relay reads agents already running on this machine and their on-disk sessions, so you can lift in-flight work into a signed contract and continue it on another provider."
      actions={
        <Button variant="default" icon="refresh" loading={scanningAgents} onClick={() => scanAgents(24)}>
          Rescan
        </Button>
      }
    >
      {scanningAgents && !agentsScanned ? (
        <div style={{ padding: 'var(--s6)', display: 'flex', justifyContent: 'center' }}>
          <Spinner size={22} />
        </div>
      ) : agents.length === 0 ? (
        <EmptyState
          icon="detect"
          title="No agents detected"
          body="Nothing is running and no recent sessions were found in the last 24 hours. Start an agent, or run a task from the Dashboard."
        />
      ) : (
        [...agents].sort(byRecency).map((a) => <AgentCard key={a.id} agent={a} onAdopt={setOpened} />)
      )}

      <SessionDetail agent={opened} onClose={() => setOpened(null)} />
    </Page>
  )
}
