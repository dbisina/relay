# Relay Desktop (Electron)

A vendor-neutral cockpit for multi-agent coding handoff. This is the Electron
port of the desktop UI: same job as the egui build in `packages/ui`, rebuilt on
React so the interface can move at the pace of the web (design-system reuse,
faster iteration, one rendering engine across Windows / macOS / Linux).

It renders the daemon; it never owns state. Every screen reads and writes the Go
daemon's `/api/*` surface. If the daemon isn't running, the app starts it
(`relay daemon`, detached) and reconnects.

## Architecture

```
src/
  main/        Electron main process
    index.ts   frameless window, secure IPC bridge, WebSocket relay
    daemon.ts  find the `relay` binary, health-check, start detached
  preload/     the ONLY surface exposed to the renderer (window.relay)
  renderer/    React SPA (Vite)
    src/
      App.tsx           shell: titlebar + sidebar + routed screen + gates
      lib/              api client, TS DTOs (mirror of daemon JSON), store, toast
      components/       design system (tokens, icons, ui primitives, chrome)
      screens/          Dashboard, Accounts, Detect, Workflow, Providers,
                        Pipelines, History, Settings, Onboarding
```

Security posture: `contextIsolation` on, `nodeIntegration` off, a strict
renderer CSP, and a single typed IPC bridge. The renderer cannot reach the
network directly; every daemon call, the event WebSocket, folder-picking, and
opening external links go through enumerated handlers in `main/index.ts`.

The daemon runs on `127.0.0.1:4748`. See `packages/daemon-go` for the API.

## Accounts, safely

The Accounts screen manages several logins per provider so a task can relay from
one to the next when one hits its limit. Relay never creates accounts and never
handles passwords: "Add account" registers an isolated profile slot; "Sign in"
launches the provider's own login (`/api/providers/account/login`), which the
user completes. Each account can point at its own isolated profile folder so
multiple logins on one provider don't clobber each other.

## Develop

```bash
npm install
npm run dev        # electron-vite: HMR renderer + live main/preload
```

The dev build expects the `relay` binary on your PATH (or beside the app) so it
can start the daemon. Build the daemon from `packages/daemon-go` first.

## Build & package

```bash
npm run typecheck  # tsc for main+preload and renderer
npm run build      # bundles to ./out
npm run package    # electron-builder: NSIS / dmg / AppImage into ./release
```

To ship the daemon inside the app, uncomment `extraResources` in
`electron-builder.yml` and point it at the per-platform `relay` binaries; the
app already looks in `resources/bin` before falling back to PATH.
