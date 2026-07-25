// Workflow.tsx — "Agents & skills". Build a profile: an ordered chain of
// providers that handle a kind of task, plus the skills to load for it. This is
// where a user decides who handles what and wires skills into the run, once,
// instead of re-explaining it to every agent.

import React, { useEffect, useMemo, useState } from 'react'
import { Page } from '../components/Page'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { providerTitle } from '../lib/format'
import { Button, Input, Field, Badge, EmptyState } from '../components/ui'
import { ChainBuilder } from '../components/ChainBuilder'
import { Icon } from '../components/Icon'
import type { Profile } from '../lib/types'

const KIND_SUGGESTIONS = ['refactor', 'tests', 'bugfix', 'feature', 'review', 'docs', 'research']

function ChipToggle({ label, on, onClick, icon }: { label: string; on: boolean; onClick: () => void; icon?: 'skill' }) {
  return (
    <button
      onClick={onClick}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 5,
        padding: '5px 10px',
        borderRadius: 'var(--r-pill)',
        fontSize: 'var(--fz-sm)',
        fontWeight: 500,
        border: `1px solid ${on ? 'var(--accent-line)' : 'var(--border-1)'}`,
        background: on ? 'var(--accent-weak)' : 'transparent',
        color: on ? 'var(--accent)' : 'var(--tx-1)',
      }}
    >
      {icon && <Icon name="skill" size={11} />}
      {label}
      {on && <Icon name="check" size={11} />}
    </button>
  )
}

function ProfileEditor({
  profile,
  availableProviders,
  availableSkills,
  onSaved,
}: {
  profile: Profile
  availableProviders: string[]
  availableSkills: string[]
  onSaved: () => void
}) {
  const toast = useToast()
  const [draft, setDraft] = useState<Profile>(profile)
  const [kindInput, setKindInput] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => setDraft(profile), [profile])

  const toggleSkill = (s: string) =>
    setDraft((d) => ({
      ...d,
      skills: d.skills.includes(s) ? d.skills.filter((x) => x !== s) : [...d.skills, s],
    }))
  const addKind = (k: string) => {
    const v = k.trim().toLowerCase()
    if (v && !draft.kinds.includes(v)) setDraft((d) => ({ ...d, kinds: [...d.kinds, v] }))
    setKindInput('')
  }

  const save = async () => {
    if (!draft.name.trim()) {
      toast('Give the profile a name first', 'error')
      return
    }
    setBusy(true)
    const r = await api.saveProfile(draft)
    setBusy(false)
    if (r.ok) {
      toast(`Saved profile "${draft.name}"`, 'ok')
      onSaved()
    } else toast(r.error || 'Could not save profile', 'error')
  }

  const skillPool = useMemo(() => {
    const set = new Set([...availableSkills, ...draft.skills])
    return Array.from(set).sort()
  }, [availableSkills, draft.skills])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s4)' }}>
      <Field label="Profile name" hint="What this workflow is for, e.g. backend, ui, quick-fixes.">
        <Input value={draft.name} onChange={(v) => setDraft((d) => ({ ...d, name: v }))} placeholder="backend" />
      </Field>

      <Field label="Provider chain" hint="Relay tries these in order and hands off down the chain as each hits its limit.">
        <ChainBuilder chain={draft.chain} available={availableProviders} onChange={(c) => setDraft((d) => ({ ...d, chain: c }))} />
      </Field>

      <Field label="Task kinds" hint="Route work of these kinds to this profile. Type and press Enter.">
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: draft.kinds.length ? 8 : 0 }}>
          {draft.kinds.map((k) => (
            <Badge key={k} tone="blue" style={{ paddingRight: 4 }}>
              {k}
              <button onClick={() => setDraft((d) => ({ ...d, kinds: d.kinds.filter((x) => x !== k) }))} style={{ color: 'inherit', display: 'flex' }}>
                <Icon name="x" size={10} />
              </button>
            </Badge>
          ))}
        </div>
        <Input value={kindInput} onChange={setKindInput} placeholder="refactor" onEnter={() => addKind(kindInput)} />
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8 }}>
          {KIND_SUGGESTIONS.filter((k) => !draft.kinds.includes(k)).map((k) => (
            <button
              key={k}
              onClick={() => addKind(k)}
              style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-2)', padding: '3px 8px', borderRadius: 'var(--r-pill)', border: '1px dashed var(--border-1)' }}
            >
              + {k}
            </button>
          ))}
        </div>
      </Field>

      <Field label="Skills to load" hint="Skills Relay carries into the run and re-attaches on every handoff, so the next agent has the same toolkit.">
        {skillPool.length === 0 ? (
          <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>
            No skills detected. Skills found in your project's instruction files appear here automatically.
          </div>
        ) : (
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {skillPool.map((s) => (
              <ChipToggle key={s} label={s} icon="skill" on={draft.skills.includes(s)} onClick={() => toggleSkill(s)} />
            ))}
          </div>
        )}
      </Field>

      <Field label="Context hint" hint="One line the router uses to match this profile, e.g. 'Go daemon and HTTP handlers'.">
        <Input value={draft.contextHint} onChange={(v) => setDraft((d) => ({ ...d, contextHint: v }))} placeholder="Go backend, orchestrator, FSM" />
      </Field>

      <div>
        <Button variant="primary" icon="check" loading={busy} onClick={save}>
          Save profile
        </Button>
      </div>
    </div>
  )
}

const blankProfile = (): Profile => ({ name: '', chain: [], kinds: [], skills: [], contextHint: '' })

export function Workflow() {
  const { details } = useStore()
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [skills, setSkills] = useState<string[]>([])
  const [selected, setSelected] = useState<Profile | null>(null)
  const [creating, setCreating] = useState(false)

  const load = async () => {
    const [p, ins] = await Promise.all([api.profiles(), api.instructions()])
    setProfiles(p)
    setSkills(ins.skills || [])
    return p
  }
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const availableProviders = details.filter((d) => d.enabled || d.probeStatus === 'available').map((d) => d.name)
  const editing = creating ? blankProfile() : selected

  const onSaved = async () => {
    const p = await load()
    setCreating(false)
    if (!selected && p.length) setSelected(p[p.length - 1])
  }

  return (
    <Page
      title="Agents & skills"
      subtitle="Decide who handles what. A profile is an ordered chain of providers for a kind of task, plus the skills Relay loads and re-attaches on every handoff."
      actions={
        <Button variant="primary" icon="plus" onClick={() => { setCreating(true); setSelected(null) }}>
          New profile
        </Button>
      }
    >
      <div style={{ display: 'grid', gridTemplateColumns: '240px 1fr', gap: 'var(--s5)', alignItems: 'start' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          {profiles.length === 0 && !creating && (
            <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)', padding: 'var(--s2)' }}>No profiles yet.</div>
          )}
          {profiles.map((p) => (
            <button
              key={p.name}
              onClick={() => { setSelected(p); setCreating(false) }}
              style={{
                textAlign: 'left',
                padding: '9px 11px',
                borderRadius: 'var(--r)',
                background: selected?.name === p.name && !creating ? 'var(--active)' : 'transparent',
                border: '1px solid ' + (selected?.name === p.name && !creating ? 'var(--border-1)' : 'transparent'),
              }}
            >
              <div style={{ fontSize: 'var(--fz-md)', fontWeight: 600 }}>{p.name}</div>
              <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 2 }}>
                {p.chain.map(providerTitle).join(' → ') || 'no chain'}
              </div>
            </button>
          ))}
        </div>

        <div style={{ borderLeft: '1px solid var(--border-0)', paddingLeft: 'var(--s5)', minHeight: 300 }}>
          {editing ? (
            <ProfileEditor
              key={creating ? '__new' : editing.name}
              profile={editing}
              availableProviders={availableProviders}
              availableSkills={skills}
              onSaved={onSaved}
            />
          ) : (
            <EmptyState
              icon="workflow"
              title="Pick or create a profile"
              body="Profiles let you route refactors to one chain of agents and tests to another, each carrying its own skills."
            />
          )}
        </div>
      </div>
    </Page>
  )
}
