// types.ts — TS mirror of the daemon DTOs (packages/ui/src/types.rs).
// The daemon emits camelCase JSON; these match it field-for-field.

export type ProviderState = 'active' | 'standby' | 'exhausted' | 'error' | 'unknown'

export interface ProviderStatus {
  name: string
  state: ProviderState
  fractionUsed: number
  remaining: number | null
  resetsAt: number | null
  isNext: boolean
  quotaSource: string | null
}

export interface AccountDto {
  label: string
  active: boolean
  configDir: string
  /** Best-effort, verified-provider-only check for whether this account has actually completed sign-in. */
  signedIn: boolean
  /** False when the provider has no verified credential check at all; signedIn must not be read as "not signed in" when this is false. */
  signedInKnown: boolean
}

export interface ProviderDetail {
  name: string
  displayName: string
  description: string
  setupHint: string
  setupUrl: string
  kind: 'cli' | 'api' | 'local' | 'extension' | string
  enabled: boolean
  declaredCap: number
  model: string | null
  baseUrl: string | null
  probeStatus:
    | 'available'
    | 'no_key'
    | 'not_found'
    | 'unavailable'
    | 'manual'
    | 'installing'
    | 'authenticating'
    | string
  probeNote: string | null
  canInstall: boolean
  canOauth: boolean
  canApiKey: boolean
  apiKeyEnvVar: string | null
  apiKeyUrl: string | null
  apiKeySet: boolean
  canLaunchOllama: boolean
  ollamaModelSuggestions: string[]
  availableModels: string[]
  accounts: AccountDto[]
  activeAccount: string
}

export interface SessionInfo {
  sessionId: string
  taskId: string
  taskGoal: string
  role: string
  activeProvider: string
  tokensUsed: number
  graphNodes: number
  handoffsDone: number
  hfsScore: number
  hfsHistory: number[]
  fsmState: string
}

export interface Profile {
  name: string
  chain: string[]
  kinds: string[]
  skills: string[]
  contextHint: string
}

export interface InstructionSource {
  path: string
  origin: string
  label: string
  bytes: number
  sha: string
}

export interface InstructionsState {
  sources: InstructionSource[]
  skills: string[]
}

export interface WalletEntry {
  provider: string
  account: string
  remaining: number
  total: number
  fractionUsed: number
  resetsAt: string | null
  source: string
  burnPerMin: number
  etaMinutes: number
}

export interface HistoryItem {
  seq: number
  ts: string
  event: string
  provider: string
  summary: string
}

export interface CommitItem {
  sha: string
  short: string
  subject: string
  when: string
}

/** One captured message in the tail of a detected session's conversation. */
export interface Turn {
  role: string // 'user' | 'assistant'
  text: string
}

export interface SessionIntel {
  sessionId: string
  transcriptPath: string
  model: string
  initialPrompt: string
  lastPrompt: string
  lastActivity: string
  plan: string[]
  tasksRemaining: string[]
  filesTouched: string[]
  skills: string[]
  mcps: string[]
  recentTurns: Turn[]
  tokensIn: number
  tokensOut: number
  messageCount: number
}

export interface DetectedAgent {
  id: string
  provider: string
  displayName: string
  surface: string
  pid: number
  running: boolean
  cmdline: string
  workDir: string
  account: string
  detectedVia: string[]
  lastActive: number
  session: SessionIntel | null
}

export interface PipelineNode {
  id: string
  provider: string
  task: string
  dependsOn: string[]
  fallback: string[]
  verify: string[]
}

export interface Pipeline {
  name: string
  nodes: PipelineNode[]
}

export interface EventLine {
  id?: number
  ts?: string
  tag?: string
  msg?: string
  type?: string
  [k: string]: unknown
}
