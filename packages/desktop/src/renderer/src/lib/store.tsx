// store.tsx — the app's single connection + live-data context.
//
// The daemon is the source of truth; this holds a cached mirror of the few
// endpoints most screens need, refreshed on an interval and nudged by the live
// WebSocket. Screens read from here or call `api` directly for one-offs.

import React, { createContext, useContext, useEffect, useRef, useState, useCallback } from 'react'
import { api } from './api'
import type {
  ProviderStatus,
  ProviderDetail,
  SessionInfo,
  WalletEntry,
  EventLine,
  DetectedAgent,
  HistoryItem,
  Profile,
} from './types'

export type Conn = 'checking' | 'up' | 'starting' | 'unreachable'

interface StoreValue {
  conn: Conn
  connDetail?: string
  session: SessionInfo | null
  providers: ProviderStatus[]
  details: ProviderDetail[]
  wallet: WalletEntry[]
  events: EventLine[]
  history: HistoryItem[]
  profiles: Profile[]
  refresh: () => void
  reconnect: () => void
  // Detected-agent scan, cached here (not in the screen) so switching tabs and
  // back doesn't re-scan — only an explicit rescan, or the first-ever visit, does.
  agents: DetectedAgent[]
  agentsScanned: boolean
  scanningAgents: boolean
  scanAgents: (sinceHours?: number) => Promise<void>
}

const StoreCtx = createContext<StoreValue | null>(null)
const MAX_EVENTS = 400

export function StoreProvider({ children }: { children: React.ReactNode }) {
  const [conn, setConn] = useState<Conn>('checking')
  const [connDetail, setConnDetail] = useState<string>()
  const [session, setSession] = useState<SessionInfo | null>(null)
  const [providers, setProviders] = useState<ProviderStatus[]>([])
  const [details, setDetails] = useState<ProviderDetail[]>([])
  const [wallet, setWallet] = useState<WalletEntry[]>([])
  const [events, setEvents] = useState<EventLine[]>([])
  const [history, setHistory] = useState<HistoryItem[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [agents, setAgents] = useState<DetectedAgent[]>([])
  const [agentsScanned, setAgentsScanned] = useState(false)
  const [scanningAgents, setScanningAgents] = useState(false)
  const timer = useRef<ReturnType<typeof setInterval>>()

  const scanAgents = useCallback(async (sinceHours = 24) => {
    setScanningAgents(true)
    const found = await api.detect(sinceHours)
    setAgents(found)
    setScanningAgents(false)
    setAgentsScanned(true)
  }, [])

  const refresh = useCallback(async () => {
    const [s, p, d, w, h, pf] = await Promise.all([
      api.status(),
      api.providers(),
      api.providerDetails(),
      api.wallet(),
      api.history(),
      api.profiles(),
    ])
    setSession(s)
    setProviders(p)
    setDetails(d)
    setWallet(w)
    setHistory(h)
    setProfiles(pf)
  }, [])

  const boot = useCallback(async () => {
    const status = (await api.ensureDaemon()) as Conn
    setConn(status)
    if (status === 'up') refresh()
  }, [refresh])

  useEffect(() => {
    boot()
    const offStatus = window.relay.onDaemonStatus(({ status, detail }) => {
      setConn(status as Conn)
      setConnDetail(detail)
    })
    const offEvent = window.relay.onEvent((raw) => {
      const msg = raw as EventLine
      if (msg?.type === '_open') {
        setConn('up')
        refresh()
        return
      }
      if (msg?.type === '_close') return
      setEvents((prev) => {
        const next = [...prev, { ...msg, id: msg.id ?? Date.now() + Math.random() }]
        return next.length > MAX_EVENTS ? next.slice(next.length - MAX_EVENTS) : next
      })
      // A handoff or state change is worth an immediate re-read.
      if (typeof msg?.tag === 'string' && /handoff|quota|state/i.test(msg.tag)) refresh()
    })
    return () => {
      offStatus()
      offEvent()
    }
  }, [boot, refresh])

  // Poll while up. Cheap endpoints; the WS covers the fast-moving event stream.
  useEffect(() => {
    if (timer.current) clearInterval(timer.current)
    if (conn === 'up') {
      timer.current = setInterval(refresh, 3000)
    }
    return () => {
      if (timer.current) clearInterval(timer.current)
    }
  }, [conn, refresh])

  const value: StoreValue = {
    conn,
    connDetail,
    session,
    providers,
    details,
    wallet,
    events,
    history,
    profiles,
    refresh,
    reconnect: boot,
    agents,
    agentsScanned,
    scanningAgents,
    scanAgents,
  }
  return <StoreCtx.Provider value={value}>{children}</StoreCtx.Provider>
}

export function useStore(): StoreValue {
  const v = useContext(StoreCtx)
  if (!v) throw new Error('useStore must be used within StoreProvider')
  return v
}
