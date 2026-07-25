// toast.tsx — lightweight, self-contained toast stack. One primary channel for
// "that worked" / "that failed" feedback so screens don't each roll their own.

import React, { createContext, useContext, useCallback, useState } from 'react'

type Kind = 'ok' | 'error' | 'info'
interface Toast {
  id: number
  kind: Kind
  text: string
}

const ToastCtx = createContext<{
  push: (text: string, kind?: Kind) => void
} | null>(null)

let seq = 1

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const push = useCallback((text: string, kind: Kind = 'info') => {
    const id = seq++
    setToasts((t) => [...t, { id, kind, text }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4200)
  }, [])

  return (
    <ToastCtx.Provider value={{ push }}>
      {children}
      <div
        style={{
          position: 'fixed',
          bottom: 'var(--s5)',
          left: '50%',
          transform: 'translateX(-50%)',
          display: 'flex',
          flexDirection: 'column',
          gap: 'var(--s2)',
          zIndex: 200,
          pointerEvents: 'none',
        }}
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--s2)',
              padding: '9px 14px',
              borderRadius: 'var(--r-lg)',
              fontSize: 'var(--fz-sm)',
              color: 'var(--tx-0)',
              background: 'var(--bg-3)',
              border: `1px solid ${
                t.kind === 'error'
                  ? 'var(--red)'
                  : t.kind === 'ok'
                    ? 'var(--green)'
                    : 'var(--border-2)'
              }`,
              boxShadow: '0 12px 30px rgba(0,0,0,0.5)',
              animation: 'relay-rise 0.24s var(--ease) both',
              maxWidth: 460,
            }}
          >
            <span
              style={{
                width: 7,
                height: 7,
                borderRadius: '50%',
                flexShrink: 0,
                background:
                  t.kind === 'error'
                    ? 'var(--red)'
                    : t.kind === 'ok'
                      ? 'var(--green)'
                      : 'var(--tx-2)',
              }}
            />
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}

export function useToast() {
  const v = useContext(ToastCtx)
  if (!v) throw new Error('useToast must be used within ToastProvider')
  return v.push
}
