// Console.tsx — full, direct access to the daemon's /api/* surface. Every
// screen in this app is really just a curated view over these same endpoints;
// this one skips the curation. Pick a known route or type your own path, GET
// or POST with a JSON body, see the raw response. Same security boundary as
// everywhere else: the preload bridge only allows /api/* on 127.0.0.1:4748.

import React, { useMemo, useState } from 'react'
import { Page } from '../components/Page'
import { api, KNOWN_ENDPOINTS } from '../lib/api'
import { Button, Badge, Field } from '../components/ui'
import { Icon } from '../components/Icon'

interface Call {
  id: number
  method: 'GET' | 'POST'
  path: string
  status: number
  ok: boolean
  ms: number
}

let seq = 1

export function Console() {
  const [method, setMethod] = useState<'GET' | 'POST'>('GET')
  const [path, setPath] = useState('/api/health')
  const [body, setBody] = useState('')
  const [bodyError, setBodyError] = useState<string | null>(null)
  const [response, setResponse] = useState<{ status: number; ok: boolean; data: unknown } | null>(null)
  const [busy, setBusy] = useState(false)
  const [history, setHistory] = useState<Call[]>([])

  const filtered = useMemo(
    () => KNOWN_ENDPOINTS.filter((e) => e.path.toLowerCase().includes(path.toLowerCase()) || path === ''),
    [path],
  )

  const pick = (e: (typeof KNOWN_ENDPOINTS)[number]) => {
    setMethod(e.method)
    setPath(e.path)
    setBody(e.note && e.method === 'POST' ? '' : body)
    setResponse(null)
  }

  const send = async () => {
    setBodyError(null)
    let parsedBody: unknown
    if (method === 'POST' && body.trim()) {
      try {
        parsedBody = JSON.parse(body)
      } catch {
        setBodyError('Body is not valid JSON')
        return
      }
    }
    setBusy(true)
    const t0 = performance.now()
    const r = method === 'GET' ? await api.raw.get(path) : await api.raw.post(path, parsedBody)
    const ms = Math.round(performance.now() - t0)
    setBusy(false)
    setResponse({ status: r.status, ok: r.ok, data: 'data' in r ? r.data : r.error })
    setHistory((h) => [{ id: seq++, method, path, status: r.status, ok: r.ok, ms }, ...h].slice(0, 20))
  }

  return (
    <Page
      title="API console"
      subtitle="Direct, full access to every daemon endpoint. Every screen in this app is a curated view over this same surface; use this when you need something they don't show."
    >
      <div style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: 'var(--s5)', alignItems: 'start' }}>
        <div>
          <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--tx-3)', marginBottom: 8 }}>
            Known routes
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2, maxHeight: 480, overflow: 'auto' }}>
            {filtered.map((e, i) => (
              <button
                key={i}
                onClick={() => pick(e)}
                style={{
                  textAlign: 'left',
                  padding: '7px 9px',
                  borderRadius: 'var(--r)',
                  background: path === e.path && method === e.method ? 'var(--active)' : 'transparent',
                  fontFamily: 'var(--font-mono)',
                  fontSize: 'var(--fz-xs)',
                }}
              >
                <span style={{ color: e.method === 'GET' ? 'var(--blue)' : 'var(--accent)', fontWeight: 700, marginRight: 6 }}>{e.method}</span>
                <span style={{ color: 'var(--tx-1)' }}>{e.path}</span>
                {e.note && <div style={{ color: 'var(--tx-3)', marginTop: 2, fontFamily: 'var(--font-ui)' }}>{e.note}</div>}
              </button>
            ))}
          </div>
        </div>

        <div>
          <div style={{ display: 'flex', gap: 6, marginBottom: 'var(--s3)' }}>
            <select
              value={method}
              onChange={(e) => setMethod(e.target.value as 'GET' | 'POST')}
              style={{ width: 90, height: 34, borderRadius: 'var(--r)', background: 'var(--bg-0)', border: '1px solid var(--border-1)', color: 'var(--tx-0)', fontWeight: 700, fontFamily: 'var(--font-mono)' }}
            >
              <option value="GET">GET</option>
              <option value="POST">POST</option>
            </select>
            <input
              value={path}
              onChange={(e) => setPath(e.target.value)}
              placeholder="/api/health"
              className="selectable mono"
              style={{ flex: 1, height: 34, padding: '0 11px', borderRadius: 'var(--r)', background: 'var(--bg-0)', border: '1px solid var(--border-1)', color: 'var(--tx-0)', fontFamily: 'var(--font-mono)', fontSize: 'var(--fz-sm)' }}
            />
            <Button variant="primary" icon="play" loading={busy} onClick={send}>
              Send
            </Button>
          </div>

          {method === 'POST' && (
            <Field label="Body (JSON)" style={{ marginBottom: 'var(--s3)' }}>
              <textarea
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder={'{\n  "name": "claude"\n}'}
                rows={5}
                className="selectable"
                style={{
                  width: '100%',
                  resize: 'vertical',
                  padding: '9px 11px',
                  borderRadius: 'var(--r)',
                  background: 'var(--bg-0)',
                  border: `1px solid ${bodyError ? 'var(--red)' : 'var(--border-1)'}`,
                  color: 'var(--tx-0)',
                  fontFamily: 'var(--font-mono)',
                  fontSize: 'var(--fz-sm)',
                }}
              />
              {bodyError && <div style={{ color: 'var(--red)', fontSize: 'var(--fz-xs)', marginTop: 4 }}>{bodyError}</div>}
            </Field>
          )}

          <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--tx-3)', marginBottom: 8 }}>
            Response
          </div>
          <div
            className="selectable"
            style={{
              minHeight: 140,
              maxHeight: 320,
              overflow: 'auto',
              padding: 'var(--s3)',
              borderRadius: 'var(--r-lg)',
              background: 'var(--bg-0)',
              border: '1px solid var(--border-0)',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--fz-sm)',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {response ? (
              <>
                <div style={{ marginBottom: 8 }}>
                  <Badge tone={response.ok ? 'green' : 'red'}>{response.status || 'error'}</Badge>
                </div>
                {typeof response.data === 'string' ? response.data : JSON.stringify(response.data, null, 2)}
              </>
            ) : (
              <span style={{ color: 'var(--tx-3)' }}>Send a request to see its response.</span>
            )}
          </div>

          {history.length > 0 && (
            <div style={{ marginTop: 'var(--s4)' }}>
              <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--tx-3)', marginBottom: 8 }}>
                Recent calls
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {history.map((c) => (
                  <div key={c.id} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 'var(--fz-xs)', fontFamily: 'var(--font-mono)', color: 'var(--tx-3)', padding: '3px 0' }}>
                    <Icon name={c.ok ? 'check' : 'x'} size={11} style={{ color: c.ok ? 'var(--green)' : 'var(--red)' }} />
                    <span style={{ color: c.method === 'GET' ? 'var(--blue)' : 'var(--accent)', fontWeight: 700 }}>{c.method}</span>
                    <span style={{ color: 'var(--tx-1)' }}>{c.path}</span>
                    <span>{c.status}</span>
                    <span>{c.ms}ms</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </Page>
  )
}
