// src/main/daemon.ts
//
// Daemon lifecycle for the desktop app. The Go daemon is the single source of
// truth (CLAUDE.md invariant); this app never owns state, it renders /api/*.
// Our only jobs here: find the `relay` binary, know whether the daemon is up,
// and start it detached if it isn't so it outlives the window.

import { spawn } from 'child_process'
import { existsSync, mkdirSync, openSync } from 'fs'
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

/**
 * Where the daemon runs. This matters more than it looks: `relay daemon` does
 * os.Getwd() and then MkdirAll(.relay) inside it. An installed app inherits a
 * cwd it does not control (on Windows, launching from a shortcut often means
 * C:\Windows\System32), so leaving cwd unset made the daemon fail to create its
 * state dir and exit before binding the port. Pin it to a directory we know is
 * writable and app-scoped.
 */
export function daemonWorkDir(): string {
  const dir = join(app.getPath('userData'), 'daemon')
  mkdirSync(dir, { recursive: true })
  return dir
}

/** Where the daemon's own stdout/stderr goes, so failures are diagnosable. */
export function daemonLogPath(): string {
  return join(app.getPath('userData'), 'daemon.log')
}

/** Spawn `relay daemon` detached so it survives the window closing. */
export function spawnDaemonDetached(): { ok: boolean; error?: string } {
  const relay = findRelayBinary()
  try {
    const cwd = daemonWorkDir()
    // Never discard the daemon's output: when it dies on startup this file is
    // the only evidence of why.
    const log = openSync(daemonLogPath(), 'a')
    const child = spawn(relay, ['daemon'], {
      cwd,
      detached: true,
      stdio: ['ignore', log, log],
      windowsHide: true,
    })
    let spawnError: string | undefined
    child.on('error', (e) => {
      spawnError = e.message
    })
    child.unref()
    // spawn() reports ENOENT asynchronously, so a missing binary shows up on
    // the next tick rather than as a throw. The health poll is the real check;
    // this only makes the common failure legible.
    if (spawnError) return { ok: false, error: spawnError }
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
    onStatus('unreachable', `could not start ${findRelayBinary()}: ${spawned.error}`)
    return 'unreachable'
  }
  for (let i = 0; i < 20; i++) {
    await new Promise((r) => setTimeout(r, 400))
    if (await daemonHealthy()) {
      onStatus('up')
      return 'up'
    }
  }
  // Started but never answered. The daemon's own log is the only thing that
  // explains why, so point at it rather than leaving a dead end.
  onStatus(
    'unreachable',
    `started ${findRelayBinary()} but it never answered on port ${DAEMON_PORT}. See ${daemonLogPath()}`,
  )
  return 'unreachable'
}
