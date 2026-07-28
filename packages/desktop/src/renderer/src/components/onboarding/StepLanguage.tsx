// components/onboarding/StepLanguage.tsx — step 1. Says what Relay does in one
// short paragraph, then lets the user pick the language the rest of the app
// speaks. Locales come from lib/i18n; this file never edits that list.

import React from 'react'
import { LOCALES, setLocale, useLocale } from '../../lib/i18n'
import { Icon } from '../Icon'
import { StepHead } from './wizard'

export function StepLanguage() {
  const { locale, t } = useLocale()
  return (
    <>
      <StepHead eyebrow={t('onboarding.language')} title={t('onboarding.welcomeTitle')} body={t('onboarding.welcomeBody')} />

      <div
        style={{
          fontSize: 'var(--fz-xs)',
          fontWeight: 600,
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--tx-3)',
          marginBottom: 'var(--s2)',
        }}
      >
        {t('onboarding.language')}
      </div>

      <div style={{ display: 'flex', gap: 'var(--s2)', flexWrap: 'wrap' }}>
        {LOCALES.map((l) => {
          const on = locale === l.code
          return (
            <button
              key={l.code}
              onClick={() => setLocale(l.code)}
              aria-pressed={on}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                padding: '7px 13px',
                borderRadius: 'var(--r-pill)',
                border: `1px solid ${on ? 'var(--accent-line)' : 'var(--border-1)'}`,
                background: on ? 'var(--accent-weak)' : 'transparent',
                color: on ? 'var(--accent)' : 'var(--tx-1)',
                fontSize: 'var(--fz-md)',
                fontWeight: 500,
                transition: 'background 0.14s var(--ease), border-color 0.14s var(--ease)',
              }}
            >
              {on && <Icon name="check" size={12} />}
              {l.label}
            </button>
          )
        })}
      </div>

      <p style={{ fontSize: 'var(--fz-xs)', color: 'var(--tx-3)', marginTop: 'var(--s3)', lineHeight: 1.5 }}>
        You can change this later in Settings. Nothing is sent anywhere, the language is stored on this
        computer.
      </p>
    </>
  )
}
