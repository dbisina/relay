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

/** Mirror of DaemonState in main/daemon.ts. */
export interface DaemonStateDto {
  status: 'checking' | 'starting' | 'restarting' | 'up' | 'failed'
  detail?: string
  logTail: string[]
  external: boolean
  binaryPath: string
  binaryFound: boolean
  triedPaths: string[]
  logPath: string
  workDir: string
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
  ensureDaemon(): Promise<'checking' | 'starting' | 'restarting' | 'up' | 'failed'> {
    return ipcRenderer.invoke('relay:ensureDaemon')
  },
  /** Full daemon state including the binary search path and its log tail. */
  daemonState(): Promise<DaemonStateDto> {
    return ipcRenderer.invoke('relay:daemonState')
  },
  /** Reveal the daemon log in the OS file manager. */
  openLogFolder(): void {
    ipcRenderer.invoke('relay:openLogFolder')
  },
  /** Subscribe to daemon-status transitions. Returns an unsubscribe fn. */
  onDaemonStatus(cb: (s: DaemonStateDto) => void): () => void {
    const h = (_e: unknown, payload: DaemonStateDto) => cb(payload)
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
