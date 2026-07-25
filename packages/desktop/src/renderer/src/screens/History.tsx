// History.tsx — the time machine. Every handoff writes a durable record and a
// git snapshot; this replays that trail so a handoff is auditable, not a leap of
// faith. Two lanes: the event timeline and the commit snapshots behind it.

import React, { useEffect, useState } from 'react'
import { Page } from '../components/Page'
import { api } from '../lib/api'
import { relTime, providerTitle, eventTone } from '../lib/format'
import { Badge, EmptyState, Spinner, StatusDot } from '../components/ui'
import { Icon } from '../components/Icon'
import type { HistoryItem, CommitItem } from '../lib/types'

export function History() {
  const [history, setHistory] = useState<HistoryItem[]>([])
  const [commits, setCommits] = useState<CommitItem[]>([])
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([api.history(), api.commits()]).then(([h, c]) => {
      setHistory(h)
      setCommits(c)
      setLoaded(true)
    })
  }, [])

  if (!loaded) {
    return (
      <Page title="Time machine">
        <div style={{ padding: 'var(--s6)', display: 'flex', justifyContent: 'center' }}><Spinner size={22} /></div>
      </Page>
    )
  }

  const empty = history.length === 0 && commits.length === 0

  return (
    <Page
      title="Time machine"
      subtitle="Every handoff is a signed record and a git snapshot. Replay the trail to see exactly where work moved between agents, and back to any point."
    >
      {empty ? (
        <EmptyState icon="history" title="No history yet" body="Once a task runs and hands off, its timeline and snapshots appear here." />
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: 'var(--s5)', alignItems: 'start' }}>
          <div>
            <div style={{ fontSize: 'var(--fz-md)', fontWeight: 600, color: 'var(--tx-1)', marginBottom: 'var(--s3)' }}>Handoff timeline</div>
            <div style={{ position: 'relative', paddingLeft: 18 }}>
              <div style={{ position: 'absolute', left: 5, top: 4, bottom: 4, width: 1, background: 'var(--border-1)' }} />
              {history.length === 0 && <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>No events.</div>}
              {history.map((h) => (
                <div key={h.seq} style={{ position: 'relative', paddingBottom: 'var(--s3)' }}>
                  <div style={{ position: 'absolute', left: -16, top: 5 }}>
                    <StatusDot tone={eventTone(h.event)} />
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                    <Badge tone={eventTone(h.event)}>{h.event}</Badge>
                    {h.provider && <span style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-1)', fontWeight: 500 }}>{providerTitle(h.provider)}</span>}
                    <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>{relTime(h.ts)}</span>
                  </div>
                  <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', marginTop: 4, lineHeight: 1.45 }} className="selectable">
                    {h.summary}
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div>
            <div style={{ fontSize: 'var(--fz-md)', fontWeight: 600, color: 'var(--tx-1)', marginBottom: 'var(--s3)' }}>Snapshot trail</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              {commits.length === 0 && <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>No snapshots.</div>}
              {commits.map((c) => (
                <div key={c.sha} style={{ display: 'flex', alignItems: 'center', gap: 'var(--s3)', padding: '9px 11px', borderRadius: 'var(--r)', border: '1px solid var(--border-0)', background: 'var(--bg-1)' }}>
                  <Icon name="history" size={14} style={{ color: 'var(--tx-3)' }} />
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--fz-xs)', color: 'var(--accent)' }}>{c.short}</span>
                  <span style={{ flex: 1, fontSize: 'var(--fz-sm)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.subject}</span>
                  <span style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>{relTime(c.when)}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </Page>
  )
}
