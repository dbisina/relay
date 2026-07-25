// Pipelines.tsx — multi-agent DAGs: several agents on dependent sub-tasks, each
// with fallbacks and acceptance checks. List saved pipelines, inspect their
// shape, run them, and build new ones or edit existing ones right here.

import React, { useEffect, useState } from 'react'
import { Page } from '../components/Page'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { providerTitle } from '../lib/format'
import { Button, Input, Field, Badge, EmptyState, Spinner, Sheet } from '../components/ui'
import { ChainBuilder } from '../components/ChainBuilder'
import { Icon } from '../components/Icon'
import type { Pipeline, PipelineNode } from '../lib/types'

function NodeCard({ node }: { node: PipelineNode }) {
  return (
    <div
      style={{
        border: '1px solid var(--border-1)',
        borderRadius: 'var(--r)',
        background: 'var(--bg-0)',
        padding: 'var(--s3)',
        minWidth: 190,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>{node.id}</span>
        {node.provider && <Badge tone="accent">{providerTitle(node.provider)}</Badge>}
      </div>
      <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-1)', marginTop: 6, lineHeight: 1.45 }}>
        {node.task || 'no task'}
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginTop: 8 }}>
        {node.dependsOn?.length > 0 && (
          <Meta icon="pipelines" text={`after ${node.dependsOn.join(', ')}`} />
        )}
        {node.fallback?.length > 0 && (
          <Meta icon="handoff" text={`fallback ${node.fallback.map(providerTitle).join(' → ')}`} />
        )}
        {node.verify?.length > 0 && <Meta icon="check" text={`${node.verify.length} acceptance check${node.verify.length > 1 ? 's' : ''}`} />}
      </div>
    </div>
  )
}
function Meta({ icon, text }: { icon: 'pipelines' | 'handoff' | 'check'; text: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>
      <Icon name={icon} size={12} />
      <span style={{ fontFamily: 'var(--font-mono)' }}>{text}</span>
    </div>
  )
}

function PipelineCard({ p, onRun, onEdit }: { p: Pipeline; onRun: () => void; onEdit: () => void }) {
  return (
    <div style={{ border: '1px solid var(--border-0)', borderRadius: 'var(--r-lg)', background: 'var(--bg-1)', padding: 'var(--s4)', marginBottom: 'var(--s3)' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--s3)' }}>
        <div>
          <div style={{ fontSize: 'var(--fz-lg)', fontWeight: 600 }}>{p.name}</div>
          <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 2 }}>
            {p.nodes.length} stage{p.nodes.length !== 1 ? 's' : ''}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 'var(--s2)' }}>
          <Button variant="ghost" icon="workflow" onClick={onEdit}>
            Edit
          </Button>
          <Button variant="primary" icon="play" onClick={onRun}>
            Run
          </Button>
        </div>
      </div>
      <div style={{ display: 'flex', gap: 'var(--s2)', flexWrap: 'wrap', overflowX: 'auto', paddingBottom: 4 }}>
        {p.nodes.map((n, i) => (
          <React.Fragment key={n.id}>
            {i > 0 && <div style={{ alignSelf: 'center', color: 'var(--tx-3)' }}><Icon name="chevronRight" size={14} /></div>}
            <NodeCard node={n} />
          </React.Fragment>
        ))}
      </div>
    </div>
  )
}

// ── Editor: one node's full form ─────────────────────────────────────────────
function NodeEditor({
  node,
  otherIds,
  availableProviders,
  onChange,
  onRemove,
}: {
  node: PipelineNode
  otherIds: string[]
  availableProviders: string[]
  onChange: (n: PipelineNode) => void
  onRemove: () => void
}) {
  const [verifyInput, setVerifyInput] = useState('')
  const toggleDependsOn = (id: string) =>
    onChange({ ...node, dependsOn: node.dependsOn.includes(id) ? node.dependsOn.filter((x) => x !== id) : [...node.dependsOn, id] })
  const addVerify = () => {
    const v = verifyInput.trim()
    if (!v) return
    onChange({ ...node, verify: [...node.verify, v] })
    setVerifyInput('')
  }

  return (
    <div style={{ border: '1px solid var(--border-1)', borderRadius: 'var(--r-lg)', background: 'var(--bg-0)', padding: 'var(--s3)', marginBottom: 'var(--s3)' }}>
      <div style={{ display: 'flex', gap: 'var(--s2)', alignItems: 'flex-end', marginBottom: 'var(--s3)' }}>
        <Field label="Stage ID" style={{ width: 110 }}>
          <Input value={node.id} onChange={(v) => onChange({ ...node, id: v.replace(/\s+/g, '-') })} placeholder="stage-1" />
        </Field>
        <Field label="Provider" style={{ flex: 1 }}>
          <select
            value={node.provider}
            onChange={(e) => onChange({ ...node, provider: e.target.value })}
            style={{ width: '100%', height: 34, padding: '0 10px', borderRadius: 'var(--r)', background: 'var(--bg-0)', border: '1px solid var(--border-1)', color: 'var(--tx-0)', fontSize: 'var(--fz-md)' }}
          >
            <option value="">Pick a provider</option>
            {availableProviders.map((p) => (
              <option key={p} value={p}>
                {providerTitle(p)}
              </option>
            ))}
          </select>
        </Field>
        <Button variant="ghost" icon="x" onClick={onRemove} title="Remove stage" />
      </div>

      <Field label="Task" style={{ marginBottom: 'var(--s3)' }}>
        <textarea
          value={node.task}
          onChange={(e) => onChange({ ...node, task: e.target.value })}
          placeholder="What this stage should do"
          rows={2}
          className="selectable"
          style={{ width: '100%', resize: 'vertical', padding: '8px 11px', borderRadius: 'var(--r)', background: 'var(--bg-0)', border: '1px solid var(--border-1)', color: 'var(--tx-0)', fontFamily: 'var(--font-ui)', fontSize: 'var(--fz-md)' }}
        />
      </Field>

      {otherIds.length > 0 && (
        <Field label="Depends on" hint="Runs after these stages complete." style={{ marginBottom: 'var(--s3)' }}>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {otherIds.map((id) => (
              <button
                key={id}
                onClick={() => toggleDependsOn(id)}
                style={{
                  padding: '4px 10px',
                  borderRadius: 'var(--r-pill)',
                  fontSize: 'var(--fz-xs)',
                  fontFamily: 'var(--font-mono)',
                  border: `1px solid ${node.dependsOn.includes(id) ? 'var(--accent-line)' : 'var(--border-1)'}`,
                  background: node.dependsOn.includes(id) ? 'var(--accent-weak)' : 'transparent',
                  color: node.dependsOn.includes(id) ? 'var(--accent)' : 'var(--tx-2)',
                }}
              >
                {id}
              </button>
            ))}
          </div>
        </Field>
      )}

      <Field label="Fallback chain" hint="If this stage's provider fails, try these next." style={{ marginBottom: 'var(--s3)' }}>
        <ChainBuilder chain={node.fallback} available={availableProviders.filter((p) => p !== node.provider)} onChange={(c) => onChange({ ...node, fallback: c })} emptyHint="No fallback set." />
      </Field>

      <Field label="Acceptance checks" hint="Shell commands that must pass before this stage is considered done.">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginBottom: node.verify.length ? 8 : 0 }}>
          {node.verify.map((v, i) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <code style={{ flex: 1, fontSize: 'var(--fz-xs)', padding: '5px 9px', background: 'var(--bg-1)', borderRadius: 'var(--r)', border: '1px solid var(--border-1)' }}>
                {v}
              </code>
              <button onClick={() => onChange({ ...node, verify: node.verify.filter((_, j) => j !== i) })} style={{ color: 'var(--tx-2)', padding: 2 }}>
                <Icon name="x" size={12} />
              </button>
            </div>
          ))}
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <Input value={verifyInput} onChange={setVerifyInput} placeholder="npm test" mono onEnter={addVerify} />
          <Button variant="default" icon="plus" onClick={addVerify}>
            Add
          </Button>
        </div>
      </Field>
    </div>
  )
}

let seq = 1
const blankNode = (): PipelineNode => ({ id: `stage-${seq++}`, provider: '', task: '', dependsOn: [], fallback: [], verify: [] })
const blankPipeline = (): Pipeline => ({ name: '', nodes: [blankNode()] })

function PipelineEditor({
  pipeline,
  availableProviders,
  onSaved,
  onClose,
}: {
  pipeline: Pipeline
  availableProviders: string[]
  onSaved: () => void
  onClose: () => void
}) {
  const toast = useToast()
  const [draft, setDraft] = useState<Pipeline>(pipeline)
  const [busy, setBusy] = useState(false)

  const updateNode = (i: number, n: PipelineNode) =>
    setDraft((d) => ({ ...d, nodes: d.nodes.map((x, j) => (j === i ? n : x)) }))
  const removeNode = (i: number) => setDraft((d) => ({ ...d, nodes: d.nodes.filter((_, j) => j !== i) }))
  const addNode = () => setDraft((d) => ({ ...d, nodes: [...d.nodes, blankNode()] }))

  const save = async () => {
    if (!draft.name.trim()) {
      toast('Give the pipeline a name first', 'error')
      return
    }
    if (draft.nodes.some((n) => !n.id.trim() || !n.provider)) {
      toast('Every stage needs an ID and a provider', 'error')
      return
    }
    setBusy(true)
    const r = await api.savePipeline(draft)
    setBusy(false)
    if (r.ok) {
      toast(`Saved pipeline "${draft.name}"`, 'ok')
      onSaved()
    } else toast(r.error || 'Could not save pipeline', 'error')
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s4)' }}>
      <Field label="Pipeline name">
        <Input value={draft.name} onChange={(v) => setDraft((d) => ({ ...d, name: v }))} placeholder="feature-rollout" />
      </Field>

      <div>
        <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--tx-2)', marginBottom: 8 }}>
          Stages
        </div>
        {draft.nodes.map((n, i) => (
          <NodeEditor
            key={i}
            node={n}
            otherIds={draft.nodes.map((x) => x.id).filter((id) => id !== n.id)}
            availableProviders={availableProviders}
            onChange={(x) => updateNode(i, x)}
            onRemove={() => removeNode(i)}
          />
        ))}
        <Button variant="default" icon="plus" onClick={addNode}>
          Add stage
        </Button>
      </div>

      <div style={{ display: 'flex', gap: 'var(--s2)', paddingTop: 'var(--s2)', borderTop: '1px solid var(--border-0)' }}>
        <Button variant="primary" icon="check" loading={busy} onClick={save}>
          Save pipeline
        </Button>
        <Button variant="ghost" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

export function Pipelines() {
  const toast = useToast()
  const { details } = useStore()
  const [pipelines, setPipelines] = useState<Pipeline[]>([])
  const [loaded, setLoaded] = useState(false)
  const [editing, setEditing] = useState<Pipeline | null>(null)

  const load = async () => {
    setPipelines(await api.pipelines())
    setLoaded(true)
  }
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const availableProviders = details.filter((d) => d.enabled || d.probeStatus === 'available').map((d) => d.name)

  const run = async (name: string) => {
    const r = await api.runPipeline(name)
    if (r.ok) toast(`Running pipeline "${name}"`, 'ok')
    else toast(r.error || 'Could not run pipeline', 'error')
  }

  return (
    <Page
      title="Pipelines"
      subtitle="Fan a job across several agents on dependent sub-tasks, each with its own fallbacks and acceptance checks."
      actions={
        <Button variant="primary" icon="plus" onClick={() => setEditing(blankPipeline())}>
          New pipeline
        </Button>
      }
    >
      {!loaded ? (
        <div style={{ padding: 'var(--s6)', display: 'flex', justifyContent: 'center' }}><Spinner size={22} /></div>
      ) : pipelines.length === 0 ? (
        <EmptyState
          icon="pipelines"
          title="No pipelines yet"
          body="Build one with the New pipeline button, or run one you've already saved."
          action={
            <Button variant="primary" icon="plus" onClick={() => setEditing(blankPipeline())}>
              New pipeline
            </Button>
          }
        />
      ) : (
        pipelines.map((p) => <PipelineCard key={p.name} p={p} onRun={() => run(p.name)} onEdit={() => setEditing(p)} />)
      )}

      <Sheet open={!!editing} onClose={() => setEditing(null)} title={editing?.name ? `Edit "${editing.name}"` : 'New pipeline'} width={640}>
        {editing && (
          <PipelineEditor
            pipeline={editing}
            availableProviders={availableProviders}
            onSaved={() => {
              setEditing(null)
              load()
            }}
            onClose={() => setEditing(null)}
          />
        )}
      </Sheet>
    </Page>
  )
}
