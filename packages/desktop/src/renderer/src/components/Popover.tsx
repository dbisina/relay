// Popover.tsx — a small anchored panel for toolbar chips (Providers, Profile,
// Threshold, Max handoffs). Lighter than the full-screen Sheet modal: it hangs
// off its trigger and closes on outside click or Escape.

import React, { useEffect, useRef } from 'react'

export function Popover({
  open,
  onClose,
  anchorRight,
  width = 300,
  children,
}: {
  open: boolean
  onClose: () => void
  anchorRight?: boolean
  width?: number
  children: React.ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      ref={ref}
      style={{
        position: 'absolute',
        bottom: 'calc(100% + 8px)',
        ...(anchorRight ? { right: 0 } : { left: 0 }),
        width,
        maxHeight: 360,
        overflow: 'auto',
        background: 'var(--bg-2)',
        border: '1px solid var(--border-2)',
        borderRadius: 'var(--r-lg)',
        boxShadow: '0 16px 44px rgba(0,0,0,0.55)',
        padding: 'var(--s3)',
        zIndex: 30,
        animation: 'relay-rise 0.16s var(--ease) both',
      }}
    >
      {children}
    </div>
  )
}
