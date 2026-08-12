// normalize.ts — the daemon's Go JSON encoder omits empty/zero array and string
// fields entirely (`omitempty`) rather than sending `[]` or `""`. The Rust egui
// client never noticed because Rust's serde `#[serde(default)]` fills those in
// silently during deserialization. TypeScript has no such step: a screen doing
// `detail.accounts.length` on an omitted field throws immediately.
//
// Fix once at the API boundary rather than sprinkling `?? []` through every
// screen — every array/nullable field a screen might touch gets a real default
// here, right where the raw daemon JSON is turned into our typed DTOs.

import type { ProviderDetail, Profile, Pipeline, PipelineNode, DetectedAgent, SessionIntel } from './types'

export function normalizeProviderDetail(d: Partial<ProviderDetail>): ProviderDetail {
  return {
    name: d.name ?? '',
    displayName: d.displayName ?? '',
    description: d.description ?? '',
    setupHint: d.setupHint ?? '',
    setupUrl: d.setupUrl ?? '',
    kind: d.kind ?? '',
    enabled: d.enabled ?? false,
    declaredCap: d.declaredCap ?? 0,
    model: d.model ?? null,
    baseUrl: d.baseUrl ?? null,
    probeStatus: d.probeStatus ?? '',
    probeNote: d.probeNote ?? null,
    canInstall: d.canInstall ?? false,
    canOauth: d.canOauth ?? false,
    canApiKey: d.canApiKey ?? false,
    apiKeyEnvVar: d.apiKeyEnvVar ?? null,
    apiKeyUrl: d.apiKeyUrl ?? null,
    apiKeySet: d.apiKeySet ?? false,
    canLaunchOllama: d.canLaunchOllama ?? false,
    ollamaModelSuggestions: d.ollamaModelSuggestions ?? [],
    availableModels: d.availableModels ?? [],
    accounts: d.accounts ?? [],
    activeAccount: d.activeAccount ?? '',
  }
}

export function normalizeProfile(p: Partial<Profile>): Profile {
  return {
    name: p.name ?? '',
    chain: p.chain ?? [],
    kinds: p.kinds ?? [],
    skills: p.skills ?? [],
    contextHint: p.contextHint ?? '',
  }
}

function normalizeNode(n: Partial<PipelineNode>): PipelineNode {
  return {
    id: n.id ?? '',
    provider: n.provider ?? '',
    task: n.task ?? '',
    dependsOn: n.dependsOn ?? [],
    fallback: n.fallback ?? [],
    verify: n.verify ?? [],
  }
}

export function normalizePipeline(p: Partial<Pipeline>): Pipeline {
  return { name: p.name ?? '', nodes: (p.nodes ?? []).map(normalizeNode) }
}

function normalizeSessionIntel(s: Partial<SessionIntel> | null | undefined): SessionIntel | null {
  if (!s) return null
  return {
    sessionId: s.sessionId ?? '',
    transcriptPath: s.transcriptPath ?? '',
    model: s.model ?? '',
    initialPrompt: s.initialPrompt ?? '',
    lastPrompt: s.lastPrompt ?? '',
    lastActivity: s.lastActivity ?? '',
    plan: s.plan ?? [],
    tasksRemaining: s.tasksRemaining ?? [],
    filesTouched: s.filesTouched ?? [],
    skills: s.skills ?? [],
    mcps: s.mcps ?? [],
    recentTurns: s.recentTurns ?? [],
    tokensIn: s.tokensIn ?? 0,
    tokensOut: s.tokensOut ?? 0,
    messageCount: s.messageCount ?? 0,
  }
}

export function normalizeDetectedAgent(a: Partial<DetectedAgent>): DetectedAgent {
  return {
    id: a.id ?? '',
    provider: a.provider ?? '',
    displayName: a.displayName ?? '',
    surface: a.surface ?? '',
    pid: a.pid ?? 0,
    running: a.running ?? false,
    cmdline: a.cmdline ?? '',
    workDir: a.workDir ?? '',
    account: a.account ?? '',
    detectedVia: a.detectedVia ?? [],
    lastActive: a.lastActive ?? 0,
    session: normalizeSessionIntel(a.session),
  }
}
