//! Shared data types for the Relay dashboard.
//! Structs deserialized from the Go daemon use camelCase JSON fields.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum ProviderState {
    Active,
    Standby,
    Exhausted,
    Error,
    #[default]
    Unknown,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderStatus {
    pub name: String,
    pub state: ProviderState,
    pub fraction_used: f32,
    pub remaining: Option<i64>,
    pub resets_at: Option<i64>,
    pub is_next: bool,
    pub quota_source: Option<String>,
}

#[derive(Debug, Clone, PartialEq)]
pub enum EventTag {
    ToolUse,
    Result,
    Quota,
    Handoff,
    System,
    Text,
    Waiting,
}

#[derive(Debug, Clone)]
pub struct AgentEventLine {
    pub id: u64,
    pub ts: String,
    pub tag: EventTag,
    pub msg: String,
}

#[derive(Debug, Clone, PartialEq)]
pub enum TimelineKind {
    Start,
    Working,
    Handoff,
    Complete,
    Error,
    Pending,
}

#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct TimelineEntry {
    pub id: String,
    pub kind: TimelineKind,
    pub provider: String,
    pub label: String,
    pub meta: String,
    pub tokens_from: u64,
    pub tokens_to: u64,
}

#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct ContractPreview {
    pub schema_version: String,
    pub signed: bool,
    pub do_not_redo: Vec<String>,
    pub next_action: String,
    pub acceptance: Vec<String>,
    pub acceptance_done: Vec<bool>,
    pub file_manifest: Vec<FileEntryPreview>,
    pub decisions: Vec<DecisionPreview>,
    pub constraints: Vec<ConstraintPreview>,
    // Rich session intent (contract schema v2).
    pub initial_prompt: String,
    pub plan: Vec<String>,
    pub tasks_remaining: Vec<String>,
    pub skills_loaded: Vec<String>,
    pub skills_in_use: Vec<String>,
    pub skills_to_use: Vec<String>,
    pub in_flight_code: Vec<InFlightPreview>,
}

#[derive(Debug, Clone)]
pub struct InFlightPreview {
    pub path: String,
    pub snippet: String,
}

#[derive(Debug, Clone)]
pub struct FileEntryPreview {
    pub path: String,
    pub sha256: String,
    pub modified: bool,
}

#[derive(Debug, Clone)]
pub struct DecisionPreview {
    pub summary: String,
    pub rationale: String,
}

#[derive(Debug, Clone)]
pub struct ConstraintPreview {
    pub rule: String,
    pub source: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionInfo {
    pub session_id: String,
    pub task_id: String,
    pub task_goal: String,
    pub role: String,
    pub active_provider: String,
    pub tokens_used: u64,
    pub graph_nodes: u64,
    pub handoffs_done: u32,
    pub hfs_score: f32,
    pub hfs_history: Vec<f32>,
    pub fsm_state: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GraphNode {
    pub id: String,
    pub node_type: String,
    pub weight: f32,
    pub label: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GraphEdge {
    pub from_id: String,
    pub to_id: String,
    pub edge_type: String,
}

#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct DashboardState {
    pub session: Option<SessionInfo>,
    pub providers: Vec<ProviderStatus>,
    pub provider_details: Vec<ProviderDetail>,
    pub profiles: Vec<Profile>,
    pub vision_config: Option<VisionConfigDto>,
    pub vision_obs: Option<VisionObservation>,
    pub instructions: Option<InstructionsState>,
    pub cost: Option<CostState>,
    pub diff: Option<DiffState>,
    pub approvals: Vec<ApprovalRequest>,
    pub events: Vec<AgentEventLine>,
    pub timeline: Vec<TimelineEntry>,
    pub contract: Option<ContractPreview>,
    pub graph_nodes: Vec<GraphNode>,
    pub graph_edges: Vec<GraphEdge>,
    pub connected: bool,
}

/// Full provider detail including probe result — from /api/config/providers.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ProviderDetail {
    pub name: String,
    pub display_name: String,
    pub description: String,
    pub setup_hint: String,
    pub setup_url: String,
    pub kind: String, // "cli" | "api" | "local" | "extension"
    pub enabled: bool,
    pub declared_cap: i64,
    pub model: Option<String>,
    pub base_url: Option<String>,
    pub probe_status: String, // available / no_key / not_found / unavailable / manual / installing / authenticating
    pub probe_note: Option<String>,
    #[serde(default)]
    pub can_install: bool,
    #[serde(default)]
    pub can_oauth: bool,
    #[serde(default)]
    pub can_api_key: bool,
    #[serde(default)]
    pub api_key_env_var: Option<String>,
    #[serde(default)]
    pub api_key_url: Option<String>,
    #[serde(default)]
    pub api_key_set: bool,
    #[serde(default)]
    pub can_launch_ollama: bool,
    #[serde(default)]
    pub ollama_model_suggestions: Vec<String>,
    #[serde(default)]
    pub available_models: Vec<String>,
    #[serde(default)]
    pub accounts: Vec<AccountDto>,
    #[serde(default)]
    pub active_account: String,
}

/// One selectable login for a provider (account-aware handoff, pillar 3).
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct AccountDto {
    pub label: String,
    #[serde(default)]
    pub active: bool,
    #[serde(default)]
    pub config_dir: String,
}

/// A multi-agent pipeline DAG (pillar 4) — from /api/pipelines.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct PipelineDto {
    pub name: String,
    #[serde(default)]
    pub nodes: Vec<PipelineNodeDto>,
}

/// One agent + task in a pipeline DAG.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct PipelineNodeDto {
    pub id: String,
    #[serde(default)]
    pub provider: String,
    #[serde(default)]
    pub task: String,
    #[serde(default)]
    pub depends_on: Vec<String>,
    #[serde(default)]
    pub fallback: Vec<String>,
    #[serde(default)]
    pub verify: Vec<String>, // acceptance commands (verifier gate)
}

/// One provider+account row in the quota wallet — from /api/quota/wallet.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct WalletEntryDto {
    pub provider: String,
    #[serde(default)]
    pub account: String,
    #[serde(default)]
    pub remaining: i64,
    #[serde(default)]
    pub total: i64,
    #[serde(default)]
    pub fraction_used: f64,
    #[serde(default)]
    pub resets_at: Option<String>,
    #[serde(default)]
    pub source: String,
    #[serde(default)]
    pub burn_per_min: f64,
    #[serde(default)]
    pub eta_minutes: f64,
}

/// One entry in the time-machine handoff timeline — from /api/history.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct HistoryItemDto {
    pub seq: i64,
    pub ts: String,
    pub event: String,
    #[serde(default)]
    pub provider: String,
    pub summary: String,
}

/// One git commit in the snapshot trail — from /api/history/commits.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct CommitDto {
    pub sha: String,
    pub short: String,
    pub subject: String,
    pub when: String,
}

/// Instruction source from /api/instructions.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct InstructionSource {
    pub path: String,
    pub origin: String,
    pub label: String,
    pub bytes: i64,
    pub sha: String,
}

/// Live cost stats — from /api/session/cost.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct CostState {
    pub session_usd: f64,
    pub tokens_in: i64,
    pub tokens_out: i64,
    pub provider: String,
}

/// Diff response — from /api/session/diff.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct DiffState {
    pub summary: String,
    pub diff: String,
}

/// One pending approval request — from /api/approvals.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ApprovalRequest {
    pub id: String,
    pub action: String,
    pub reason: String,
    pub severity: String,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct InstructionsState {
    #[serde(default)]
    pub sources: Vec<InstructionSource>,
    #[serde(default)]
    pub skills: Vec<String>,
}

/// Profile from /api/profiles.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct Profile {
    pub name: String,
    pub chain: Vec<String>,
    pub kinds: Vec<String>,
    pub skills: Vec<String>,
    pub context_hint: String,
}

/// Vision config from /api/vision/config.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct VisionConfigDto {
    pub enabled: bool,
    pub provider: String,
    pub model: String,
    pub api_key_env: String,
    pub base_url: String,
    pub poll_ms: i64,
    pub window_match: String,
    #[serde(default)]
    pub api_key_set: bool,
    #[serde(default)]
    pub available: bool,
    #[serde(default)]
    pub last_error: Option<String>,
}

/// Vision observation from /api/vision/probe.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct VisionObservation {
    pub needs_input: bool,
    pub question: String,
    pub choices: Vec<String>,
    pub summary: String,
    #[serde(default)]
    pub raw_text: String,
}

/// A coding agent detected already running on this machine — from /api/detect.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct DetectedAgent {
    pub id: String,
    pub provider: String,
    pub display_name: String,
    #[serde(default)]
    pub surface: String,
    #[serde(default)]
    pub pid: i64,
    #[serde(default)]
    pub running: bool,
    #[serde(default)]
    pub cmdline: String,
    #[serde(default)]
    pub work_dir: String,
    #[serde(default)]
    pub account: String,
    #[serde(default)]
    pub detected_via: Vec<String>,
    #[serde(default)]
    pub last_active: i64,
    #[serde(default)]
    pub session: Option<SessionIntel>,
}

/// Intent extracted from a detected agent's on-disk session.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct SessionIntel {
    #[serde(default)]
    pub session_id: String,
    #[serde(default)]
    pub transcript_path: String,
    #[serde(default)]
    pub model: String,
    #[serde(default)]
    pub initial_prompt: String,
    #[serde(default)]
    pub last_prompt: String,
    #[serde(default)]
    pub last_activity: String,
    #[serde(default)]
    pub plan: Vec<String>,
    #[serde(default)]
    pub tasks_remaining: Vec<String>,
    #[serde(default)]
    pub files_touched: Vec<String>,
    #[serde(default)]
    pub skills: Vec<String>,
    #[serde(default)]
    pub mcps: Vec<String>,
    #[serde(default)]
    pub tokens_in: i64,
    #[serde(default)]
    pub tokens_out: i64,
    #[serde(default)]
    pub message_count: i64,
}

/// Installed Ollama model — from /api/ollama/models.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct OllamaModel {
    pub name: String,
    pub size: i64,
    pub param_size: String,
    pub family: String,
    pub is_vision: bool,
}

/// Pullable model — curated list, also from /api/ollama/models.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct CuratedVisionModel {
    pub tag: String,
    pub display_name: String,
    pub description: String,
    pub size: String,
}

/// Full response shape from /api/ollama/models.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct OllamaModelsResponse {
    #[serde(default)]
    pub installed: Vec<OllamaModel>,
    #[serde(default)]
    pub curated: Vec<CuratedVisionModel>,
}

impl DashboardState {
    pub fn empty() -> Self {
        Self {
            connected: false,
            session: None,
            providers: Vec::new(),
            provider_details: Vec::new(),
            profiles: Vec::new(),
            vision_config: None,
            vision_obs: None,
            instructions: None,
            cost: None,
            diff: None,
            approvals: Vec::new(),
            events: Vec::new(),
            timeline: Vec::new(),
            contract: None,
            graph_nodes: Vec::new(),
            graph_edges: Vec::new(),
        }
    }
}
