// Onboarding.tsx — the sixty-second first run. Three steps: what this is,
// enable the agents you have, sign in to one account. Every step is skippable;
// the goal is a working rotation, not a form.

import React, { useState } from 'react'
import { api } from '../lib/api'
import { useStore } from '../lib/store'
import { useToast } from '../lib/toast'
import { providerTitle } from '../lib/format'
import { Button, Input, ProviderGlyph } from '../components/ui'
import { Icon } from '../components/Icon'
import { LOCALES, useLocale, setLocale } from '../lib/i18n'

function LanguagePicker() {
  const { locale, t } = useLocale()
  return (
    <div style={{ marginTop: 'var(--s4)' }}>
      <div style={{ fontSize: 'var(--fz-xs)', fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--tx-3)', marginBottom: 8 }}>
        {t('onboarding.language')}
      </div>
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {LOCALES.map((l) => (
          <button
            key={l.code}
            onClick={() => setLocale(l.code)}
            style={{
              padding: '6px 12px',
              borderRadius: 'var(--r-pill)',
              border: `1px solid ${locale === l.code ? 'var(--accent-line)' : 'var(--border-1)'}`,
              background: locale === l.code ? 'var(--accent-weak)' : 'transparent',
              color: locale === l.code ? 'var(--accent)' : 'var(--tx-1)',
              fontSize: 'var(--fz-sm)',
              fontWeight: 500,
            }}
          >
            {l.label}
          </button>
        ))}
      </div>
    </div>
  )
}

function Dots({ step, total }: { step: number; total: number }) {
  return (
    <div style={{ display: 'flex', gap: 6 }}>
      {Array.from({ length: total }).map((_, i) => (
        <span
          key={i}
          style={{
            width: i === step ? 20 : 7,
            height: 7,
            borderRadius: 'var(--r-pill)',
            background: i === step ? 'var(--accent)' : i < step ? 'var(--accent-line)' : 'var(--bg-3)',
            transition: 'all 0.2s var(--ease)',
          }}
        />
      ))}
    </div>
  )
}

export function Onboarding({ onDone }: { onDone: () => void }) {
  const toast = useToast()
  const { t } = useLocale()
  const { details, refresh } = useStore()
  const [step, setStep] = useState(0)
  const [pending, setPending] = useState<Record<string, boolean>>({})
  const [acctProvider, setAcctProvider] = useState('')
  const [acctLabel, setAcctLabel] = useState('personal')
  const [busy, setBusy] = useState(false)

  const enableable = details.filter((d) => d.canOauth || d.canApiKey || d.kind === 'cli' || d.kind === 'local')
  const enabledNow = details.filter((d) => d.enabled || pending[d.name])

  const toggleEnable = async (name: string, on: boolean) => {
    setPending((p) => ({ ...p, [name]: on }))
    const r = await api.saveProviderConfig({ name, enabled: on })
    if (!r.ok) {
      setPending((p) => ({ ...p, [name]: !on }))
      toast(r.error || 'Could not update provider', 'error')
    } else {
      setTimeout(refresh, 300)
    }
  }

  const addFirstAccount = async () => {
    if (!acctProvider || !acctLabel.trim()) return
    setBusy(true)
    const add = await api.addAccount(acctProvider, acctLabel.trim(), '')
    if (!add.ok) {
      setBusy(false)
      toast(add.error || 'Could not add account', 'error')
      return
    }
    await api.loginAccount(acctProvider, acctLabel.trim())
    setBusy(false)
    toast(`Added "${acctLabel}". Finish sign-in in the window that opens.`, 'ok')
    refresh()
    onDone()
  }

  const card: React.CSSProperties = {
    width: 560,
    maxWidth: '100%',
    background: 'var(--bg-1)',
    border: '1px solid var(--border-1)',
    borderRadius: 'var(--r-lg)',
    padding: 'var(--s6)',
    boxShadow: '0 24px 70px rgba(0,0,0,0.5)',
    animation: 'relay-rise 0.28s var(--ease) both',
  }

  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 'var(--s6)',
        background: 'radial-gradient(1000px 600px at 50% 20%, var(--bg-1), var(--bg-0) 70%)',
        overflow: 'auto',
      }}
    >
      <div style={card}>
        {step === 0 && (
          <>
            <div
              style={{
                width: 46,
                height: 46,
                borderRadius: 'var(--r-lg)',
                background: 'var(--accent-weak)',
                border: '1px solid var(--accent-line)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--accent)',
                marginBottom: 'var(--s4)',
              }}
            >
              <Icon name="handoff" size={24} />
            </div>
            <h2 style={{ fontSize: 'var(--fz-2xl)', fontWeight: 700, letterSpacing: '-0.01em' }}>{t('onboarding.welcomeTitle')}</h2>
            <p style={{ fontSize: 'var(--fz-md)', color: 'var(--tx-2)', marginTop: 'var(--s2)', lineHeight: 1.6 }}>
              {t('onboarding.welcomeBody')}
            </p>
            <LanguagePicker />
          </>
        )}

        {step === 1 && (
          <>
            <h2 style={{ fontSize: 'var(--fz-xl)', fontWeight: 700 }}>{t('onboarding.enableTitle')}</h2>
            <p style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', marginTop: 6, lineHeight: 1.55 }}>
              {t('onboarding.enableBody')}
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 'var(--s4)', maxHeight: 300, overflow: 'auto' }}>
              {enableable.map((d) => {
                const on = d.enabled || !!pending[d.name]
                return (
                  <button
                    key={d.name}
                    onClick={() => toggleEnable(d.name, !on)}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 'var(--s3)',
                      padding: '10px 12px',
                      borderRadius: 'var(--r)',
                      border: `1px solid ${on ? 'var(--accent-line)' : 'var(--border-1)'}`,
                      background: on ? 'var(--accent-weak)' : 'transparent',
                      textAlign: 'left',
                    }}
                  >
                    <ProviderGlyph name={d.name} size={30} />
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 'var(--fz-md)', fontWeight: 600 }}>{d.displayName || providerTitle(d.name)}</div>
                      <div style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>{d.kind}</div>
                    </div>
                    <span
                      style={{
                        width: 20,
                        height: 20,
                        borderRadius: '50%',
                        border: `1.5px solid ${on ? 'var(--accent)' : 'var(--border-2)'}`,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'var(--accent-ink)',
                        background: on ? 'var(--accent)' : 'transparent',
                      }}
                    >
                      {on && <Icon name="check" size={12} />}
                    </span>
                  </button>
                )
              })}
            </div>
            <div style={{ marginTop: 'var(--s3)', fontSize: 'var(--fz-xs)', color: 'var(--tx-3)' }}>
              {enabledNow.length} enabled
            </div>
          </>
        )}

        {step === 2 && (
          <>
            <h2 style={{ fontSize: 'var(--fz-xl)', fontWeight: 700 }}>{t('onboarding.accountTitle')}</h2>
            <p style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-2)', marginTop: 6, lineHeight: 1.55 }}>
              {t('onboarding.accountBody')}
            </p>
            <div style={{ marginTop: 'var(--s4)', display: 'flex', flexDirection: 'column', gap: 'var(--s3)' }}>
              <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                {enabledNow.length === 0 ? (
                  <div style={{ fontSize: 'var(--fz-sm)', color: 'var(--tx-3)' }}>Enable a provider first to sign in.</div>
                ) : (
                  enabledNow.map((d) => (
                    <button
                      key={d.name}
                      onClick={() => setAcctProvider(d.name)}
                      style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: 7,
                        padding: '7px 12px',
                        borderRadius: 'var(--r-pill)',
                        border: `1px solid ${acctProvider === d.name ? 'var(--accent-line)' : 'var(--border-1)'}`,
                        background: acctProvider === d.name ? 'var(--accent-weak)' : 'transparent',
                        color: acctProvider === d.name ? 'var(--accent)' : 'var(--tx-1)',
                        fontSize: 'var(--fz-sm)',
                        fontWeight: 500,
                      }}
                    >
                      <ProviderGlyph name={d.name} size={20} />
                      {providerTitle(d.name)}
                    </button>
                  ))
                )}
              </div>
              {acctProvider && (
                <div style={{ display: 'flex', gap: 6 }}>
                  <Input value={acctLabel} onChange={setAcctLabel} placeholder="personal" onEnter={addFirstAccount} />
                  <Button variant="primary" icon="key" loading={busy} onClick={addFirstAccount}>
                    {t('onboarding.addAndSignIn')}
                  </Button>
                </div>
              )}
            </div>
          </>
        )}

        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginTop: 'var(--s6)',
            paddingTop: 'var(--s4)',
            borderTop: '1px solid var(--border-0)',
          }}
        >
          <Dots step={step} total={3} />
          <div style={{ display: 'flex', gap: 'var(--s2)' }}>
            {step > 0 && (
              <Button variant="ghost" onClick={() => setStep((s) => s - 1)}>
                {t('onboarding.back')}
              </Button>
            )}
            {step < 2 ? (
              <Button variant="primary" onClick={() => setStep((s) => s + 1)}>
                {step === 0 ? t('onboarding.getStarted') : t('onboarding.next')}
              </Button>
            ) : (
              <Button variant="ghost" onClick={onDone}>
                {t('onboarding.finish')}
              </Button>
            )}
          </div>
        </div>

        {step > 0 && (
          <button
            onClick={onDone}
            style={{ position: 'absolute', top: 'var(--s4)', right: 'var(--s4)', color: 'var(--tx-3)', fontSize: 'var(--fz-xs)', padding: 4 }}
          >
            {t('onboarding.skip')}
          </button>
        )}
      </div>
    </div>
  )
}
