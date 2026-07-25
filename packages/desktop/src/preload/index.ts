// src/preload/index.ts
//
// The single, frozen API surface exposed to the renderer as `window.relay`.
// Everything the UI can do to the outside world is enumerated here; there is no
// other channel. Keep this thin: it forwards to the main-process IPC handlers.

import { contextBridge, ipcRenderer } from 'electron'

export interface RelayResponse<T = unknown> {
  ok: boolean
  status: number
  data?: T
  error?: string
}

const relay = {
  /** GET /api/<path>. `path` must start with /api/. */
  get<T = unknown>(path: string): Promise<RelayResponse<T>> {
    return ipcRenderer.invoke('relay:request', { method: 'GET', path })
  },
  /** POST /api/<path> with an optional JSON body. */
  post<T = unknown>(path: string, body?: unknown): Promise<RelayResponse<T>> {
    return ipcRenderer.invoke('relay:request', { method: 'POST', path, body })
  },
  /** One-shot health probe. */
  health(): Promise<boolean> {
    return ipcRenderer.invoke('relay:health')
  },
  /** Ensure the daemon is up (start it if needed). Resolves with final status. */
  ensureDaemon(): Promise<'checking' | 'up' | 'starting' | 'unreachable'> {
    return ipcRenderer.invoke('relay:ensureDaemon')
  },
  /** Subscribe to daemon-status transitions. Returns an unsubscribe fn. */
  onDaemonStatus(cb: (s: { status: string; detail?: string }) => void): () => void {
    const h = (_e: unknown, payload: { status: string; detail?: string }) => cb(payload)
    ipcRenderer.on('relay:daemonStatus', h)
    return () => ipcRenderer.removeListener('relay:daemonStatus', h)
  },
  /** Subscribe to the live WebSocket event stream. Returns an unsubscribe fn. */
  onEvent(cb: (msg: unknown) => void): () => void {
    const h = (_e: unknown, payload: unknown) => cb(payload)
    ipcRenderer.on('relay:ws', h)
    return () => ipcRenderer.removeListener('relay:ws', h)
  },
  /** Native folder picker. Resolves to an absolute path or null. */
  pickFolder(): Promise<string | null> {
    return ipcRenderer.invoke('relay:pickFolder')
  },
  /** OS account username, for the home screen's greeting. Empty string if unavailable. */
  whoami(): Promise<string> {
    return ipcRenderer.invoke('relay:whoami')
  },
  /** Open an http(s) URL in the user's real browser. */
  openExternal(url: string): void {
    ipcRenderer.invoke('relay:openExternal', url)
  },
  /** Frameless-window controls. */
  win: {
    minimize: () => ipcRenderer.send('win:minimize'),
    maximizeToggle: () => ipcRenderer.send('win:maximizeToggle'),
    close: () => ipcRenderer.send('win:close'),
  },
}

contextBridge.exposeInMainWorld('relay', relay)

export type RelayApi = typeof relay
