// ensureBridge.ts — the renderer talks to the outside world only through
// window.relay (installed by the preload). In the real app it's always present.
// If it's ever missing (e.g. the renderer opened in a bare browser for design
// preview), install a safe no-op stub so the UI degrades to an honest
// "daemon offline" state instead of throwing on first render.

import type { RelayApi } from '../../../preload'

if (typeof window !== 'undefined' && !window.relay) {
  const noop = () => {}
  const stub: RelayApi = {
    get: async () => ({ ok: false, status: 0, error: 'no bridge' }),
    post: async () => ({ ok: false, status: 0, error: 'no bridge' }),
    health: async () => false,
    ensureDaemon: async () => 'failed',
    daemonState: async () => ({
      status: 'failed' as const,
      logTail: [],
      external: false,
      binaryPath: '',
      binaryFound: false,
      triedPaths: [],
      logPath: '',
      workDir: '',
    }),
    openLogFolder: noop,
    openedFolder: async () => null,
    onOpenedFolder: () => noop,
    onDaemonStatus: () => noop,
    onEvent: () => noop,
    pickFolder: async () => null,
    whoami: async () => '',
    openExternal: noop,
    win: { minimize: noop, maximizeToggle: noop, close: noop },
  }
  ;(window as unknown as { relay: RelayApi }).relay = stub
}
