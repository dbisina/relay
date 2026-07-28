// src/main/index.ts
//
// Main process. Owns the frameless window, the daemon lifecycle, and the ONLY
// bridge the renderer has to the outside world. The renderer is sandboxed
// (contextIsolation on, nodeIntegration off) and can reach nothing directly:
// every API call, the WebSocket stream, folder picking, and opening external
// URLs are proxied through the typed IPC handlers below. That keeps a strict
// renderer CSP possible while still talking to the loopback daemon.

import { app, BrowserWindow, ipcMain, shell, dialog, Tray, Menu, nativeImage } from 'electron'
import { join } from 'path'
import { existsSync, statSync } from 'fs'
import { userInfo } from 'os'
import WebSocket from 'ws'
import {
  DAEMON_BASE,
  DAEMON_WS,
  DaemonSupervisor,
  daemonHealthy,
  daemonLogPath,
  type DaemonState,
  type DaemonStatus,
} from './daemon'

let win: BrowserWindow | null = null
let tray: Tray | null = null
let ws: WebSocket | null = null
let wsRetry: NodeJS.Timeout | null = null
let isQuitting = false
let lastDaemonStatus: DaemonStatus = 'checking'

// One supervisor for the app's lifetime. It pushes every state change to the
// renderer and to the tray menu, so both always show the truth.
const daemon = new DaemonSupervisor((s: DaemonState) => {
  lastDaemonStatus = s.status
  win?.webContents.send('relay:daemonStatus', s)
  tray?.setContextMenu(buildTrayMenu())
  if (s.status === 'up') connectWs()
})

/** Resolve an icon file across dev and packaged layouts. Tries each name in order. */
function resolveIcon(...names: string[]): string | undefined {
  const roots = [
    process.resourcesPath || '', // packaged (extraResources)
    join(__dirname, '../../resources'), // dev (out/main -> package/resources)
  ]
  for (const name of names) {
    for (const root of roots) {
      const p = join(root, name)
      if (existsSync(p)) return p
    }
  }
  return undefined
}
const windowIcon = () => resolveIcon('icon.png', 'icon.ico')
const trayIcon = () => resolveIcon('tray.png', 'icon.png')

/**
 * The folder passed by the "Open in Relay" shell entry, if any.
 *
 * The installer registers a command of the form `Relay.exe "%V"`, so the folder
 * arrives as a plain argv entry. Electron adds its own switches, and an unpacked
 * dev run also carries the script path, so this takes the last argument that is
 * an existing directory rather than trusting a fixed index.
 */
function folderFromArgv(argv: string[]): string | null {
  for (let i = argv.length - 1; i >= 1; i--) {
    const a = argv[i]
    if (!a || a.startsWith('-')) continue
    try {
      if (existsSync(a) && statSync(a).isDirectory()) return a
    } catch {
      /* not a path we can read, keep looking */
    }
  }
  return null
}

/** Folder the user most recently opened Relay on. */
let openedFolder: string | null = null

function setOpenedFolder(dir: string | null): void {
  if (!dir) return
  openedFolder = dir
  win?.webContents.send('relay:openedFolder', dir)
}

function createWindow(): void {
  win = new BrowserWindow({
    width: 1240,
    height: 800,
    minWidth: 900,
    minHeight: 600,
    show: false,
    frame: false, // custom titlebar in the renderer (matches the egui build)
    titleBarStyle: 'hidden',
    icon: windowIcon(),
    backgroundColor: '#0a0908',
    trafficLightPosition: { x: 14, y: 16 }, // macOS: inset the stoplights into our bar
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false, // preload needs `require`; renderer is still isolated
    },
  })

  win.once('ready-to-show', () => {
    win?.show()
    // Replay the launch folder now that the renderer exists to hear it.
    if (openedFolder) win?.webContents.send('relay:openedFolder', openedFolder)
  })

  // Never let the renderer navigate away or spawn windows to arbitrary URLs.
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith('http')) shell.openExternal(url)
    return { action: 'deny' }
  })
  win.webContents.on('will-navigate', (e, url) => {
    if (!url.startsWith('file://') && !url.startsWith('http://localhost')) e.preventDefault()
  })

  win.on('closed', () => (win = null))

  // With a tray icon present, closing the window hides it (like Slack/Discord)
  // rather than quitting — the daemon and its work keep running in the tray.
  // Tray's own "Quit Relay" sets isQuitting first so this lets the real close through.
  win.on('close', (e) => {
    if (!isQuitting && tray) {
      e.preventDefault()
      win?.hide()
    }
  })

  if (process.env.ELECTRON_RENDERER_URL) {
    win.loadURL(process.env.ELECTRON_RENDERER_URL)
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

// ── WebSocket relay: connect to the daemon, forward frames to the renderer ────
function connectWs(): void {
  if (wsRetry) clearTimeout(wsRetry)
  try {
    ws = new WebSocket(DAEMON_WS)
  } catch {
    scheduleWsRetry()
    return
  }
  ws.on('open', () => win?.webContents.send('relay:ws', { type: '_open' }))
  ws.on('message', (buf) => {
    let data: unknown
    try {
      data = JSON.parse(buf.toString())
    } catch {
      data = { type: '_raw', raw: buf.toString() }
    }
    win?.webContents.send('relay:ws', data)
  })
  ws.on('close', () => {
    win?.webContents.send('relay:ws', { type: '_close' })
    scheduleWsRetry()
  })
  ws.on('error', () => ws?.close())
}
function scheduleWsRetry(): void {
  if (wsRetry) clearTimeout(wsRetry)
  wsRetry = setTimeout(connectWs, 2500)
}

// ── System tray ────────────────────────────────────────────────────────────
const statusLabel: Record<DaemonStatus, string> = {
  checking: 'Connecting…',
  starting: 'Starting daemon…',
  restarting: 'Restarting daemon…',
  up: 'Daemon connected',
  failed: 'Daemon offline',
}

function buildTrayMenu(): Menu {
  return Menu.buildFromTemplate([
    { label: 'Open Relay', click: () => showWindow() },
    { type: 'separator' },
    { label: statusLabel[lastDaemonStatus], enabled: false },
    { label: 'Reconnect daemon', click: () => void daemon.ensure() },
    { type: 'separator' },
    {
      label: 'Quit Relay',
      click: () => {
        isQuitting = true
        app.quit()
      },
    },
  ])
}

function showWindow(): void {
  if (!win) {
    createWindow()
    return
  }
  if (win.isMinimized()) win.restore()
  win.show()
  win.focus()
}

function createTray(): void {
  if (tray) return // never stack a second icon on the same process
  const iconPath = trayIcon()
  const image = iconPath ? nativeImage.createFromPath(iconPath) : nativeImage.createEmpty()
  tray = new Tray(image.isEmpty() ? image : image.resize({ width: 20, height: 20 }))
  tray.setToolTip('Relay')
  tray.setContextMenu(buildTrayMenu())
  tray.on('click', () => showWindow())
}

// ── Typed IPC surface (mirrored in preload/index.ts) ─────────────────────────
function registerIpc(): void {
  // Generic daemon HTTP proxy. Only the daemon origin is reachable, and only
  // /api/* paths — the renderer can't point this at anything else.
  ipcMain.handle('relay:request', async (_e, arg: { method: string; path: string; body?: unknown }) => {
    const { method, path, body } = arg
    if (!path.startsWith('/api/')) return { ok: false, status: 0, error: 'blocked path' }
    try {
      const res = await fetch(`${DAEMON_BASE}${path}`, {
        method,
        headers: body != null ? { 'content-type': 'application/json' } : undefined,
        body: body != null ? JSON.stringify(body) : undefined,
      })
      const text = await res.text()
      let data: unknown = null
      try {
        data = text ? JSON.parse(text) : null
      } catch {
        data = text
      }
      return { ok: res.ok, status: res.status, data }
    } catch (e) {
      return { ok: false, status: 0, error: (e as Error).message }
    }
  })

  ipcMain.handle('relay:health', () => daemonHealthy())

  ipcMain.handle('relay:ensureDaemon', async () => {
    const s = await daemon.ensure()
    return s.status
  })

  /** Full daemon state, for the connection screen's diagnostics. */
  ipcMain.handle('relay:daemonState', () => daemon.current())

  ipcMain.handle('relay:openedFolder', () => openedFolder)

  ipcMain.handle('relay:openLogFolder', () => {
    shell.showItemInFolder(daemonLogPath())
  })

  ipcMain.handle('relay:whoami', () => {
    try {
      return userInfo().username
    } catch {
      return ''
    }
  })

  ipcMain.handle('relay:pickFolder', async () => {
    if (!win) return null
    const r = await dialog.showOpenDialog(win, {
      title: 'Choose project folder for Relay',
      properties: ['openDirectory'],
    })
    return r.canceled || r.filePaths.length === 0 ? null : r.filePaths[0]
  })

  ipcMain.handle('relay:openExternal', (_e, url: string) => {
    if (typeof url === 'string' && /^https?:\/\//.test(url)) shell.openExternal(url)
  })

  ipcMain.on('win:minimize', () => win?.minimize())
  ipcMain.on('win:maximizeToggle', () => (win?.isMaximized() ? win.unmaximize() : win?.maximize()))
  ipcMain.on('win:close', () => win?.close())
}

// Closing the window only hides it, so relaunching from the shortcut would
// otherwise start a second process, and every process adds its own tray icon.
// Hold a single-instance lock and surface the existing window instead.
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
} else {
  app.on('second-instance', (_e, argv) => {
    setOpenedFolder(folderFromArgv(argv))
    showWindow()
  })

  app.whenReady().then(() => {
    openedFolder = folderFromArgv(process.argv)
    registerIpc()
    createWindow()
    createTray()
    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) createWindow()
      else showWindow()
    })
  })
}

app.on('before-quit', () => {
  isQuitting = true
  // Stop the daemon we started. One we merely attached to is left running, so
  // a daemon the user launched from the CLI survives quitting the app.
  daemon.stop()
  // Windows leaves a ghost icon in the notification area if the tray is not
  // explicitly destroyed before exit.
  tray?.destroy()
  tray = null
})

// With a tray present the app lives there; window-all-closed no longer quits
// it (see the window's own 'close' handler, which hides rather than closes).
// This still matters for the rare case the tray failed to create.
app.on('window-all-closed', () => {
  if (ws) ws.close()
  if (process.platform !== 'darwin' && !tray) app.quit()
})
