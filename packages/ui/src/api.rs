//! api.rs — polls the Relay daemon HTTP API on localhost:4748.

use std::sync::mpsc;
use std::thread;
use std::time::Duration;

use crate::types::{
    AgentEventLine, ApprovalRequest, ConstraintPreview, ContractPreview, CostState, DashboardState,
    DecisionPreview, DiffState, EventTag, FileEntryPreview, GraphEdge, GraphNode, InFlightPreview,
    InstructionsState, Profile, ProviderDetail, ProviderStatus, SessionInfo, TimelineEntry,
    TimelineKind, VisionConfigDto, VisionObservation,
};

const DAEMON_BASE: &str = "http://127.0.0.1:4748";
const POLL_MS: u64 = 1_500;

/// Open a URL in the user's default browser.
pub fn open_url(url: &str) -> std::io::Result<()> {
    #[cfg(target_os = "windows")]
    {
        std::process::Command::new("cmd")
            .args(["/c", "start", url])
            .spawn()?;
        Ok(())
    }
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open").arg(url).spawn()?;
        Ok(())
    }
    #[cfg(all(unix, not(target_os = "macos")))]
    {
        std::process::Command::new("xdg-open").arg(url).spawn()?;
        Ok(())
    }
}

/// Open a native folder picker. Returns the selected path, or None if cancelled.
/// Blocks the calling thread — caller should run on a background thread
/// or accept a brief UI freeze while the OS dialog is open.
pub fn pick_project_folder() -> Option<String> {
    rfd::FileDialog::new()
        .set_title("Choose project folder for Relay")
        .pick_folder()
        .map(|p| p.to_string_lossy().into_owned())
}

/// Locate the relay binary: check next to current exe first, then fall back to PATH.
fn find_relay_binary() -> String {
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let name = if cfg!(windows) { "relay.exe" } else { "relay" };
            let candidate = dir.join(name);
            if candidate.exists() {
                return candidate.to_string_lossy().into_owned();
            }
        }
    }
    if cfg!(windows) { "relay.exe" } else { "relay" }.to_string()
}

/// Fire-and-forget POST /api/handoff.
pub fn send_handoff() {
    thread::spawn(|| {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let _ = agent
            .post(&format!("{}/api/handoff", DAEMON_BASE))
            .send_string("{}");
    });
}

/// Run `relay init` in the current working directory.
pub fn send_init() {
    thread::spawn(|| {
        let relay = find_relay_binary();
        let _ = std::process::Command::new(&relay).arg("init").spawn();
    });
}

/// Spawn `relay daemon` detached from this UI process so it keeps running
/// after the window is closed (the daemon is the source of truth; the CLI and
/// a future launch of the UI both reattach to it). On Windows we detach from
/// the console and the parent's process group; on Unix we start a new session.
fn spawn_daemon_detached() {
    let relay = find_relay_binary();
    let mut cmd = std::process::Command::new(&relay);
    cmd.arg("daemon");
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP — no inherited console,
        // independent lifetime from the UI.
        cmd.creation_flags(0x0000_0008 | 0x0000_0200);
    }
    #[cfg(unix)]
    {
        use std::os::unix::process::CommandExt;
        // Detach into its own session so it is not killed with the UI.
        unsafe {
            cmd.pre_exec(|| {
                libc_setsid();
                Ok(())
            });
        }
    }
    let _ = cmd.spawn();
}

#[cfg(unix)]
fn libc_setsid() {
    // Minimal setsid without pulling in the libc crate.
    extern "C" {
        fn setsid() -> i32;
    }
    unsafe {
        setsid();
    }
}

/// Returns true if a daemon is already answering on the local API.
pub fn daemon_healthy() -> bool {
    let agent = ureq::AgentBuilder::new()
        .timeout(Duration::from_millis(600))
        .build();
    agent
        .get(&format!("{}/api/health", DAEMON_BASE))
        .call()
        .map(|r| r.status() == 200)
        .unwrap_or(false)
}

/// Start the relay daemon (HTTP server only, no session).
pub fn send_start_daemon() {
    thread::spawn(spawn_daemon_detached);
}

/// Ensure a daemon is running: if one is already reachable, do nothing;
/// otherwise start it detached. Called once at UI startup so opening the app
/// always brings the daemon up (and the CLI can share it).
pub fn ensure_daemon_running() {
    thread::spawn(|| {
        if daemon_healthy() {
            return;
        }
        spawn_daemon_detached();
    });
}

/// Trigger auto-install via POST /api/providers/install.
pub fn send_install_provider(name: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(5))
            .build();
        let payload = serde_json::json!({ "name": name });
        let _ = agent
            .post(&format!("{}/api/providers/install", DAEMON_BASE))
            .send_json(payload);
    });
}

/// Switch the active account for a provider — POST /api/providers/account.
/// Fire-and-forget; the provider poll reflects the new active account (pillar 3).
pub fn send_switch_account(provider: String, label: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(8))
            .build();
        let _ = agent
            .post(&format!("{}/api/providers/account", DAEMON_BASE))
            .send_json(serde_json::json!({ "provider": provider, "label": label }));
    });
}

/// Add a provider account — POST /api/providers/account/add.
pub fn send_add_account(provider: String, label: String, config_dir: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(8))
            .build();
        let _ = agent
            .post(&format!("{}/api/providers/account/add", DAEMON_BASE))
            .send_json(serde_json::json!({
                "provider": provider, "label": label, "configDir": config_dir
            }));
    });
}

/// Remove a provider account — POST /api/providers/account/remove.
pub fn send_remove_account(provider: String, label: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(8))
            .build();
        let _ = agent
            .post(&format!("{}/api/providers/account/remove", DAEMON_BASE))
            .send_json(serde_json::json!({ "provider": provider, "label": label }));
    });
}

/// Open a terminal to sign in to an account — POST /api/providers/account/login.
pub fn send_login_account(provider: String, label: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(8))
            .build();
        let _ = agent
            .post(&format!("{}/api/providers/account/login", DAEMON_BASE))
            .send_json(serde_json::json!({ "provider": provider, "label": label }));
    });
}

/// Start OAuth browser flow via POST /api/providers/oauth.
pub fn send_oauth_provider(name: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(5))
            .build();
        let payload = serde_json::json!({ "name": name });
        let _ = agent
            .post(&format!("{}/api/providers/oauth", DAEMON_BASE))
            .send_json(payload);
    });
}

/// Save API key via POST /api/providers/api-key.
pub fn send_api_key(name: String, value: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(5))
            .build();
        let payload = serde_json::json!({ "name": name, "value": value });
        let _ = agent
            .post(&format!("{}/api/providers/api-key", DAEMON_BASE))
            .send_json(payload);
    });
}

/// Fetch installed Ollama models + curated list. Synchronous-ish via channel.
pub fn send_list_ollama_models(
    base_url: String,
    tx: std::sync::mpsc::Sender<Result<crate::types::OllamaModelsResponse, String>>,
) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(6))
            .build();
        let url = if base_url.is_empty() {
            format!("{}/api/ollama/models", DAEMON_BASE)
        } else {
            format!(
                "{}/api/ollama/models?baseUrl={}",
                DAEMON_BASE,
                urlencoding_minimal(&base_url)
            )
        };
        let result = agent
            .get(&url)
            .call()
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<crate::types::OllamaModelsResponse>()
                    .map_err(|e| e.to_string())
            });
        let _ = tx.send(result);
    });
}

/// Pause/resume agent execution — POST /api/session/pause.
pub fn send_pause(pause: bool) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let _ = agent
            .post(&format!("{}/api/session/pause", DAEMON_BASE))
            .send_json(serde_json::json!({ "pause": pause }));
    });
}

/// Approve or deny a pending request — POST /api/approvals/<id>.
pub fn send_approval(id: String, approved: bool, note: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let _ = agent
            .post(&format!("{}/api/approvals/{}", DAEMON_BASE, id))
            .send_json(serde_json::json!({ "approved": approved, "note": note }));
    });
}

/// Launch a provider via `ollama launch` — POST /api/ollama/launch.
pub fn send_ollama_launch(provider: String, model: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(5))
            .build();
        let _ = agent
            .post(&format!("{}/api/ollama/launch", DAEMON_BASE))
            .send_json(serde_json::json!({ "provider": provider, "model": model }));
    });
}

/// Pull a model from the Ollama library — POST /api/ollama/pull.
pub fn send_pull_ollama_model(tag: String, base_url: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(5))
            .build();
        let _ = agent
            .post(&format!("{}/api/ollama/pull", DAEMON_BASE))
            .send_json(serde_json::json!({ "baseUrl": base_url, "tag": tag }));
    });
}

/// Minimal URL-encode for query string values (alnum + dash/dot/underscore pass through).
fn urlencoding_minimal(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        if c.is_alphanumeric()
            || c == '-'
            || c == '_'
            || c == '.'
            || c == '~'
            || c == '/'
            || c == ':'
        {
            out.push(c);
        } else {
            for b in c.to_string().as_bytes() {
                out.push_str(&format!("%{:02X}", b));
            }
        }
    }
    out
}

/// Upsert a profile via POST /api/profiles.
pub fn send_update_profile(p: Profile) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let _ = agent
            .post(&format!("{}/api/profiles", DAEMON_BASE))
            .send_json(serde_json::json!({
                "name": p.name,
                "chain": p.chain,
                "kinds": p.kinds,
                "skills": p.skills,
                "contextHint": p.context_hint,
                "delete": false,
            }));
    });
}

/// Delete a profile.
pub fn send_delete_profile(name: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let _ = agent
            .post(&format!("{}/api/profiles", DAEMON_BASE))
            .send_json(serde_json::json!({ "name": name, "delete": true }));
    });
}

/// Update vision config.
pub fn send_update_vision_config(cfg: VisionConfigDto) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let _ = agent
            .post(&format!("{}/api/vision/config", DAEMON_BASE))
            .send_json(serde_json::json!({
                "enabled":     cfg.enabled,
                "provider":    cfg.provider,
                "model":       cfg.model,
                "apiKeyEnv":   cfg.api_key_env,
                "baseUrl":     cfg.base_url,
                "pollMs":      cfg.poll_ms,
                "windowMatch": cfg.window_match,
            }));
    });
}

/// Probe vision once. Returns observation via callback channel.
pub fn send_vision_probe(tx: std::sync::mpsc::Sender<Result<VisionObservation, String>>) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(60))
            .build();
        let result = agent
            .post(&format!("{}/api/vision/probe", DAEMON_BASE))
            .send_string("{}")
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<VisionObservation>()
                    .map_err(|e| e.to_string())
            });
        let _ = tx.send(result);
    });
}

/// Scan for AI coding agents already running on this machine — GET /api/detect.
/// On-demand (not polled): the daemon shells out to the OS process table, so we
/// only run it when the user opens the page or presses Rescan.
pub fn send_detect_scan(
    since_hours: i64,
    tx: std::sync::mpsc::Sender<Result<Vec<crate::types::DetectedAgent>, String>>,
) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(20))
            .build();
        let url = if since_hours > 0 {
            format!("{}/api/detect?sinceHours={}", DAEMON_BASE, since_hours)
        } else {
            format!("{}/api/detect", DAEMON_BASE)
        };
        let result = agent
            .get(&url)
            .call()
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<Vec<crate::types::DetectedAgent>>()
                    .map_err(|e| e.to_string())
            });
        let _ = tx.send(result);
    });
}

/// Outcome of an adopt call: the rendered brief plus, when `start` was requested,
/// whether the daemon launched a continuation session.
#[derive(Debug, Clone, Default)]
pub struct AdoptOutcome {
    pub markdown: String,
    pub started: bool,
    pub start_error: Option<String>,
}

/// Render + persist a continuation brief for a detected agent — POST
/// /api/detect/adopt. When `start` is true the daemon also launches a Relay
/// session, pinned to `target`, to continue the lifted work. Returns the brief
/// and the start status on success.
pub fn send_adopt(
    id: String,
    target: String,
    start: bool,
    tx: std::sync::mpsc::Sender<Result<AdoptOutcome, String>>,
) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(20))
            .build();
        let result = agent
            .post(&format!("{}/api/detect/adopt", DAEMON_BASE))
            .send_json(serde_json::json!({ "id": id, "target": target, "start": start }))
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<serde_json::Value>()
                    .map_err(|e| e.to_string())
            })
            .and_then(|v| {
                if let Some(e) = v.get("error").and_then(|e| e.as_str()) {
                    return Err(e.to_string());
                }
                Ok(AdoptOutcome {
                    markdown: v
                        .get("markdown")
                        .and_then(|m| m.as_str())
                        .unwrap_or("")
                        .to_string(),
                    started: v.get("started").and_then(|b| b.as_bool()).unwrap_or(false),
                    start_error: v
                        .get("startError")
                        .and_then(|s| s.as_str())
                        .map(|s| s.to_string()),
                })
            });
        let _ = tx.send(result);
    });
}

/// Fetch all pipelines — GET /api/pipelines (pillar 4).
pub fn send_list_pipelines(
    tx: std::sync::mpsc::Sender<Result<Vec<crate::types::PipelineDto>, String>>,
) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(6))
            .build();
        let result = agent
            .get(&format!("{}/api/pipelines", DAEMON_BASE))
            .call()
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<Vec<crate::types::PipelineDto>>()
                    .map_err(|e| e.to_string())
            });
        let _ = tx.send(result);
    });
}

/// Save the full pipeline list — POST /api/pipelines. `body` is the raw JSON
/// array the user is editing; the daemon validates before persisting.
pub fn send_save_pipelines(body: String, tx: std::sync::mpsc::Sender<Result<(), String>>) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(8))
            .build();
        let result = agent
            .post(&format!("{}/api/pipelines", DAEMON_BASE))
            .set("Content-Type", "application/json")
            .send_string(&body)
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<serde_json::Value>()
                    .map_err(|e| e.to_string())
            })
            .and_then(|v| match v.get("error").and_then(|e| e.as_str()) {
                Some(e) => Err(e.to_string()),
                None => Ok(()),
            });
        let _ = tx.send(result);
    });
}

/// Run a pipeline by name — POST /api/pipelines/run. Fire-and-forget; progress
/// shows up as system events in the dashboard event stream.
pub fn send_run_pipeline(name: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(8))
            .build();
        let _ = agent
            .post(&format!("{}/api/pipelines/run", DAEMON_BASE))
            .send_json(serde_json::json!({ "name": name }));
    });
}

/// Fetch the quota wallet — GET /api/quota/wallet.
pub fn send_wallet(tx: std::sync::mpsc::Sender<Result<Vec<crate::types::WalletEntryDto>, String>>) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(6))
            .build();
        let result = agent
            .get(&format!("{}/api/quota/wallet", DAEMON_BASE))
            .call()
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<Vec<crate::types::WalletEntryDto>>()
                    .map_err(|e| e.to_string())
            });
        let _ = tx.send(result);
    });
}

/// Fetch the time-machine handoff timeline — GET /api/history.
pub fn send_history(
    tx: std::sync::mpsc::Sender<Result<Vec<crate::types::HistoryItemDto>, String>>,
) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(6))
            .build();
        let result = agent
            .get(&format!("{}/api/history", DAEMON_BASE))
            .call()
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<Vec<crate::types::HistoryItemDto>>()
                    .map_err(|e| e.to_string())
            });
        let _ = tx.send(result);
    });
}

/// Fetch the git commit trail — GET /api/history/commits.
pub fn send_commits(tx: std::sync::mpsc::Sender<Result<Vec<crate::types::CommitDto>, String>>) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(6))
            .build();
        let result = agent
            .get(&format!("{}/api/history/commits", DAEMON_BASE))
            .call()
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<Vec<crate::types::CommitDto>>()
                    .map_err(|e| e.to_string())
            });
        let _ = tx.send(result);
    });
}

/// Fetch a commit's diff — GET /api/history/diff?sha=. Returns the diff text.
pub fn send_diff(sha: String, tx: std::sync::mpsc::Sender<Result<String, String>>) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(10))
            .build();
        let result = agent
            .get(&format!(
                "{}/api/history/diff?sha={}",
                DAEMON_BASE,
                urlencoding_minimal(&sha)
            ))
            .call()
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<serde_json::Value>()
                    .map_err(|e| e.to_string())
            })
            .and_then(|v| match v.get("error").and_then(|e| e.as_str()) {
                Some(e) => Err(e.to_string()),
                None => Ok(v
                    .get("diff")
                    .and_then(|d| d.as_str())
                    .unwrap_or("")
                    .to_string()),
            });
        let _ = tx.send(result);
    });
}

/// Create a non-destructive rewind branch at a snapshot — POST /api/history/rewind.
pub fn send_rewind(sha: String, tx: std::sync::mpsc::Sender<Result<String, String>>) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(8))
            .build();
        let result = agent
            .post(&format!("{}/api/history/rewind", DAEMON_BASE))
            .send_json(serde_json::json!({ "sha": sha }))
            .map_err(|e| e.to_string())
            .and_then(|r| {
                r.into_json::<serde_json::Value>()
                    .map_err(|e| e.to_string())
            })
            .and_then(|v| match v.get("error").and_then(|e| e.as_str()) {
                Some(e) => Err(e.to_string()),
                None => Ok(v
                    .get("hint")
                    .and_then(|h| h.as_str())
                    .unwrap_or("rewind branch created")
                    .to_string()),
            });
        let _ = tx.send(result);
    });
}

/// Send a user reply to the active session via POST /api/session/reply.
pub fn send_session_reply(reply: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let _ = agent
            .post(&format!("{}/api/session/reply", DAEMON_BASE))
            .send_json(serde_json::json!({ "reply": reply }));
    });
}

/// Toggle a provider's enabled state via POST /api/config/providers.
pub fn send_update_provider(
    name: String,
    enabled: bool,
    cap: Option<i64>,
    model: Option<String>,
    base_url: Option<String>,
) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();
        let mut payload = serde_json::json!({ "name": name, "enabled": enabled });
        if let Some(c) = cap {
            payload["declaredCap"] = serde_json::json!(c);
        }
        if let Some(m) = model {
            payload["model"] = serde_json::json!(m);
        }
        if let Some(u) = base_url {
            payload["baseUrl"] = serde_json::json!(u);
        }
        let _ = agent
            .post(&format!("{}/api/config/providers", DAEMON_BASE))
            .send_json(payload);
    });
}

/// Launch a task. If the daemon is already running, POST /api/run.
/// Otherwise spawn `relay run --yes "<task>"` which starts its own daemon.
pub fn send_run_task(task: String) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();

        let payload = serde_json::json!({"task": task, "threshold": 0.85});
        let ok = agent
            .post(&format!("{}/api/run", DAEMON_BASE))
            .send_json(payload)
            .map(|r| {
                r.into_json::<serde_json::Value>()
                    .ok()
                    .and_then(|v| v["ok"].as_bool())
                    .unwrap_or(false)
            })
            .unwrap_or(false);

        if !ok {
            // Daemon not running or no run callback — spawn as standalone process
            let relay = find_relay_binary();
            let _ = std::process::Command::new(&relay)
                .args(["run", "--yes", &task])
                .spawn();
        }
    });
}

pub fn fetch_project_graph(project_path: &str) -> Option<(Vec<GraphNode>, Vec<GraphEdge>)> {
    let agent = ureq::AgentBuilder::new()
        .timeout(Duration::from_secs(5))
        .build();

    let url = format!(
        "{}/api/graph/project?path={}",
        DAEMON_BASE,
        urlencoding_minimal(project_path)
    );

    let graph_detail = agent
        .get(&url)
        .call()
        .ok()
        .and_then(|r| r.into_json::<serde_json::Value>().ok());

    let nodes: Vec<GraphNode> = graph_detail
        .as_ref()
        .and_then(|v| serde_json::from_value(v["nodes"].clone()).ok())
        .unwrap_or_default();

    let edges: Vec<GraphEdge> = graph_detail
        .as_ref()
        .and_then(|v| serde_json::from_value(v["edges"].clone()).ok())
        .unwrap_or_default();

    Some((nodes, edges))
}

pub fn spawn_poll_thread(tx: mpsc::Sender<DashboardState>) {
    thread::spawn(move || {
        let agent = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(3))
            .build();

        let mut last_event_id: u64 = 0;
        let mut state = DashboardState::empty();
        let mut daemon_launched = false;

        loop {
            match poll_daemon(&agent, &mut last_event_id) {
                Ok(updates) => {
                    state = merge_updates(state.clone(), updates);
                    state.connected = true;
                    // A successful poll re-arms the auto-start: if the daemon
                    // dies later, the next failure relaunches it once.
                    daemon_launched = false;
                }
                Err(_) => {
                    state.connected = false;
                    // Auto-start daemon on first failure — user only needs to
                    // open relay-ui; they never have to run a second command.
                    // Spawned detached so it outlives this UI process.
                    if !daemon_launched {
                        daemon_launched = true;
                        spawn_daemon_detached();
                    }
                }
            }

            if tx.send(state.clone()).is_err() {
                break;
            }
            thread::sleep(Duration::from_millis(POLL_MS));
        }
    });
}

// ── Poll helpers ──────────────────────────────────────────────────────────────

struct PollResult {
    session: Option<SessionInfo>,
    providers: Option<Vec<ProviderStatus>>,
    provider_details: Option<Vec<ProviderDetail>>,
    profiles: Option<Vec<Profile>>,
    vision_config: Option<VisionConfigDto>,
    instructions: Option<InstructionsState>,
    cost: Option<CostState>,
    diff: Option<DiffState>,
    approvals: Option<Vec<ApprovalRequest>>,
    new_events: Vec<AgentEventLine>,
    contract: Option<ContractPreview>,
    graph_nodes: Option<Vec<GraphNode>>,
    graph_edges: Option<Vec<GraphEdge>>,
}

fn poll_daemon(
    agent: &ureq::Agent,
    last_event_id: &mut u64,
) -> Result<PollResult, Box<dyn std::error::Error>> {
    agent.get(&format!("{}/api/health", DAEMON_BASE)).call()?;

    // ── /api/status → SessionInfo ─────────────────────────────────────────────
    let session: Option<SessionInfo> = agent
        .get(&format!("{}/api/status", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<SessionInfo>().ok());

    // ── /api/providers → Vec<ProviderStatus> ─────────────────────────────────
    let providers: Option<Vec<ProviderStatus>> = agent
        .get(&format!("{}/api/providers", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<Vec<ProviderStatus>>().ok());

    // ── /api/events?since=N → new events ─────────────────────────────────────
    let new_events: Vec<AgentEventLine> = agent
        .get(&format!(
            "{}/api/events?since={}",
            DAEMON_BASE, last_event_id
        ))
        .call()
        .ok()
        .and_then(|r| r.into_json::<Vec<serde_json::Value>>().ok())
        .map(|vs| vs.into_iter().filter_map(parse_event_json).collect())
        .unwrap_or_default();

    if let Some(last) = new_events.last() {
        *last_event_id = last.id;
    }

    // ── /api/contract → ContractPreview ──────────────────────────────────────
    let contract: Option<ContractPreview> = agent
        .get(&format!("{}/api/contract", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<serde_json::Value>().ok())
        .and_then(parse_contract_json);

    // ── /api/config/providers → ProviderDetail list ──────────────────────────
    let provider_details: Option<Vec<ProviderDetail>> = agent
        .get(&format!("{}/api/config/providers", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<Vec<ProviderDetail>>().ok());

    // ── /api/profiles → Profile list ─────────────────────────────────────────
    let profiles: Option<Vec<Profile>> = agent
        .get(&format!("{}/api/profiles", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<Vec<Profile>>().ok());

    // ── /api/vision/config → VisionConfigDto ─────────────────────────────────
    let vision_config: Option<VisionConfigDto> = agent
        .get(&format!("{}/api/vision/config", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<VisionConfigDto>().ok());

    // ── /api/instructions → InstructionsState ────────────────────────────────
    let instructions: Option<InstructionsState> = agent
        .get(&format!("{}/api/instructions", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<InstructionsState>().ok());

    // ── /api/session/cost ────────────────────────────────────────────────────
    let cost: Option<CostState> = agent
        .get(&format!("{}/api/session/cost", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<CostState>().ok());

    // ── /api/session/diff ────────────────────────────────────────────────────
    let diff: Option<DiffState> = agent
        .get(&format!("{}/api/session/diff", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<DiffState>().ok());

    // ── /api/approvals ───────────────────────────────────────────────────────
    let approvals: Option<Vec<ApprovalRequest>> = agent
        .get(&format!("{}/api/approvals", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<Vec<ApprovalRequest>>().ok());

    // ── /api/graph/detail → nodes + edges ────────────────────────────────────
    let graph_detail = agent
        .get(&format!("{}/api/graph/detail", DAEMON_BASE))
        .call()
        .ok()
        .and_then(|r| r.into_json::<serde_json::Value>().ok());

    let graph_nodes: Option<Vec<GraphNode>> = graph_detail
        .as_ref()
        .and_then(|v| serde_json::from_value(v["nodes"].clone()).ok());

    let graph_edges: Option<Vec<GraphEdge>> = graph_detail
        .as_ref()
        .and_then(|v| serde_json::from_value(v["edges"].clone()).ok());

    Ok(PollResult {
        session,
        providers,
        provider_details,
        profiles,
        vision_config,
        instructions,
        cost,
        diff,
        approvals,
        new_events,
        contract,
        graph_nodes,
        graph_edges,
    })
}

fn parse_event_json(v: serde_json::Value) -> Option<AgentEventLine> {
    let id = v["id"].as_u64()?;
    let ts = v["ts"].as_str()?.to_string();
    let tag = match v["tag"].as_str()? {
        "tool_use" => EventTag::ToolUse,
        "result" => EventTag::Result,
        "quota" => EventTag::Quota,
        "handoff" => EventTag::Handoff,
        "text" => EventTag::Text,
        "waiting" => EventTag::Waiting,
        _ => EventTag::System,
    };
    let msg = v["msg"].as_str()?.to_string();
    Some(AgentEventLine { id, ts, tag, msg })
}

fn parse_contract_json(v: serde_json::Value) -> Option<ContractPreview> {
    if v.get("error").is_some() {
        return None;
    }

    // Handles both the Go daemon JSON shape and the legacy TS shape
    let signed = v["signature"].is_string() && !v["signature"].as_str().unwrap_or("").is_empty();
    Some(ContractPreview {
        schema_version: v
            .get("version")
            .and_then(|x| x.as_u64())
            .map(|n| format!("{}.0.0", n))
            .unwrap_or_else(|| "?".into()),
        signed,
        do_not_redo: json_str_array(&v["doNotRedo"]),
        next_action: v["nextAction"].as_str().unwrap_or("").to_string(),
        acceptance: json_str_array(&v["acceptanceAssertions"]),
        acceptance_done: vec![false; v["acceptanceAssertions"].as_array().map_or(0, |a| a.len())],
        file_manifest: parse_file_manifest(&v["fileManifest"]),
        decisions: parse_decisions(&v["decisions"]),
        constraints: parse_constraints(&v["constraints"]),
        initial_prompt: v["initialPrompt"].as_str().unwrap_or("").to_string(),
        plan: json_str_array(&v["plan"]),
        tasks_remaining: json_str_array(&v["tasksRemaining"]),
        skills_loaded: json_str_array(&v["skillsLoaded"]),
        skills_in_use: json_str_array(&v["skillsInUse"]),
        skills_to_use: json_str_array(&v["skillsToUse"]),
        in_flight_code: parse_in_flight(&v["inFlightCode"]),
    })
}

fn parse_in_flight(v: &serde_json::Value) -> Vec<InFlightPreview> {
    v.as_array()
        .map(|a| {
            a.iter()
                .map(|e| InFlightPreview {
                    path: e["path"].as_str().unwrap_or("").to_string(),
                    snippet: e["snippet"].as_str().unwrap_or("").to_string(),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn json_str_array(v: &serde_json::Value) -> Vec<String> {
    v.as_array()
        .map(|a| {
            a.iter()
                .filter_map(|s| s.as_str().map(String::from))
                .collect()
        })
        .unwrap_or_default()
}

fn merge_updates(mut state: DashboardState, updates: PollResult) -> DashboardState {
    if let Some(s) = updates.session {
        state.session = Some(s);
    }
    if let Some(p) = updates.providers {
        state.providers = p;
    }
    if let Some(d) = updates.provider_details {
        state.provider_details = d;
    }
    if let Some(p) = updates.profiles {
        state.profiles = p;
    }
    if let Some(v) = updates.vision_config {
        state.vision_config = Some(v);
    }
    if let Some(i) = updates.instructions {
        state.instructions = Some(i);
    }
    if let Some(c) = updates.cost {
        state.cost = Some(c);
    }
    if let Some(d) = updates.diff {
        state.diff = Some(d);
    }
    if let Some(a) = updates.approvals {
        state.approvals = a;
    }
    if let Some(c) = updates.contract {
        state.contract = Some(c);
    }
    if let Some(n) = updates.graph_nodes {
        state.graph_nodes = n;
    }
    if let Some(e) = updates.graph_edges {
        state.graph_edges = e;
    }

    for ev in updates.new_events {
        state.events.push(ev);
        if state.events.len() > 500 {
            state.events.remove(0);
        }
    }
    state.timeline = build_timeline(&state);
    state
}

fn parse_file_manifest(v: &serde_json::Value) -> Vec<FileEntryPreview> {
    v.as_array()
        .map(|items| {
            items
                .iter()
                .filter_map(|item| {
                    Some(FileEntryPreview {
                        path: item["path"].as_str()?.to_string(),
                        sha256: item["sha256"].as_str().unwrap_or("").to_string(),
                        modified: item["modified"].as_bool().unwrap_or(false),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn parse_decisions(v: &serde_json::Value) -> Vec<DecisionPreview> {
    v.as_array()
        .map(|items| {
            items
                .iter()
                .filter_map(|item| {
                    Some(DecisionPreview {
                        summary: item["summary"].as_str()?.to_string(),
                        rationale: item["rationale"].as_str().unwrap_or("").to_string(),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn parse_constraints(v: &serde_json::Value) -> Vec<ConstraintPreview> {
    v.as_array()
        .map(|items| {
            items
                .iter()
                .filter_map(|item| {
                    Some(ConstraintPreview {
                        rule: item["rule"].as_str()?.to_string(),
                        source: item["source"].as_str().unwrap_or("").to_string(),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn build_timeline(state: &DashboardState) -> Vec<TimelineEntry> {
    let Some(session) = &state.session else {
        return Vec::new();
    };

    let mut entries = Vec::new();
    entries.push(TimelineEntry {
        id: "start".into(),
        kind: TimelineKind::Start,
        provider: session.active_provider.clone(),
        label: format!("{} session", session.active_provider),
        meta: session.task_id.clone(),
        tokens_from: 0,
        tokens_to: 0,
    });

    for ev in state
        .events
        .iter()
        .filter(|ev| ev.tag == EventTag::Handoff)
        .take(20)
    {
        entries.push(TimelineEntry {
            id: format!("event-{}", ev.id),
            kind: TimelineKind::Handoff,
            provider: session.active_provider.clone(),
            label: "handoff".into(),
            meta: format!("{} {}", ev.ts, ev.msg),
            tokens_from: 0,
            tokens_to: 0,
        });
    }

    let kind = match session.fsm_state.as_str() {
        "ERROR" => TimelineKind::Error,
        "COMPLETE" | "DONE" | "FINISHED" => TimelineKind::Complete,
        "RUNNING" => TimelineKind::Working,
        "DISPATCHED" | "PAUSING" | "SNAPSHOTTED" | "ENVELOPE_BUILT" | "RESUMING" => {
            TimelineKind::Pending
        }
        _ => TimelineKind::Working,
    };
    entries.push(TimelineEntry {
        id: "current".into(),
        kind,
        provider: session.active_provider.clone(),
        label: session.fsm_state.to_lowercase(),
        meta: session.task_goal.clone(),
        tokens_from: 0,
        tokens_to: session.tokens_used,
    });

    entries
}
