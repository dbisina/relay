// src/main/daemon.ts
//
// Daemon lifecycle. The Go daemon is the single source of truth (CLAUDE.md
// invariant); this app never owns state. What it does own is making sure a
// daemon is actually running, which on a fresh install is harder than it looks:
//
//  - The binary is unsigned, so Windows Defender scans it on first execution.
//    That can take far longer than a naive few-second health check allows, and
//    the old code gave up after 8s and never tried again.
//  - `relay daemon` does os.Getwd() then MkdirAll(.relay), so it must be given
//    a writable cwd rather than inheriting the launcher's (often System32).
//  - When it fails it fails on stderr, which the old code discarded.
//
// So the daemon is supervised rather than fire-and-forget: spawn it, keep its
// output, wait patiently, restart it if it dies, and always be able to say why.

import { spawn, execFile, type ChildProcess } from 'child_process'
import { existsSync, mkdirSync, appendFileSync } from 'fs'
import { join, dirname } from 'path'
import { app } from 'electron'

export const DAEMON_HOST = '127.0.0.1'
export const DAEMON_PORT = 4748
export const DAEMON_BASE = `http://${DAEMON_HOST}:${DAEMON_PORT}`
export const DAEMON_WS = `ws://${DAEMON_HOST}:${DAEMON_PORT}/ws`

/** How long to wait for a first start before calling it failed. Generous on
 *  purpose: an unsigned binary being virus-scanned can take a long time. */
const FIRST_START_TIMEOUT_MS = 90_000
const POLL_INTERVAL_MS = 500
/** Restarts allowed inside RESTART_WINDOW_MS before we stop trying. */
const MAX_RESTARTS = 3
const RESTART_WINDOW_MS = 60_000
const LOG_TAIL_LINES = 40

export type DaemonStatus = 'checking' | 'starting' | 'up' | 'restarting' | 'failed'

export interface DaemonState {
  status: DaemonStatus
  /** Human-readable explanation, present on failure. */
  detail?: string
  /** Last lines the daemon itself printed. The real diagnostic. */
  logTail: string[]
  /** True when something else already served the port and we just attached. */
  external: boolean
  binaryPath: string
  /** False when the bundled daemon is missing (quarantined, partial install). */
  binaryFound: boolean
  /** Every path checked, so a user can see why we came up empty. */
  triedPaths: string[]
  /**
   * Result of running the binary with --version before trusting it to serve.
   * Separates "this binary cannot run on this machine" from "the daemon
   * subcommand fails", which look identical from the outside.
   */
  probe?: string
  logPath: string
  workDir: string
}

/**
 * Locate the relay binary. Order:
 *   1. RELAY_BIN env override (explicit path)
 *   2. beside our own executable (portable install)
 *   3. bundled in the packaged app (resources/bin)
 *   4. dev monorepo locations (so a checked-out repo just works)
 *   5. bare name on PATH
 */
export interface BinaryResolution {
  /** What we will actually exec. May be a bare name if nothing was found. */
  path: string
  /** True when we located a real file rather than falling back to PATH. */
  found: boolean
  /** Every absolute location checked, in order, for diagnostics. */
  tried: string[]
}

/**
 * Locate the relay binary, reporting what was tried. The bundled copy going
 * missing (antivirus quarantine of an unsigned binary is the usual reason) is
 * indistinguishable from a broken install unless we can say exactly where we
 * looked, so this returns the search path rather than just an answer.
 */
export function resolveRelayBinary(): BinaryResolution {
  const name = process.platform === 'win32' ? 'relay.exe' : 'relay'
  const tried: string[] = []

  const env = process.env.RELAY_BIN
  if (env) {
    tried.push(`${env}  (RELAY_BIN)`)
    if (existsSync(env)) return { path: env, found: true, tried }
  }

  // On Windows this must be checked first: the app's own exe sits beside
  // itself, and `relay.exe` vs `Relay.exe` is the SAME FILE case-insensitively.
  // Checking "beside the app" before "resources/bin" made the resolver
  // confirm the Electron app itself as "the daemon", which then got spawned
  // as a second app instance, hit the single-instance lock, and exited 0
  // with no output. Looked identical to the daemon dying silently.
  const ownExe = app.getPath('exe')
  const candidates = [
    join(process.resourcesPath || '', 'bin', name), // packaged bundle
    join(dirname(ownExe), name), // portable install: daemon beside the app
  ]

  if (!app.isPackaged) {
    const roots = [process.cwd(), app.getAppPath()]
    for (const r of roots) {
      candidates.push(
        join(r, '..', 'daemon-go', name),
        join(r, '..', '..', 'packages', 'daemon-go', name),
      )
    }
  }

  const samePath = (a: string, b: string): boolean =>
    process.platform === 'win32' ? a.toLowerCase() === b.toLowerCase() : a === b

  for (const c of candidates) {
    if (!c) continue
    // Never trust a candidate that resolves to our own executable.
    if (samePath(c, ownExe)) continue
    tried.push(c)
    if (existsSync(c)) return { path: c, found: true, tried }
  }

  tried.push(`${name}  (via PATH)`)
  return { path: name, found: false, tried }
}

/** Convenience wrapper for callers that only need the path. */
export function findRelayBinary(): string {
  return resolveRelayBinary().path
}

/**
 * Where the daemon runs. `relay daemon` creates .relay/ in its cwd, so this
 * must be writable. An installed app inherits a cwd it does not control.
 */
export function daemonWorkDir(): string {
  const dir = join(app.getPath('userData'), 'daemon')
  mkdirSync(dir, { recursive: true })
  return dir
}

export function daemonLogPath(): string {
  return join(app.getPath('userData'), 'daemon.log')
}

/** True if something is already answering the local API. */
export async function daemonHealthy(timeoutMs = 1500): Promise<boolean> {
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

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

export class DaemonSupervisor {
  private child: ChildProcess | null = null
  private logTail: string[] = []
  private restarts: number[] = []
  private stopping = false
  private starting = false
  private state: DaemonState

  constructor(private onChange: (s: DaemonState) => void) {
    const bin = resolveRelayBinary()
    this.state = {
      status: 'checking',
      logTail: [],
      external: false,
      binaryPath: bin.path,
      binaryFound: bin.found,
      triedPaths: bin.tried,
      logPath: daemonLogPath(),
      workDir: '',
    }
  }

  current(): DaemonState {
    return { ...this.state, logTail: [...this.logTail] }
  }

  private set(status: DaemonStatus, detail?: string): void {
    this.state = { ...this.state, status, detail }
    this.onChange(this.current())
  }

  private record(chunk: string): void {
    for (const line of chunk.split(/\r?\n/)) {
      const t = line.trim()
      if (!t) continue
      this.logTail.push(t)
      if (this.logTail.length > LOG_TAIL_LINES) this.logTail.shift()
    }
    try {
      appendFileSync(this.state.logPath, chunk)
    } catch {
      /* logging must never take the app down */
    }
  }

  /**
   * Run the binary with --version. It prints and exits immediately by design,
   * so it answers one question nothing else can: can this executable run here
   * at all? A daemon that dies silently and a binary the machine refuses to
   * execute produce the same symptom without this.
   */
  private probeBinary(binary: string): Promise<string> {
    return new Promise((resolve) => {
      execFile(binary, ['--version'], { timeout: 15_000, windowsHide: true }, (err, stdout, stderr) => {
        const out = `${stdout || ''}${stderr || ''}`.trim()
        if (err) {
          resolve(`--version failed: ${err.message}${out ? ` | output: ${out}` : ' | no output'}`)
        } else {
          resolve(out ? `--version says: ${out}` : '--version produced no output, which is wrong for this binary')
        }
      })
    })
  }

  /** Ensure a daemon is reachable, starting and supervising one if needed. */
  async ensure(): Promise<DaemonState> {
    if (this.starting) return this.current()
    this.starting = true
    try {
      this.set('checking')

      // Something already serving? Attach rather than fighting over the port.
      if (await daemonHealthy()) {
        this.state.external = this.child === null
        this.set('up')
        return this.current()
      }

      return await this.spawnAndWait('starting')
    } finally {
      this.starting = false
    }
  }

  private async spawnAndWait(phase: DaemonStatus): Promise<DaemonState> {
    const bin = resolveRelayBinary()
    const binary = bin.path
    const workDir = daemonWorkDir()
    this.state = {
      ...this.state,
      binaryPath: binary,
      binaryFound: bin.found,
      triedPaths: bin.tried,
      workDir,
      external: false,
    }

    // Nothing to run. Fail loudly here rather than letting spawn() report an
    // async ENOENT that leaves an empty log and no explanation.
    if (!bin.found) {
      this.set(
        'failed',
        'The Relay daemon could not be found. It ships inside this app, so it has most likely been quarantined by antivirus or the install is incomplete. Reinstall, or allow the file and retry.',
      )
      return this.current()
    }

    // Only on a first start: on a restart we already know the binary runs.
    if (phase === 'starting' && !this.state.probe) {
      this.state = { ...this.state, probe: await this.probeBinary(binary) }
    }

    this.set(phase)

    try {
      this.child = spawn(binary, ['daemon'], {
        cwd: workDir,
        stdio: ['ignore', 'pipe', 'pipe'],
        windowsHide: true,
        // Give the daemon its own process group. Relay's window is a GUI
        // process with no console; without this the console child stays in the
        // app's group and receives the console control events the shell
        // broadcasts (CTRL_C, and the CTRL_CLOSE/LOGOFF on session changes).
        // Its signal.NotifyContext for os.Interrupt then fires at once and it
        // exits 0 having printed almost nothing, which is exactly what a clean
        // install reported. The same binary run from a terminal, which has its
        // own console, stays up fine. We keep the pipes and do not unref, so it
        // is still supervised and its output still captured.
        detached: process.platform === 'win32',
      })
    } catch (e) {
      this.set('failed', `could not launch ${binary}: ${(e as Error).message}`)
      return this.current()
    }

    this.child.stdout?.on('data', (d) => this.record(String(d)))
    this.child.stderr?.on('data', (d) => this.record(String(d)))

    this.child.on('error', (e) => {
      this.record(`[spawn error] ${e.message}\n`)
      this.set('failed', `could not launch ${binary}: ${e.message}`)
    })

    // 'close' rather than 'exit': exit can fire before the stdout and stderr
    // pipes have drained, which would drop the daemon's own last words, the one
    // thing worth having when it dies during startup.
    this.child.on('close', (code, signal) => {
      this.child = null
      if (this.stopping) return
      this.record(`[daemon exited code=${code} signal=${signal ?? 'none'}]\n`)
      if (code === 0 && !signal) {
        // A clean exit having printed nothing is its own diagnosis: the daemon
        // prints its listening address before it blocks, so getting to a
        // successful exit silently means it stopped before it began serving.
        this.record('[it exited successfully without printing anything]\n')
      }
      void this.handleUnexpectedExit()
    })

    // Wait for it to answer. Long window: first run of an unsigned binary can
    // sit in a virus scan for a while before it ever executes.
    const deadline = Date.now() + FIRST_START_TIMEOUT_MS
    while (Date.now() < deadline) {
      await sleep(POLL_INTERVAL_MS)
      if (await daemonHealthy()) {
        this.set('up')
        return this.current()
      }
      // Died during startup: no point waiting out the full window.
      if (!this.child && !this.stopping) break
    }

    if (this.state.status !== 'failed') {
      const why = this.logTail.length
        ? 'It exited, see the output below.'
        : `It did not answer on port ${DAEMON_PORT} within ${Math.round(FIRST_START_TIMEOUT_MS / 1000)}s.`
      this.set('failed', `Started ${binary} but could not reach it. ${why}`)
    }
    return this.current()
  }

  /** Restart on an unexpected death, but refuse to loop forever. */
  private async handleUnexpectedExit(): Promise<void> {
    const now = Date.now()
    this.restarts = this.restarts.filter((t) => now - t < RESTART_WINDOW_MS)
    if (this.restarts.length >= MAX_RESTARTS) {
      this.set(
        'failed',
        `The daemon exited ${this.restarts.length} times in under a minute, so it will not be restarted again automatically.`,
      )
      return
    }
    this.restarts.push(now)
    await sleep(1000)
    if (this.stopping) return
    await this.spawnAndWait('restarting')
  }

  /** Kill the daemon we started. A daemon we merely attached to is left alone. */
  stop(): void {
    this.stopping = true
    if (this.child) {
      try {
        this.child.kill()
      } catch {
        /* already gone */
      }
      this.child = null
    }
  }
}
