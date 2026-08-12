// api.ts — typed client over the preload bridge (window.relay). Every call is a
// thin, named wrapper around a daemon endpoint so screens never hand-write paths.
//
// Every read that returns an array/object DTO the daemon can send with omitted
// fields (Go `omitempty`) is passed through the matching normalizer in
// normalize.ts before it reaches a screen. See that file for why.

import type {
  ProviderStatus,
  ProviderDetail,
  SessionInfo,
  Profile,
  InstructionsState,
  WalletEntry,
  HistoryItem,
  CommitItem,
  DetectedAgent,
  Pipeline,
} from './types'
import { normalizeProviderDetail, normalizeProfile, normalizePipeline, normalizeDetectedAgent } from './normalize'

/** Response body of POST /api/detect/adopt. Mirrors the map cmd/relay/detect.go's adoptDetectedSince returns. */
export interface AdoptResult {
  id: string
  target: string
  path: string
  markdown: string
  contractPath: string
  workDir: string
  started?: boolean
  startError?: string
  /** Interactive mode: whether the provider's CLI was opened in a terminal. */
  opened?: boolean
  openError?: string
}

async function get<T>(path: string, fallback: T): Promise<T> {
  const r = await window.relay.get<T>(path)
  if (r.ok && r.data != null) return r.data
  return fallback
}

async function post(path: string, body?: unknown): Promise<{ ok: boolean; error?: string }> {
  const r = await window.relay.post<{ ok?: boolean; error?: string }>(path, body)
  if (!r.ok) return { ok: false, error: r.error || `request failed (${r.status})` }
  const err = r.data && typeof r.data === 'object' ? (r.data as { error?: string }).error : undefined
  if (err) return { ok: false, error: err }
  return { ok: true }
}

/** Like post(), but keeps the response body: for a POST whose caller needs to read something back (adopt's rendered brief), not just ok/error. */
async function postData<T>(path: string, body?: unknown): Promise<{ ok: boolean; data?: T; error?: string }> {
  const r = await window.relay.post<T>(path, body)
  if (!r.ok) return { ok: false, error: r.error || `request failed (${r.status})` }
  return { ok: true, data: r.data }
}

export const api = {
  health: () => window.relay.health(),
  ensureDaemon: () => window.relay.ensureDaemon(),

  // Reads
  status: () => get<SessionInfo | null>('/api/status', null),
  providers: () => get<ProviderStatus[]>('/api/providers', []),
  providerDetails: async () => (await get<Partial<ProviderDetail>[]>('/api/config/providers', [])).map(normalizeProviderDetail),
  profiles: async () => (await get<Partial<Profile>[]>('/api/profiles', [])).map(normalizeProfile),
  instructions: () => get<InstructionsState>('/api/instructions', { sources: [], skills: [] }),
  wallet: () => get<WalletEntry[]>('/api/quota/wallet', []),
  history: () => get<HistoryItem[]>('/api/history', []),
  commits: () => get<CommitItem[]>('/api/history/commits', []),
  detect: async (sinceHours = 24) =>
    (await get<Partial<DetectedAgent>[]>(`/api/detect?sinceHours=${sinceHours}`, [])).map(normalizeDetectedAgent),
  pipelines: async () => (await get<Partial<Pipeline>[]>('/api/pipelines', [])).map(normalizePipeline),

  // Contract / graph / retrieval (read-only inspection surface)
  contract: () => get<Record<string, unknown> | null>('/api/contract', null),
  graph: () => get<Record<string, unknown> | null>('/api/graph', null),
  approvals: () => get<Array<Record<string, unknown>>>('/api/approvals', []),
  circuit: () => get<Record<string, unknown> | null>('/api/circuit', null),
  outcomes: () => get<Record<string, unknown> | null>('/api/outcomes', null),
  cost: () => get<Record<string, unknown> | null>('/api/session/cost', null),
  diff: () => get<Record<string, unknown> | null>('/api/session/diff', null),
  visionConfig: () => get<Record<string, unknown> | null>('/api/vision/config', null),
  ollamaModels: () => get<Record<string, unknown> | null>('/api/ollama/models', null),

  // Session control
  run: (task: string, opts?: { threshold?: number; maxHandoffs?: number; providers?: string[] }) =>
    post('/api/run', {
      task,
      threshold: opts?.threshold ?? 0.85,
      maxHandoffs: opts?.maxHandoffs ?? 0,
      providers: opts?.providers ?? [],
    }),
  handoff: () => post('/api/handoff'),
  pause: () => post('/api/session/pause'),

  // Accounts (user-in-the-loop: `login` launches the provider's own auth)
  addAccount: (provider: string, label: string, configDir = '') =>
    post('/api/providers/account/add', { provider, label, configDir }),
  loginAccount: (provider: string, label: string) =>
    post('/api/providers/account/login', { provider, label }),
  switchAccount: (provider: string, label: string) =>
    post('/api/providers/account', { provider, label }),
  removeAccount: (provider: string, label: string) =>
    post('/api/providers/account/remove', { provider, label }),

  // Provider auth / install
  installProvider: (name: string) => post('/api/providers/install', { name }),
  oauthProvider: (name: string) => post('/api/providers/oauth', { name }),
  apiKeyProvider: (name: string, value: string) =>
    post('/api/providers/api-key', { name, value }),
  saveProviderConfig: (body: unknown) => post('/api/config/providers', body),

  // Profiles (agent + skill routing)
  saveProfile: (profile: Profile) => post('/api/profiles', profile),

  // Detect / adopt
  adopt: (id: string, target: string, start: boolean, interactive = false) =>
    postData<AdoptResult>('/api/detect/adopt', { id, target, start, interactive }),

  /**
   * Hand a detected session to another provider, optionally switching which
   * account of that provider is active first. Account switching is a separate
   * daemon call, so this sequences them and fails loudly on the first error
   * rather than adopting into the wrong account.
   *
   * Two continue modes, mutually exclusive:
   *   - interactive: open the provider's own CLI in a terminal, in the adopted
   *     project, seeded with the brief, and let the developer drive it.
   *   - start (headless): Relay runs the provider itself and streams events.
   * The response body carries `opened`/`started` so the caller can report which
   * actually happened.
   */
  handOffSession: async (
    id: string,
    target: string,
    opts?: { account?: string; start?: boolean; interactive?: boolean },
  ): Promise<{ ok: boolean; error?: string; opened?: boolean; started?: boolean }> => {
    if (opts?.account && target) {
      const sw = await post('/api/providers/account', { provider: target, label: opts.account })
      if (!sw.ok) return { ok: false, error: `could not switch account: ${sw.error}` }
    }
    const interactive = opts?.interactive ?? false
    // In interactive mode the terminal launch is the action, so `start` is off.
    const r = await postData<AdoptResult>('/api/detect/adopt', {
      id,
      target,
      start: interactive ? false : opts?.start ?? true,
      interactive,
    })
    if (!r.ok) return { ok: false, error: r.error }
    // The daemon returns HTTP 200 with a body-level {error} when adoption itself
    // fails (e.g. the agent exited between scan and click), so it must be checked
    // explicitly — postData, unlike post, does not. Without this a real failure
    // would fall through to { ok: true } and the panel would claim success.
    const bodyErr = (r.data as unknown as { error?: string })?.error
    if (bodyErr) return { ok: false, error: bodyErr }
    if (interactive && r.data?.opened === false) {
      return { ok: false, error: r.data.openError || 'could not open the provider CLI' }
    }
    if (!interactive && r.data?.started === false) {
      return { ok: false, error: r.data.startError || 'could not start the session' }
    }
    return { ok: true, opened: r.data?.opened, started: r.data?.started }
  },

  // Pipelines
  savePipeline: (pipeline: Pipeline) => post('/api/pipelines', pipeline),
  runPipeline: (name: string) => post('/api/pipelines/run', { name }),

  // Raw escape hatch — the Console screen's "full API access" surface. Bypasses
  // every typed wrapper above; still constrained to /api/* by the preload bridge.
  raw: {
    get: (path: string) => window.relay.get(path),
    post: (path: string, body?: unknown) => window.relay.post(path, body),
  },
}

/** Every daemon route, for the Console screen's path picker. Source: server.go. */
export const KNOWN_ENDPOINTS: { method: 'GET' | 'POST'; path: string; note?: string }[] = [
  { method: 'GET', path: '/api/health' },
  { method: 'GET', path: '/api/status' },
  { method: 'GET', path: '/api/providers' },
  { method: 'GET', path: '/api/config/providers' },
  { method: 'POST', path: '/api/config/providers', note: '{ name, enabled, declaredCap?, model?, baseUrl? }' },
  { method: 'POST', path: '/api/providers/install', note: '{ name }' },
  { method: 'POST', path: '/api/providers/oauth', note: '{ name }' },
  { method: 'POST', path: '/api/providers/api-key', note: '{ name, value }' },
  { method: 'GET', path: '/api/profiles' },
  { method: 'POST', path: '/api/profiles', note: '{ name, chain, kinds, skills, contextHint }' },
  { method: 'GET', path: '/api/detect?sinceHours=24' },
  { method: 'POST', path: '/api/detect/adopt', note: '{ id, target, start, interactive }' },
  { method: 'POST', path: '/api/providers/account', note: '{ provider, label }' },
  { method: 'POST', path: '/api/providers/account/add', note: '{ provider, label, configDir }' },
  { method: 'POST', path: '/api/providers/account/remove', note: '{ provider, label }' },
  { method: 'POST', path: '/api/providers/account/login', note: '{ provider, label }' },
  { method: 'GET', path: '/api/pipelines' },
  { method: 'POST', path: '/api/pipelines', note: '{ name, nodes: [...] }' },
  { method: 'POST', path: '/api/pipelines/run', note: '{ name }' },
  { method: 'GET', path: '/api/quota/wallet' },
  { method: 'GET', path: '/api/history' },
  { method: 'GET', path: '/api/history/commits' },
  { method: 'GET', path: '/api/history/diff?sha=' },
  { method: 'POST', path: '/api/history/rewind', note: '{ sha }' },
  { method: 'GET', path: '/api/instructions' },
  { method: 'GET', path: '/api/contract' },
  { method: 'GET', path: '/api/graph' },
  { method: 'GET', path: '/api/graph/detail' },
  { method: 'GET', path: '/api/graph/project' },
  { method: 'GET', path: '/api/retrieval' },
  { method: 'POST', path: '/api/run', note: '{ task, threshold, maxHandoffs, providers }' },
  { method: 'POST', path: '/api/handoff' },
  { method: 'GET', path: '/api/vision/config' },
  { method: 'GET', path: '/api/vision/probe' },
  { method: 'POST', path: '/api/session/reply', note: '{ text } (answers a vision prompt)' },
  { method: 'GET', path: '/api/session/diff' },
  { method: 'GET', path: '/api/session/cost' },
  { method: 'POST', path: '/api/session/pause' },
  { method: 'GET', path: '/api/approvals' },
  { method: 'GET', path: '/api/circuit' },
  { method: 'GET', path: '/api/outcomes' },
  { method: 'GET', path: '/api/redactions' },
  { method: 'GET', path: '/api/ollama/models' },
  { method: 'POST', path: '/api/ollama/pull', note: '{ name }' },
  { method: 'POST', path: '/api/ollama/launch', note: '{ provider, model }' },
]
