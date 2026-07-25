// src/main/daemon.ts
//
// Daemon lifecycle for the desktop app. The Go daemon is the single source of
// truth (CLAUDE.md invariant); this app never owns state, it renders /api/*.
// Our only jobs here: find the `relay` binary, know whether the daemon is up,
// and start it detached if it isn't so it outlives the window.

import { spawn } from 'child_process'
import { existsSync } from 'fs'
import { join, dirname } from 'path'
import { app } from 'electron'

export const DAEMON_HOST = '127.0.0.1'
export const DAEMON_PORT = 4748
export const DAEMON_BASE = `http://${DAEMON_HOST}:${DAEMON_PORT}`
export const DAEMON_WS = `ws://${DAEMON_HOST}:${DAEMON_PORT}/ws`

/**
 * Locate the relay binary. Order:
 *   1. RELAY_BIN env override (explicit path)
 *   2. beside our own executable (portable install)
 *   3. bundled in the packaged app (resources/bin)
 *   4. dev monorepo locations (so a checked-out repo just works)
 *   5. bare name on PATH
 */
export function findRelayBinary(): string {
  const name = process.platform === 'win32' ? 'relay.exe' : 'relay'

  const env = process.env.RELAY_BIN
  if (env && existsSync(env)) return env

  const candidates = [
    join(dirname(app.getPath('exe')), name), // beside the app
    join(process.resourcesPath || '', 'bin', name), // packaged bundle
  ]

  // In a dev checkout the daemon is built into packages/daemon-go. cwd is the
  // desktop package when run via npm; also probe from the app path for safety.
  if (!app.isPackaged) {
    const roots = [process.cwd(), app.getAppPath()]
    for (const r of roots) {
      candidates.push(
        join(r, '..', 'daemon-go', name),
        join(r, '..', '..', 'packages', 'daemon-go', name),
      )
    }
  }

  const found = candidates.find((p) => p && existsSync(p))
  return found || name // fall back to PATH
}

/** True if a daemon is already answering on the local API. */
export async function daemonHealthy(timeoutMs = 700): Promise<boolean> {
  const ctl = new AbortController()
  const t = setTimeout(() => ctl.abort(), timeoutMs)
  try {
    const res = await fetch(`${DAEMON_BASE}/api/health`, { signal: ctl.signal })
    return res.ok
  } catch {
    return false
  } finally {
    clearTimeout(t)
  }
}

/** Spawn `relay daemon` detached so it survives the window closing. */
export function spawnDaemonDetached(): { ok: boolean; error?: string } {
  const relay = findRelayBinary()
  try {
    const child = spawn(relay, ['daemon'], {
      detached: true,
      stdio: 'ignore',
      windowsHide: true,
    })
    child.on('error', () => {
      /* surfaced by the health poll below, not here */
    })
    child.unref()
    return { ok: true }
  } catch (e) {
    return { ok: false, error: (e as Error).message }
  }
}

export type DaemonStatus = 'checking' | 'up' | 'starting' | 'unreachable'

/**
 * Ensure the daemon is reachable: if it's already up, done; otherwise start it
 * and poll health for a few seconds. Reports each transition via `onStatus` so
 * the renderer can show an honest connecting state instead of a dead screen.
 */
export async function ensureDaemon(
  onStatus: (s: DaemonStatus, detail?: string) => void,
): Promise<DaemonStatus> {
  onStatus('checking')
  if (await daemonHealthy()) {
    onStatus('up')
    return 'up'
  }
  onStatus('starting')
  const spawned = spawnDaemonDetached()
  if (!spawned.ok) {
    onStatus('unreachable', spawned.error)
    return 'unreachable'
  }
  for (let i = 0; i < 20; i++) {
    await new Promise((r) => setTimeout(r, 400))
    if (await daemonHealthy()) {
      onStatus('up')
      return 'up'
    }
  }
  onStatus('unreachable', 'daemon did not answer after start')
  return 'unreachable'
}
