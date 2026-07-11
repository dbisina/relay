//! app.rs — RelayApp: eframe::App implementation.
//!
//! Direction A: 38px titlebar | 48px icon rail | 90px provider bar | content | opt 256px context
//! Direction B: 38px titlebar | 220px full sidebar | content | always-on 256px context

use std::collections::HashMap;
use std::sync::mpsc;
use std::time::Instant;

use egui::{
    Align, Color32, Frame, Layout, Rect, RichText, Rounding, ScrollArea, Sense, Stroke, Ui, Vec2,
};

use crate::api::spawn_poll_thread;
use crate::theme::*;
use crate::types::{
    AgentEventLine, ApprovalRequest, DashboardState, DetectedAgent, EventTag, GraphEdge, GraphNode,
    InstructionsState, PipelineDto, Profile, ProviderDetail, ProviderState, ProviderStatus,
    TimelineEntry, TimelineKind, VisionConfigDto, VisionObservation,
};

// ── Enums ─────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq)]
pub enum NavPage {
    Projects,
    Dashboard,
    Detect,
    Graph,
    Profiles,
    Pipeline,
    Wallet,
    History,
    Audit,
    Settings,
}

#[derive(Debug, Clone, PartialEq)]
pub enum LayoutDir {
    A,
    B,
}

#[derive(Debug, Clone, PartialEq)]
pub enum MainTab {
    EventStream,
    Files,
    Decisions,
    Contract,
    Diff,
}

#[derive(Debug, Clone, PartialEq)]
pub enum SettingsTab {
    General,
    Providers,
    Vision,
    Security,
    About,
}

// ── RelayApp ──────────────────────────────────────────────────────────────────

pub struct RelayApp {
    state: DashboardState,
    rx: mpsc::Receiver<DashboardState>,
    nav: NavPage,
    layout_dir: LayoutDir,
    main_tab: MainTab,
    scroll_to_bottom: bool,
    initialized: bool,

    // Drawer / overlays
    show_drawer: bool,
    show_handoff: bool,
    handoff_start: Option<Instant>,

    // Slash palette
    show_palette: bool,
    palette_text: String,
    palette_sel: usize,

    // Pause state (UI-only; reflected via send_pause)
    paused: bool,

    // New-task popup
    new_task_open: bool,
    new_task_text: String,

    // Projects page
    active_project: Option<String>,
    project_task_text: String,

    // Settings page
    settings_tab: SettingsTab,
    settings_hitl: bool,
    settings_telem: bool,
    settings_thresh: u32,

    // Graph simulation
    graph_pos: HashMap<String, Vec2>,
    graph_vel: HashMap<String, Vec2>,
    graph_pan: Vec2,
    graph_scale: f32,
    graph_node_ids: Vec<String>,

    project_graph_nodes: Vec<GraphNode>,
    project_graph_edges: Vec<GraphEdge>,
    last_graph_fetch: Option<Instant>,
}

// initial_nav_page lets RELAY_UI_PAGE pick the startup page (for screenshots /
// deep-linking); defaults to the dashboard.
fn initial_nav_page() -> NavPage {
    match std::env::var("RELAY_UI_PAGE")
        .unwrap_or_default()
        .to_lowercase()
        .as_str()
    {
        "wallet" => NavPage::Wallet,
        "history" => NavPage::History,
        "detect" => NavPage::Detect,
        "pipeline" | "pipelines" => NavPage::Pipeline,
        "projects" => NavPage::Projects,
        "graph" => NavPage::Graph,
        "profiles" => NavPage::Profiles,
        "audit" => NavPage::Audit,
        "settings" => NavPage::Settings,
        _ => NavPage::Dashboard,
    }
}

impl RelayApp {
    pub fn new(cc: &eframe::CreationContext<'_>) -> Self {
        cc.egui_ctx.set_visuals(egui::Visuals::dark());
        apply(&cc.egui_ctx);
        // Opening the app brings the daemon up if it isn't already running.
        // The daemon is spawned detached, so closing this window leaves it
        // running for the CLI to keep using.
        crate::api::ensure_daemon_running();
        let (tx, rx) = mpsc::channel();
        spawn_poll_thread(tx);
        Self {
            state: DashboardState::empty(),
            rx,
            nav: initial_nav_page(),
            layout_dir: LayoutDir::A,
            main_tab: MainTab::EventStream,
            scroll_to_bottom: true,
            initialized: false,
            show_drawer: false,
            show_handoff: false,
            handoff_start: None,
            show_palette: false,
            palette_text: String::new(),
            palette_sel: 0,
            paused: false,
            new_task_open: false,
            new_task_text: String::new(),
            active_project: None,
            project_task_text: String::new(),
            settings_tab: SettingsTab::General,
            settings_hitl: true,
            settings_telem: false,
            settings_thresh: 80,
            graph_pos: HashMap::new(),
            graph_vel: HashMap::new(),
            graph_pan: Vec2::ZERO,
            graph_scale: 1.0,
            graph_node_ids: Vec::new(),
            project_graph_nodes: Vec::new(),
            project_graph_edges: Vec::new(),
            last_graph_fetch: None,
        }
    }

    fn pump_updates(&mut self) {
        while let Ok(s) = self.rx.try_recv() {
            // Update the global graph if on dashboard
            if self.nav == NavPage::Dashboard {
                let new_ids: Vec<String> = s.graph_nodes.iter().map(|n| n.id.clone()).collect();
                if new_ids != self.graph_node_ids {
                    self.reset_graph_layout(&s.graph_nodes);
                }
            }
            self.state = s;
        }

        // Periodically fetch project-specific graph if active
        if self.nav == NavPage::Graph {
            if let Some(proj) = &self.active_project {
                let should_fetch = match self.last_graph_fetch {
                    None => true,
                    Some(t) => t.elapsed().as_secs() >= 3,
                };
                if should_fetch {
                    if let Some((nodes, edges)) = crate::api::fetch_project_graph(proj) {
                        let new_ids: Vec<String> = nodes.iter().map(|n| n.id.clone()).collect();
                        if new_ids != self.graph_node_ids {
                            self.reset_graph_layout(&nodes);
                        }
                        self.project_graph_nodes = nodes;
                        self.project_graph_edges = edges;
                        self.last_graph_fetch = Some(Instant::now());
                    }
                }
            }
        }
    }

    fn reset_graph_layout(&mut self, nodes: &[GraphNode]) {
        use std::f32::consts::TAU;
        let n = nodes.len();
        self.graph_pos.clear();
        self.graph_vel.clear();
        self.graph_node_ids = nodes.iter().map(|n| n.id.clone()).collect();
        for (i, node) in nodes.iter().enumerate() {
            let angle = (i as f32 / n.max(1) as f32) * TAU;
            let r = 180.0 + (node.id.len() as f32 * 7.0) % 60.0;
            self.graph_pos
                .insert(node.id.clone(), Vec2::new(angle.cos() * r, angle.sin() * r));
            self.graph_vel.insert(node.id.clone(), Vec2::ZERO);
        }
    }

    fn step_graph_sim(&mut self, nodes: &[GraphNode], edges: &[GraphEdge]) {
        if nodes.is_empty() {
            return;
        }
        let repulsion = 3500.0_f32;
        let spring_k = 0.12_f32;
        let spring_rest = 70.0_f32;
        let damping = 0.75_f32;
        let center_pull = 0.015_f32;
        let ids: Vec<&str> = nodes.iter().map(|n| n.id.as_str()).collect();
        let mut forces: HashMap<&str, Vec2> = ids.iter().map(|&id| (id, Vec2::ZERO)).collect();
        for i in 0..ids.len() {
            for j in (i + 1)..ids.len() {
                let (pi, pj) = match (self.graph_pos.get(ids[i]), self.graph_pos.get(ids[j])) {
                    (Some(&a), Some(&b)) => (a, b),
                    _ => continue,
                };
                let delta = pi - pj;
                let dist = delta.length().max(1.0);
                let f = repulsion / (dist * dist);
                let dir = delta / dist;
                *forces.get_mut(ids[i]).unwrap() += dir * f;
                *forces.get_mut(ids[j]).unwrap() -= dir * f;
            }
        }
        for edge in edges {
            let (pi, pj) = match (
                self.graph_pos.get(edge.from_id.as_str()),
                self.graph_pos.get(edge.to_id.as_str()),
            ) {
                (Some(&a), Some(&b)) => (a, b),
                _ => continue,
            };
            let delta = pj - pi;
            let dist = delta.length().max(0.01);
            let force = spring_k * (dist - spring_rest);
            let dir = delta / dist;
            if let Some(f) = forces.get_mut(edge.from_id.as_str()) {
                *f += dir * force;
            }
            if let Some(f) = forces.get_mut(edge.to_id.as_str()) {
                *f -= dir * force;
            }
        }
        for node in nodes {
            let p = match self.graph_pos.get_mut(&node.id) {
                Some(x) => x,
                None => continue,
            };
            let v = match self.graph_vel.get_mut(&node.id) {
                Some(x) => x,
                None => continue,
            };
            let f = forces.get(node.id.as_str()).copied().unwrap_or(Vec2::ZERO);
            *v = (*v + f) * damping - *p * center_pull;
            *p += *v;
        }
    }
}

impl eframe::App for RelayApp {
    fn update(&mut self, ctx: &egui::Context, _frame: &mut eframe::Frame) {
        if !self.initialized {
            ctx.send_viewport_cmd(egui::ViewportCommand::InnerSize(Vec2::new(1280.0, 780.0)));
            self.initialized = true;
        }
        self.pump_updates();
        ctx.request_repaint_after(std::time::Duration::from_millis(800));

        if self.nav == NavPage::Graph {
            // Simulate whichever graph is actually on screen: the selected
            // project's graph if one is active, otherwise the live session
            // graph. Previously only the project graph was simulated, so the
            // session view fell back to a static, overlapping dust cloud.
            let (nodes, edges) = if self.active_project.is_some() {
                (
                    self.project_graph_nodes.clone(),
                    self.project_graph_edges.clone(),
                )
            } else {
                (
                    self.state.graph_nodes.clone(),
                    self.state.graph_edges.clone(),
                )
            };
            let new_ids: Vec<String> = nodes.iter().map(|n| n.id.clone()).collect();
            if new_ids != self.graph_node_ids {
                self.reset_graph_layout(&nodes);
            }
            for _ in 0..4 {
                self.step_graph_sim(&nodes, &edges);
            }
            ctx.request_repaint();
        }
        if self.show_handoff {
            ctx.request_repaint();
        }

        let has_session = self.state.session.is_some();
        let show_ctx = match self.layout_dir {
            LayoutDir::A => self.show_drawer && has_session && self.nav == NavPage::Dashboard,
            LayoutDir::B => has_session && self.nav == NavPage::Dashboard,
        };
        let show_provider_bar =
            self.layout_dir == LayoutDir::A && has_session && self.nav == NavPage::Dashboard;

        // ── Panels (egui processes in registration order) ──────────────────
        draw_titlebar(
            ctx,
            &self.state,
            &mut self.layout_dir,
            &mut self.show_handoff,
            &mut self.handoff_start,
            &mut self.new_task_open,
        );

        match self.layout_dir {
            LayoutDir::A => draw_icon_rail(ctx, &mut self.nav),
            LayoutDir::B => draw_full_sidebar(
                ctx,
                &mut self.nav,
                &self.state,
                &mut self.active_project,
                &mut self.new_task_open,
            ),
        }

        if show_ctx {
            let closeable = self.layout_dir == LayoutDir::A;
            draw_context_panel(ctx, &self.state, closeable, &mut self.show_drawer);
        }

        if show_provider_bar {
            draw_provider_bar(ctx, &self.state);
        }

        // Graph refs
        let graph_pos = &self.graph_pos;
        let graph_pan = &mut self.graph_pan;
        let graph_scale = &mut self.graph_scale;
        let project_graph_nodes = &self.project_graph_nodes;
        let project_graph_edges = &self.project_graph_edges;
        let nav_copy = self.nav.clone();
        let mut nav_out: Option<NavPage> = None;

        draw_central(
            ctx,
            &self.state,
            &nav_copy,
            &mut self.main_tab,
            &mut self.scroll_to_bottom,
            &mut self.show_drawer,
            &self.layout_dir,
            graph_pos,
            graph_pan,
            graph_scale,
            &mut self.new_task_open,
            &mut self.new_task_text,
            &mut self.active_project,
            &mut self.project_task_text,
            project_graph_nodes,
            project_graph_edges,
            &mut self.settings_tab,
            &mut self.settings_hitl,
            &mut self.settings_telem,
            &mut self.settings_thresh,
            &mut nav_out,
        );

        if let Some(new_nav) = nav_out {
            self.nav = new_nav;
        }

        // ── Overlays ──────────────────────────────────────────────────────────
        if self.show_handoff {
            draw_handoff_overlay(
                ctx,
                &self.state,
                self.handoff_start.unwrap_or_else(Instant::now),
                &mut self.show_handoff,
            );
        }

        // Ctrl/Cmd+K → toggle slash palette
        let toggle_palette = ctx.input(|i| i.modifiers.command && i.key_pressed(egui::Key::K));
        if toggle_palette {
            self.show_palette = !self.show_palette;
            if self.show_palette {
                self.palette_text.clear();
                self.palette_sel = 0;
            }
        }

        // Approval bar — top of screen when any pending
        if !self.state.approvals.is_empty() {
            draw_approval_bar(ctx, &self.state.approvals);
        }

        // Slash palette modal
        if self.show_palette {
            draw_slash_palette(
                ctx,
                &mut self.show_palette,
                &mut self.palette_text,
                &mut self.palette_sel,
                &mut self.show_handoff,
                &mut self.handoff_start,
                &mut self.new_task_open,
                &mut self.nav,
                &mut self.main_tab,
                &mut self.paused,
            );
        }
    }
}

// ═══════════════════════════════════════════════════════════════════════════
// TITLEBAR
// ═══════════════════════════════════════════════════════════════════════════

fn draw_titlebar(
    ctx: &egui::Context,
    state: &DashboardState,
    layout_dir: &mut LayoutDir,
    show_handoff: &mut bool,
    handoff_start: &mut Option<Instant>,
    new_task_open: &mut bool,
) {
    egui::TopBottomPanel::top("titlebar")
        .exact_height(38.0)
        .frame(
            Frame::none()
                .fill(BG1)
                .inner_margin(egui::Margin::symmetric(SP3, 0.0)),
        )
        .show_separator_line(true)
        .show(ctx, |ui| {
            ui.horizontal_centered(|ui| {
                // ── Left: identity + session info ──────────────────────────
                dot(ui, GREEN, 5.0);
                ui.add_space(SP1);
                ui.label(RichText::new("relay").color(TX2).size(10.0).monospace());

                if let Some(s) = &state.session {
                    ui.add_space(3.0);
                    ui.label(RichText::new("·").color(TX3).size(9.0));
                    ui.add_space(3.0);
                    ui.label(RichText::new(&s.task_id).color(TX1).size(SZ_SM).monospace());
                    ui.add_space(3.0);
                    let goal = if s.task_goal.len() > 60 {
                        format!("{}…", &s.task_goal[..57])
                    } else {
                        s.task_goal.clone()
                    };
                    ui.label(RichText::new(goal).color(TX2).size(SZ_XS));

                    // Direction B: compact provider chips
                    if *layout_dir == LayoutDir::B && !state.providers.is_empty() {
                        ui.add_space(SP2);
                        ui.label(RichText::new("·").color(TX3).size(9.0));
                        ui.add_space(3.0);
                        for (i, p) in state.providers.iter().enumerate() {
                            if i > 0 {
                                ui.label(RichText::new("›").color(TX3).size(9.0));
                            }
                            let (dot_col, name_col) = if p.state == ProviderState::Active {
                                (GREEN, TX0)
                            } else {
                                (TX3, TX2)
                            };
                            if p.state == ProviderState::Active {
                                dot(ui, dot_col, 4.0);
                            }
                            let pct_col = if p.fraction_used > 0.75 { YELLOW } else { TX2 };
                            ui.label(RichText::new(&p.name).color(name_col).size(9.5).monospace());
                            ui.label(
                                RichText::new(format!("{:.0}%", p.fraction_used * 100.0))
                                    .color(pct_col)
                                    .size(9.5)
                                    .monospace(),
                            );
                        }
                    }
                }

                // ── Right ──────────────────────────────────────────────────
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    ui.add_space(SP1);

                    // (Window controls removed — OS provides real ones)

                    // Layout switcher — layout1 (icon rail) / layout2 (full sidebar)
                    ui.add_space(SP2);
                    for (icon_name, is_a, tooltip) in [
                        ("layout1", true, "Focus — icon rail"),
                        ("layout2", false, "Workspace — full sidebar"),
                    ] {
                        let active = is_a == (*layout_dir == LayoutDir::A);
                        let fill = if active {
                            ACCENT_BG
                        } else {
                            Color32::TRANSPARENT
                        };
                        let col = if active { ACCENT } else { TX3 };
                        let (rect, resp) =
                            ui.allocate_exact_size(Vec2::new(22.0, 22.0), Sense::click());
                        ui.painter().rect_filled(rect, R, fill);
                        if active {
                            ui.painter().rect_stroke(rect, R, Stroke::new(1.0, ACCENT));
                        }
                        paint_icon(ui.painter(), rect.center(), 12.0, icon_name, col);
                        if resp.clicked() {
                            *layout_dir = if is_a { LayoutDir::A } else { LayoutDir::B };
                        }
                        if resp.hovered() {
                            ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                            egui::show_tooltip_at_pointer(
                                ui.ctx(),
                                ui.layer_id(),
                                egui::Id::new(tooltip),
                                |ui| {
                                    ui.label(RichText::new(tooltip).color(TX0).size(SZ_XS));
                                },
                            );
                        }
                    }

                    ui.add_space(SP1);
                    ui.separator();
                    ui.add_space(SP1);

                    if let Some(s) = &state.session {
                        // HFS score
                        ui.label(
                            RichText::new(format!("{:.2}", s.hfs_score))
                                .color(GREEN)
                                .size(SZ_SM)
                                .monospace()
                                .strong(),
                        );
                        ui.label(RichText::new("HFS").color(TX3).size(9.0).monospace());
                        ui.add_space(SP2);

                        // Handoff now
                        let warn = state
                            .providers
                            .iter()
                            .any(|p| p.state == ProviderState::Active && p.fraction_used >= 0.70);
                        let hbtn = btn_primary(ui, "Handoff now");
                        if warn {
                            // pulse effect: slightly brighter border
                            let r = hbtn.rect;
                            ui.painter()
                                .rect_stroke(r.expand(1.0), R, Stroke::new(1.5, ACCENT));
                        }
                        if hbtn.clicked() {
                            crate::api::send_handoff();
                            *show_handoff = true;
                            *handoff_start = Some(Instant::now());
                        }
                        ui.add_space(SP1);

                        // New task
                        if btn(ui, "New task").clicked() {
                            *new_task_open = true;
                        }
                    }
                });
            });
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// ICON RAIL — Direction A, 48px
// ═══════════════════════════════════════════════════════════════════════════

fn draw_icon_rail(ctx: &egui::Context, nav: &mut NavPage) {
    // .rail-btn: 38×38, border-radius:7, transparent → rgba(.05) hover → rgba(.07) active
    let items: &[(&str, NavPage, &str)] = &[
        ("projects", NavPage::Projects, "Projects"),
        ("dashboard", NavPage::Dashboard, "Dashboard"),
        ("detect", NavPage::Detect, "Detected agents"),
        ("graph", NavPage::Graph, "Graph"),
        ("profiles", NavPage::Profiles, "Profiles"),
        ("graph", NavPage::Pipeline, "Pipelines"),
        ("dashboard", NavPage::Wallet, "Quota wallet"),
        ("detect", NavPage::History, "Time machine"),
        ("audit", NavPage::Audit, "Audit"),
        ("settings", NavPage::Settings, "Settings"),
    ];
    egui::SidePanel::left("icon_rail")
        .exact_width(48.0)
        .resizable(false)
        .frame(
            Frame::none()
                .fill(BG1)
                .inner_margin(egui::Margin::same(0.0)),
        )
        .show_separator_line(true)
        .show(ctx, |ui| {
            ui.add_space(SP3);
            for (icon, page, label) in items {
                let active = nav == page;
                // Center 38×38 button in 48px rail
                let outer = Vec2::new(48.0, 38.0);
                let (outer_rect, resp) = ui.allocate_exact_size(outer, Sense::click());
                let btn_rect = outer_rect.shrink2(Vec2::new(5.0, 0.0));

                // .rail-btn fill
                let fill = if active {
                    RAIL_ACTIVE
                } else if resp.hovered() {
                    RAIL_HOVER
                } else {
                    Color32::TRANSPARENT
                };
                ui.painter()
                    .rect_filled(btn_rect, Rounding::same(7.0), fill);

                // Active left accent bar (2px, full row height, at rail edge)
                if active {
                    let bar =
                        Rect::from_min_size(outer_rect.min, Vec2::new(2.0, outer_rect.height()));
                    ui.painter().rect_filled(bar, Rounding::ZERO, ACCENT);
                }

                // Icon
                let icon_col = if active {
                    TX0
                } else if resp.hovered() {
                    Color32::from_rgba_premultiplied(166, 166, 166, 166) // rgba(.65)
                } else {
                    NAV_TX // rgba(.38)
                };
                paint_icon(ui.painter(), btn_rect.center(), 16.0, icon, icon_col);

                if resp.hovered() {
                    ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                    egui::show_tooltip_at_pointer(
                        ui.ctx(),
                        ui.layer_id(),
                        egui::Id::new(*label),
                        |ui| {
                            ui.label(RichText::new(*label).color(TX0).size(SZ_XS));
                        },
                    );
                }
                if resp.clicked() {
                    *nav = (*page).clone();
                }
                ui.add_space(3.0);
            }
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// FULL SIDEBAR — Direction B, 220px
// ═══════════════════════════════════════════════════════════════════════════

fn draw_full_sidebar(
    ctx: &egui::Context,
    nav: &mut NavPage,
    state: &DashboardState,
    active_project: &mut Option<String>,
    new_task_open: &mut bool,
) {
    let items: &[(&str, NavPage, &str)] = &[
        ("⊞", NavPage::Projects, "Projects"),
        ("▣", NavPage::Dashboard, "Dashboard"),
        ("⊙", NavPage::Detect, "Detected agents"),
        ("◎", NavPage::Graph, "Graph"),
        ("☰", NavPage::Profiles, "Profiles"),
        ("⛓", NavPage::Pipeline, "Pipelines"),
        ("◈", NavPage::Wallet, "Quota wallet"),
        ("⏱", NavPage::History, "Time machine"),
        ("⛨", NavPage::Audit, "Audit"),
        ("⚙", NavPage::Settings, "Settings"),
    ];
    egui::SidePanel::left("full_sidebar")
        .exact_width(220.0)
        .resizable(false)
        .frame(
            Frame::none()
                .fill(BG1)
                .inner_margin(egui::Margin::same(0.0)),
        )
        .show_separator_line(true)
        .show(ctx, |ui| {
            // Workspace section
            Frame::none()
                .fill(BG1)
                .inner_margin(egui::Margin {
                    left: SP3,
                    right: SP3,
                    top: SP2,
                    bottom: SP2,
                })
                .show(ui, |ui| {
                    ui.label(RichText::new("WORKSPACE").color(TX3).size(8.5).monospace());
                    ui.add_space(SP1);
                    match active_project {
                        Some(proj) => {
                            let short = proj.split('/').next_back().unwrap_or(proj.as_str());
                            Frame::none()
                                .fill(BG3)
                                .stroke(Stroke::new(1.0, BORDER1))
                                .rounding(R_SM)
                                .inner_margin(egui::Margin::symmetric(SP2, SP1))
                                .show(ui, |ui| {
                                    ui.horizontal(|ui| {
                                        let (fi_rect, _) = ui
                                            .allocate_exact_size(Vec2::splat(14.0), Sense::hover());
                                        paint_icon(
                                            ui.painter(),
                                            fi_rect.center(),
                                            14.0,
                                            "folder",
                                            ACCENT,
                                        );
                                        ui.vertical(|ui| {
                                            ui.label(
                                                RichText::new(short)
                                                    .color(TX0)
                                                    .size(10.5)
                                                    .monospace()
                                                    .strong(),
                                            );
                                            ui.label(
                                                RichText::new(proj.as_str())
                                                    .color(TX2)
                                                    .size(8.5)
                                                    .monospace(),
                                            );
                                        });
                                    });
                                });
                        }
                        None => {
                            let (rect, resp) = ui.allocate_exact_size(
                                Vec2::new(ui.available_width(), 30.0),
                                Sense::click(),
                            );
                            let fill = if resp.hovered() { BG3 } else { BG2 };
                            ui.painter().rect_filled(rect, R_SM, fill);
                            ui.painter()
                                .rect_stroke(rect, R_SM, Stroke::new(1.0, BORDER1));
                            ui.painter().text(
                                rect.center(),
                                egui::Align2::CENTER_CENTER,
                                "Open project…",
                                egui::FontId::new(11.0, egui::FontFamily::Proportional),
                                TX2,
                            );
                            if resp.clicked() {
                                if let Some(picked) = crate::api::pick_project_folder() {
                                    *active_project = Some(picked);
                                }
                            }
                            if resp.hovered() {
                                ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                            }
                        }
                    }
                });

            h_rule(ui);

            // Nav items — .nav-item: h:32, pad:0 14px, border-left:2 solid transparent
            ui.add_space(SP1);
            for (icon, page, label) in items {
                let active = nav == page;
                let desired = Vec2::new(ui.available_width(), 32.0);
                let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());

                // Background
                let fill = if active {
                    NAV_ACTIVE
                } else if resp.hovered() {
                    NAV_HOVER
                } else {
                    Color32::TRANSPARENT
                };
                ui.painter().rect_filled(rect, Rounding::ZERO, fill);

                // Left border: 2px, orange on active, transparent otherwise
                if active {
                    let bar = Rect::from_min_size(rect.min, Vec2::new(2.0, rect.height()));
                    ui.painter().rect_filled(bar, Rounding::ZERO, ACCENT);
                }

                // Icon (14px, at left+14px)
                let icon_col = if active {
                    TX0
                } else if resp.hovered() {
                    Color32::from_rgba_premultiplied(166, 166, 166, 166)
                } else {
                    NAV_TX
                };
                let icon_cx = rect.left() + 14.0 + 7.0; // 14px pad + half 14px icon
                paint_icon(
                    ui.painter(),
                    egui::pos2(icon_cx, rect.center().y),
                    14.0,
                    icon,
                    icon_col,
                );

                // Label text
                let text_col = if active {
                    TX0
                } else if resp.hovered() {
                    Color32::from_rgba_premultiplied(166, 166, 166, 166) // rgba(.65)
                } else {
                    NAV_TX
                };
                ui.painter().text(
                    egui::pos2(rect.left() + 14.0 + 14.0 + 9.0, rect.center().y),
                    egui::Align2::LEFT_CENTER,
                    *label,
                    egui::FontId::new(12.5, egui::FontFamily::Proportional),
                    text_col,
                );
                if resp.hovered() {
                    ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                }
                if resp.clicked() {
                    *nav = (*page).clone();
                }
            }

            h_rule(ui);

            // Recent
            ui.add_space(SP1);
            ui.horizontal(|ui| {
                ui.add_space(SP3);
                ui.label(RichText::new("RECENT").color(TX3).size(8.5).monospace());
            });
            ui.add_space(SP1);
            if let Some(s) = &state.session {
                let desired = Vec2::new(ui.available_width(), 24.0);
                let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
                if resp.hovered() {
                    ui.painter().rect_filled(rect, Rounding::ZERO, BG2);
                }
                let truncated = if s.task_goal.len() > 26 {
                    format!("{}…", &s.task_goal[..23])
                } else {
                    s.task_goal.clone()
                };
                ui.painter().text(
                    egui::pos2(rect.left() + SP3, rect.center().y),
                    egui::Align2::LEFT_CENTER,
                    truncated,
                    egui::FontId::new(11.0, egui::FontFamily::Proportional),
                    TX1,
                );
                ui.painter().text(
                    egui::pos2(rect.right() - SP2, rect.center().y),
                    egui::Align2::RIGHT_CENTER,
                    "now",
                    egui::FontId::new(9.5, egui::FontFamily::Monospace),
                    GREEN,
                );
                if resp.clicked() {
                    *nav = NavPage::Dashboard;
                }
                if resp.hovered() {
                    ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                }
            }

            let _ = new_task_open;
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// PROVIDER BAR — Direction A, arc rings
// ═══════════════════════════════════════════════════════════════════════════

fn draw_provider_bar(ctx: &egui::Context, state: &DashboardState) {
    egui::TopBottomPanel::top("provider_bar")
        .exact_height(92.0)
        .frame(
            Frame::none()
                .fill(BG2)
                .inner_margin(egui::Margin::same(0.0)),
        )
        .show_separator_line(true)
        .show(ctx, |ui| {
            ui.horizontal_centered(|ui| {
                ui.add_space(SP5);

                // Provider rings
                for (i, p) in state.providers.iter().enumerate() {
                    if i > 0 {
                        // Connector arrow
                        ui.add_space(SP2);
                        let is_next = p.is_next;
                        let arrow_col = if is_next { ACCENT } else { TX3 };
                        let (rect, _) =
                            ui.allocate_exact_size(Vec2::new(32.0, 10.0), Sense::hover());
                        let painter = ui.painter();
                        let mid_y = rect.center().y;
                        let x0 = rect.left();
                        let x1 = rect.right() - 5.0;
                        painter.line_segment(
                            [egui::pos2(x0, mid_y), egui::pos2(x1, mid_y)],
                            Stroke::new(1.0, arrow_col),
                        );
                        painter.line_segment(
                            [egui::pos2(x1 - 4.0, mid_y - 3.0), egui::pos2(x1, mid_y)],
                            Stroke::new(1.2, arrow_col),
                        );
                        painter.line_segment(
                            [egui::pos2(x1 - 4.0, mid_y + 3.0), egui::pos2(x1, mid_y)],
                            Stroke::new(1.2, arrow_col),
                        );
                        ui.add_space(SP2);
                    }
                    draw_provider_ring(ui, p);
                }

                // Divider
                ui.add_space(SP5);
                let (rect, _) = ui.allocate_exact_size(Vec2::new(1.0, 40.0), Sense::hover());
                ui.painter().rect_filled(rect, Rounding::ZERO, BORDER1);
                ui.add_space(SP5);

                // Session stats
                if let Some(s) = &state.session {
                    let stats = [
                        ("TOKENS", fmt_tokens(s.tokens_used), YELLOW),
                        ("GRAPH", format!("{} nodes", s.graph_nodes), TX0),
                        ("HANDOFFS", format!("{} done", s.handoffs_done), GREEN),
                        (
                            "STATE",
                            s.fsm_state.clone(),
                            if s.fsm_state == "RUNNING" { GREEN } else { TX2 },
                        ),
                    ];
                    for (label, val, col) in stats {
                        ui.vertical(|ui| {
                            ui.add_space(SP2);
                            ui.label(RichText::new(label).color(TX3).size(8.5).monospace());
                            ui.add_space(2.0);
                            ui.label(
                                RichText::new(val)
                                    .color(col)
                                    .size(12.0)
                                    .monospace()
                                    .strong(),
                            );
                        });
                        ui.add_space(SP5);
                    }
                }
            });
        });
}

fn draw_provider_ring(ui: &mut Ui, p: &ProviderStatus) {
    let size = 54.0_f32;
    let ring_r = size * 0.38;
    let active = p.state == ProviderState::Active;
    let sw_bg = if active { 2.5 } else { 2.0 };
    let sw_fill = if active { 3.5 } else { 2.5 };

    let fraction = p.fraction_used.clamp(0.0, 1.0);
    let fill_col = if fraction > 0.75 {
        YELLOW
    } else if fraction > 0.55 {
        ACCENT
    } else {
        GREEN
    };
    let ring_col = if active { fill_col } else { TX3 };

    let (alloc_rect, _) = ui.allocate_exact_size(Vec2::new(size, size + 28.0), Sense::hover());
    let ring_center = egui::pos2(alloc_rect.center().x, alloc_rect.top() + size / 2.0);
    let painter = ui.painter();

    // Background ring (full circle)
    paint_arc(
        painter,
        ring_center,
        ring_r,
        0.0,
        1.0,
        Stroke::new(sw_bg, Color32::from_rgba_premultiplied(18, 18, 18, 18)),
    );

    // Fill arc (proportion used)
    if fraction > 0.001 {
        paint_arc(
            painter,
            ring_center,
            ring_r,
            0.0,
            fraction,
            Stroke::new(sw_fill, ring_col),
        );
    }

    // Center pct text
    let pct_text = format!("{:.0}%", fraction * 100.0);
    let pct_size = if active { 10.5 } else { 9.5 };
    let pct_col = if active { TX0 } else { TX2 };
    painter.text(
        ring_center,
        egui::Align2::CENTER_CENTER,
        &pct_text,
        egui::FontId::new(pct_size, egui::FontFamily::Monospace),
        pct_col,
    );

    // Provider name
    let name_y = alloc_rect.top() + size + 4.0;
    let name_size = if active { 12.0 } else { 11.0 };
    let name_col = if active { TX0 } else { TX1 };
    painter.text(
        egui::pos2(alloc_rect.center().x, name_y),
        egui::Align2::CENTER_TOP,
        &p.name,
        egui::FontId::new(name_size, egui::FontFamily::Proportional),
        name_col,
    );

    // State label
    let state_text = if active {
        if fraction > 0.75 {
            "near limit"
        } else {
            "active"
        }
    } else if p.is_next {
        "next ›"
    } else {
        "standby"
    };
    let state_col = if p.is_next {
        ACCENT
    } else if active {
        if fraction > 0.75 {
            YELLOW
        } else {
            GREEN
        }
    } else {
        TX3
    };
    painter.text(
        egui::pos2(alloc_rect.center().x, name_y + 14.0),
        egui::Align2::CENTER_TOP,
        state_text,
        egui::FontId::new(8.5, egui::FontFamily::Monospace),
        state_col,
    );
}

/// Draw an arc (0.0=top, clockwise) using sequential line segments.
fn paint_arc(
    painter: &egui::Painter,
    center: egui::Pos2,
    radius: f32,
    from_frac: f32,
    to_frac: f32,
    stroke: Stroke,
) {
    use std::f32::consts::{FRAC_PI_2, TAU};
    const N: usize = 48;
    let i_start = (N as f32 * from_frac) as usize;
    let i_end = ((N as f32 * to_frac.min(1.0)) as usize + 1).min(N);
    let pts: Vec<egui::Pos2> = (i_start..=i_end)
        .map(|i| {
            let a = (i as f32 / N as f32) * TAU - FRAC_PI_2;
            egui::pos2(center.x + radius * a.cos(), center.y + radius * a.sin())
        })
        .collect();
    for w in pts.windows(2) {
        painter.line_segment([w[0], w[1]], stroke);
    }
}

// ═══════════════════════════════════════════════════════════════════════════
// CONTEXT DRAWER — right panel
// ═══════════════════════════════════════════════════════════════════════════

fn draw_context_panel(
    ctx: &egui::Context,
    state: &DashboardState,
    closeable: bool,
    show_drawer: &mut bool,
) {
    egui::SidePanel::right("context_drawer")
        .exact_width(256.0)
        .resizable(false)
        .frame(
            Frame::none()
                .fill(BG1)
                .inner_margin(egui::Margin::same(0.0)),
        )
        .show_separator_line(true)
        .show(ctx, |ui| {
            // HFS widget
            Frame::none()
                .inner_margin(egui::Margin {
                    left: SP4,
                    right: SP4,
                    top: SP3,
                    bottom: SP2,
                })
                .fill(BG1)
                .show(ui, |ui| {
                    ui.horizontal(|ui| {
                        ui.vertical(|ui| {
                            if let Some(s) = &state.session {
                                ui.horizontal(|ui| {
                                    ui.label(
                                        RichText::new(format!("{:.2}", s.hfs_score))
                                            .color(TX0)
                                            .size(24.0)
                                            .monospace()
                                            .strong(),
                                    );
                                    ui.add_space(SP1);
                                    ui.label(
                                        RichText::new("fidelity").color(TX3).size(8.5).monospace(),
                                    );
                                });
                                ui.add_space(SP2);
                                sparkline(ui, &s.hfs_history, 22.0);
                            } else {
                                ui.label(RichText::new("—").color(TX3).size(24.0).monospace());
                            }
                        });
                        if closeable {
                            ui.with_layout(Layout::right_to_left(Align::Min), |ui| {
                                if ui.small_button("×").clicked() {
                                    *show_drawer = false;
                                }
                            });
                        }
                    });
                });

            h_rule(ui);

            // Timeline
            ui.add_space(4.0);
            // Instructions panel
            if let Some(ins) = &state.instructions {
                if !ins.sources.is_empty() || !ins.skills.is_empty() {
                    draw_instructions_panel(ui, ins);
                    h_rule(ui);
                }
            }

            ui.horizontal(|ui| {
                ui.add_space(SP4);
                ui.label(
                    RichText::new("SESSION TIMELINE")
                        .color(TX3)
                        .size(8.5)
                        .monospace(),
                );
            });
            ui.add_space(SP1);
            ScrollArea::vertical()
                .id_salt("ctx_timeline")
                .max_height(180.0)
                .show(ui, |ui| {
                    for (i, entry) in state.timeline.iter().enumerate() {
                        timeline_row(ui, entry, i < state.timeline.len() - 1);
                    }
                    if state.timeline.is_empty() {
                        ui.horizontal(|ui| {
                            ui.add_space(SP4);
                            ui.label(
                                RichText::new("no events yet")
                                    .color(TX3)
                                    .size(SZ_XS)
                                    .italics(),
                            );
                        });
                    }
                });

            h_rule(ui);

            // Contract preview
            if let Some(c) = &state.contract {
                ScrollArea::vertical()
                    .id_salt("ctx_contract")
                    .show(ui, |ui| {
                        Frame::none()
                            .inner_margin(egui::Margin::symmetric(SP4, SP3))
                            .show(ui, |ui| {
                                ui.horizontal(|ui| {
                                    ui.label(
                                        RichText::new("LAST CONTRACT")
                                            .color(TX3)
                                            .size(8.5)
                                            .monospace(),
                                    );
                                    ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                                        if c.signed {
                                            ui.label(
                                                RichText::new("signed")
                                                    .color(GREEN)
                                                    .size(9.0)
                                                    .monospace(),
                                            );
                                            dot(ui, GREEN, 4.0);
                                        }
                                    });
                                });
                                ui.add_space(SP2);

                                // DO NOT REDO
                                ui.label(
                                    RichText::new("DO NOT REDO")
                                        .color(TX3)
                                        .size(8.0)
                                        .monospace(),
                                );
                                ui.add_space(2.0);
                                Frame::none()
                                    .fill(BG3)
                                    .stroke(Stroke::new(1.0, BORDER0))
                                    .rounding(R_SM)
                                    .inner_margin(egui::Margin::same(SP2))
                                    .show(ui, |ui| {
                                        for s in &c.do_not_redo {
                                            ui.label(
                                                RichText::new(s)
                                                    .color(YELLOW)
                                                    .size(10.5)
                                                    .monospace(),
                                            );
                                        }
                                    });
                                ui.add_space(SP1);
                                // NEXT ACTION
                                ui.label(
                                    RichText::new("NEXT ACTION")
                                        .color(TX3)
                                        .size(8.0)
                                        .monospace(),
                                );
                                ui.add_space(2.0);
                                Frame::none()
                                    .fill(BG3)
                                    .stroke(Stroke::new(1.0, BORDER0))
                                    .rounding(R_SM)
                                    .inner_margin(egui::Margin::same(SP2))
                                    .show(ui, |ui| {
                                        ui.label(
                                            RichText::new(&c.next_action)
                                                .color(TX0)
                                                .size(10.5)
                                                .monospace(),
                                        );
                                    });
                                ui.add_space(SP1);

                                // Acceptance
                                ui.label(
                                    RichText::new("ACCEPTANCE").color(TX3).size(8.0).monospace(),
                                );
                                ui.add_space(2.0);
                                Frame::none()
                                    .fill(BG3)
                                    .stroke(Stroke::new(1.0, BORDER0))
                                    .rounding(R_SM)
                                    .inner_margin(egui::Margin::same(SP2))
                                    .show(ui, |ui| {
                                        for (item, done) in
                                            c.acceptance.iter().zip(c.acceptance_done.iter())
                                        {
                                            let (icon, col) =
                                                if *done { ("✓", GREEN) } else { ("○", TX3) };
                                            ui.horizontal(|ui| {
                                                ui.label(RichText::new(icon).color(col).size(10.5));
                                                ui.label(
                                                    RichText::new(item)
                                                        .color(TX0)
                                                        .size(10.5)
                                                        .monospace(),
                                                );
                                            });
                                        }
                                    });
                            });
                    });
            }
        });
}

// Design spec:
//   grid 24px + 1fr, gap 9px, padding 5px 18px
//   icon: 14×14 circle, marginTop 3
//     kind='active'  → bg=C.grS (10% green), border=1px solid C.gr, inner 5px green dot
//     kind='future'  → bg=C.s3, border=1px DASHED C.b1
//     other (done)   → bg=C.s3, border=1px solid C.b1
//   connector line: x=26, top=18, bottom=0, 1px C.b0
//   text: label 11.5 weight 500 (future→t2, else t0), meta 10.5 t2, tok 9.5 t3 mono
// one_line_fit collapses all whitespace (incl. newlines) to single spaces and
// truncates to roughly fit `width` px, so painter.text renders exactly one line
// and never overflows a fixed-height row into its neighbour.
fn one_line_fit(s: &str, width: f32, font_px: f32) -> String {
    let collapsed: String = s.split_whitespace().collect::<Vec<_>>().join(" ");
    let max_chars = ((width / (font_px * 0.6)) as usize).max(4);
    if collapsed.chars().count() > max_chars {
        collapsed.chars().take(max_chars - 1).collect::<String>() + "…"
    } else {
        collapsed
    }
}

fn timeline_row(ui: &mut Ui, entry: &TimelineEntry, has_more: bool) {
    let pad_x = 18.0;
    let icon_sz = 14.0;
    let gap = 9.0;
    let row_pad_top = 5.0;

    // Determine height by content lines (label + meta + optional tok)
    let has_tok = entry.tokens_to > 0;
    let row_h = if has_tok { 56.0 } else { 44.0 };

    let (rect, _) = ui.allocate_exact_size(Vec2::new(ui.available_width(), row_h), Sense::hover());
    let painter = ui.painter();

    let icon_x = rect.left() + pad_x;
    let icon_cx = icon_x + icon_sz / 2.0; // 26 from left edge with 18 padding
    let icon_top = rect.top() + row_pad_top + 3.0; // marginTop 3
    let icon_cy = icon_top + icon_sz / 2.0;

    // Connector line (behind icon, from icon bottom to row bottom)
    if has_more {
        painter.line_segment(
            [
                egui::pos2(icon_cx, icon_top + icon_sz),
                egui::pos2(icon_cx, rect.bottom()),
            ],
            Stroke::new(1.0, BORDER0),
        );
    }

    // Icon styling per design kinds:
    //   Working = active (green filled dot, green border)
    //   Pending = future (s3 bg, dashed b1 border)
    //   Start/Handoff/Complete/Error = "done" style (s3 bg, solid b1 border)
    let is_future = entry.kind == TimelineKind::Pending;
    let is_active = entry.kind == TimelineKind::Working;
    let bg = if is_active { GREEN_BG } else { BG3 };
    let border_col = if is_active { GREEN } else { BORDER1 };

    painter.circle_filled(egui::pos2(icon_cx, icon_cy), icon_sz / 2.0, bg);

    if is_future {
        // Dashed circle — approximate with 12 small segments
        let radius = icon_sz / 2.0 - 0.5;
        let center = egui::pos2(icon_cx, icon_cy);
        let segs = 12;
        for i in 0..segs {
            if i % 2 == 1 {
                continue;
            }
            let a0 = (i as f32 / segs as f32) * std::f32::consts::TAU;
            let a1 = ((i + 1) as f32 / segs as f32) * std::f32::consts::TAU;
            painter.line_segment(
                [
                    egui::pos2(center.x + radius * a0.cos(), center.y + radius * a0.sin()),
                    egui::pos2(center.x + radius * a1.cos(), center.y + radius * a1.sin()),
                ],
                Stroke::new(1.0, border_col),
            );
        }
    } else {
        painter.circle_stroke(
            egui::pos2(icon_cx, icon_cy),
            icon_sz / 2.0 - 0.5,
            Stroke::new(1.0, border_col),
        );
    }

    // Inner 5px dot for active
    if is_active {
        painter.circle_filled(egui::pos2(icon_cx, icon_cy), 2.5, GREEN);
    }

    // Text column starts at icon_x + 24 + 9
    let tx = icon_x + 24.0 + gap;
    let col_w = rect.right() - tx - pad_x;

    // Label — 11.5px, weight 500. Collapse newlines + truncate so multi-line
    // entries (e.g. file lists) never overflow the fixed row height and overlap
    // the next entry.
    let label_col = if is_future { TX2 } else { TX0 };
    painter.text(
        egui::pos2(tx, icon_cy - 6.0),
        egui::Align2::LEFT_CENTER,
        one_line_fit(&entry.label, col_w, 11.5),
        egui::FontId::new(11.5, egui::FontFamily::Proportional),
        label_col,
    );

    // Meta — 10.5px, t2
    painter.text(
        egui::pos2(tx, icon_cy + 6.0),
        egui::Align2::LEFT_CENTER,
        one_line_fit(&entry.meta, col_w, 10.5),
        egui::FontId::new(10.5, egui::FontFamily::Proportional),
        TX2,
    );

    // Token info — 9.5px mono, t3 (only if tokens present)
    if has_tok {
        let tok_text = if entry.tokens_from > 0 {
            format!(
                "{} → {}",
                fmt_tokens(entry.tokens_from),
                fmt_tokens(entry.tokens_to)
            )
        } else {
            fmt_tokens(entry.tokens_to)
        };
        painter.text(
            egui::pos2(tx, icon_cy + 19.0),
            egui::Align2::LEFT_CENTER,
            &tok_text,
            egui::FontId::new(9.5, egui::FontFamily::Monospace),
            TX3,
        );
    }
}

// ═══════════════════════════════════════════════════════════════════════════
// CENTRAL PANEL router
// ═══════════════════════════════════════════════════════════════════════════

#[allow(clippy::too_many_arguments)]
fn draw_central(
    ctx: &egui::Context,
    state: &DashboardState,
    nav: &NavPage,
    tab: &mut MainTab,
    scroll_bottom: &mut bool,
    show_drawer: &mut bool,
    layout_dir: &LayoutDir,
    graph_pos: &HashMap<String, Vec2>,
    graph_pan: &mut Vec2,
    graph_scale: &mut f32,
    new_task_open: &mut bool,
    _new_task_text: &mut String,
    active_project: &mut Option<String>,
    project_task_text: &mut String,
    project_graph_nodes: &[GraphNode],
    project_graph_edges: &[GraphEdge],
    settings_tab: &mut SettingsTab,
    settings_hitl: &mut bool,
    settings_telem: &mut bool,
    settings_thresh: &mut u32,
    nav_out: &mut Option<NavPage>,
) {
    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0))
        .show(ctx, |ui| {
            if !state.connected && state.session.is_some() {
                Frame::none()
                    .fill(Color32::from_rgba_premultiplied(24, 16, 4, 220))
                    .stroke(Stroke::new(1.0, ACCENT))
                    .inner_margin(egui::Margin::symmetric(SP4, SP2))
                    .show(ui, |ui| {
                        ui.horizontal(|ui| {
                            ui.label(
                                RichText::new("○ disconnected")
                                    .color(ACCENT)
                                    .size(SZ_XS)
                                    .strong()
                                    .monospace(),
                            );
                            ui.add_space(SP2);
                            ui.label(
                                RichText::new("Daemon not reachable — last known state shown.")
                                    .color(TX1)
                                    .size(SZ_XS),
                            );
                        });
                    });
            }

            match nav {
                NavPage::Dashboard => draw_dashboard(
                    ui,
                    state,
                    tab,
                    scroll_bottom,
                    show_drawer,
                    layout_dir,
                    new_task_open,
                ),
                NavPage::Detect => draw_detect_page(ui, state),
                NavPage::Projects => draw_projects_page(
                    ui,
                    state,
                    active_project,
                    project_task_text,
                    new_task_open,
                    nav_out,
                ),
                NavPage::Graph => draw_graph_page(
                    ui,
                    state,
                    graph_pos,
                    graph_pan,
                    graph_scale,
                    active_project,
                    project_graph_nodes,
                    project_graph_edges,
                ),
                NavPage::Profiles => draw_profiles_page_live(ui, state),
                NavPage::Pipeline => draw_pipeline_page(ui, state),
                NavPage::Wallet => draw_wallet_page(ui, state),
                NavPage::History => draw_history_page(ui, state),
                NavPage::Audit => draw_audit_page(ui, state),
                NavPage::Settings => draw_settings_page(
                    ui,
                    state,
                    settings_tab,
                    settings_hitl,
                    settings_telem,
                    settings_thresh,
                ),
            }
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// DASHBOARD
// ═══════════════════════════════════════════════════════════════════════════

fn draw_dashboard(
    ui: &mut Ui,
    state: &DashboardState,
    tab: &mut MainTab,
    scroll_bottom: &mut bool,
    show_drawer: &mut bool,
    layout_dir: &LayoutDir,
    new_task_open: &mut bool,
) {
    if state.session.is_none() {
        draw_empty_dashboard(ui, state, new_task_open);
        return;
    }

    // Panel header
    egui::TopBottomPanel::top("panel_header")
        .exact_height(36.0)
        .frame(
            Frame::none()
                .fill(BG2)
                .inner_margin(egui::Margin::symmetric(SP5, 0.0)),
        )
        .show_separator_line(true)
        .show_inside(ui, |ui| {
            ui.horizontal_centered(|ui| {
                if let Some(s) = &state.session {
                    ui.label(
                        RichText::new(&s.task_id)
                            .color(TX0)
                            .strong()
                            .size(SZ_SM)
                            .monospace(),
                    );
                    ui.add_space(4.0);
                    ui.label(RichText::new("·").color(TX3).size(9.0));
                    ui.add_space(4.0);
                    // Collapse the goal to one line — adopted tasks inline a whole
                    // multi-line brief, which would otherwise blow out this 36px header.
                    let goal_1l = one_line_fit(&s.task_goal, 560.0, SZ_XS);
                    ui.label(
                        RichText::new(format!("{} · role:{}", goal_1l, s.role))
                            .color(TX2)
                            .size(SZ_XS),
                    );
                    ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                        ui.label(
                            RichText::new(&s.session_id)
                                .color(TX3)
                                .size(9.5)
                                .monospace(),
                        );
                        ui.add_space(SP2);
                        // Context drawer toggle (Direction A only)
                        if *layout_dir == LayoutDir::A {
                            let icon = if *show_drawer {
                                "drawer_close"
                            } else {
                                "drawer_open"
                            };
                            let tip = if *show_drawer {
                                "Hide context"
                            } else {
                                "Show context"
                            };
                            if icon_btn(ui, icon, tip).clicked() {
                                *show_drawer = !*show_drawer;
                            }
                        }
                    });
                }
            });
        });

    // Tab bar
    egui::TopBottomPanel::top("tab_bar")
        .exact_height(32.0)
        .frame(
            Frame::none()
                .fill(BG2)
                .inner_margin(egui::Margin::symmetric(SP5, 0.0)),
        )
        .show_separator_line(true)
        .show_inside(ui, |ui| {
            ui.horizontal_centered(|ui| {
                tab_btn(ui, tab, MainTab::EventStream, "Event stream");
                tab_btn(ui, tab, MainTab::Diff, "Diff");
                tab_btn(ui, tab, MainTab::Files, "Files");
                tab_btn(ui, tab, MainTab::Decisions, "Decisions");
                tab_btn(ui, tab, MainTab::Contract, "Contract");
            });
        });

    // Footer status bar
    egui::TopBottomPanel::bottom("session_footer")
        .exact_height(32.0)
        .frame(
            Frame::none()
                .fill(BG1)
                .inner_margin(egui::Margin::symmetric(SP5, 0.0)),
        )
        .show_separator_line(true)
        .show_inside(ui, |ui| {
            ui.horizontal_centered(|ui| {
                if let Some(s) = &state.session {
                    for (lbl, val, col) in [
                        ("ACTIVE", s.active_provider.as_str(), ACCENT),
                        ("TOKENS", &fmt_tokens(s.tokens_used), YELLOW),
                        (
                            "STATE",
                            s.fsm_state.as_str(),
                            if s.fsm_state == "RUNNING" { GREEN } else { TX2 },
                        ),
                    ] {
                        ui.horizontal(|ui| {
                            ui.label(RichText::new(lbl).color(TX3).size(8.0).monospace());
                            ui.add_space(4.0);
                            ui.label(
                                RichText::new(val)
                                    .color(col)
                                    .size(10.5)
                                    .monospace()
                                    .strong(),
                            );
                        });
                        ui.add_space(SP4);
                    }
                    // Cost cell
                    if let Some(c) = &state.cost {
                        ui.horizontal(|ui| {
                            ui.label(RichText::new("COST").color(TX3).size(8.0).monospace());
                            ui.add_space(4.0);
                            let col = if c.session_usd > 1.0 { YELLOW } else { GREEN };
                            ui.label(
                                RichText::new(format!("${:.4}", c.session_usd))
                                    .color(col)
                                    .size(10.5)
                                    .monospace()
                                    .strong(),
                            );
                        });
                        ui.add_space(SP4);
                    }
                }
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    let (c, t) = if state.connected {
                        (GREEN, "● daemon")
                    } else {
                        (TX2, "○ disconnected")
                    };
                    ui.label(RichText::new(t).color(c).size(9.5).monospace());
                    dot(ui, c, 5.0);
                });
            });
        });

    // Content
    egui::CentralPanel::default()
        .frame(
            Frame::none()
                .fill(BG0)
                .inner_margin(egui::Margin::same(0.0)),
        )
        .show_inside(ui, |ui| match tab {
            MainTab::EventStream => draw_event_stream(ui, state, scroll_bottom),
            MainTab::Diff => draw_diff_tab(ui, state),
            MainTab::Files => draw_files_tab(ui, state),
            MainTab::Decisions => draw_decisions_tab(ui, state),
            MainTab::Contract => draw_contract_tab(ui, state),
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// EMPTY / ONBOARDING STATE
// ═══════════════════════════════════════════════════════════════════════════

fn draw_empty_dashboard(ui: &mut Ui, state: &DashboardState, new_task_open: &mut bool) {
    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0).inner_margin(egui::Margin::symmetric(SP4, SP4)))
        .show_inside(ui, |ui| {
            ui.vertical_centered(|ui| {
                ui.add_space(40.0);
                ui.label(RichText::new("No active session").color(TX1).size(15.0).strong());
                ui.add_space(SP2);
                if state.connected {
                    ui.label(RichText::new("Daemon reachable. Start a task to begin.")
                        .color(TX2).size(SZ_XS));
                } else {
                    ui.label(RichText::new("Local daemon not running.")
                        .color(TX2).size(SZ_XS));
                    ui.add_space(SP2);
                    if btn_primary(ui, "Start daemon").clicked() {
                        crate::api::send_start_daemon();
                    }
                }
                ui.add_space(SP4 + SP2);
            });

            ui.vertical_centered(|ui| {
                ui.set_width(ui.available_width().min(680.0));

                if setup_step(ui, "1", "Initialize project",
                    "relay init",
                    "Creates .relay/relay.toml, signing key, audit log, and graph database.",
                    Some("Run init")) {
                    crate::api::send_init();
                }
                ui.add_space(SP2);
                setup_step(ui, "2", "Configure providers",
                    ".relay/relay.toml",
                    "Enable providers, set declared caps, choose handoff order.", None);
                ui.add_space(SP2);
                if setup_step(ui, "3", "Start a task",
                    "relay run \"your task here\"",
                    "Launches daemon and first provider. Streams live events here.",
                    Some("New task")) {
                    *new_task_open = true;
                }
                ui.add_space(SP2);
                setup_step(ui, "4", "Live controls",
                    "New task  ·  Handoff now",
                    "New task launches from any screen. Handoff now appears when a session is active.", None);
            });
        });
}

fn setup_step(
    ui: &mut Ui,
    idx: &str,
    title: &str,
    command: &str,
    detail: &str,
    btn_label: Option<&str>,
) -> bool {
    let mut clicked = false;
    Frame::none()
        .fill(BG3)
        .stroke(Stroke::new(1.0, BORDER0))
        .rounding(R_LG)
        .inner_margin(egui::Margin::symmetric(SP3, SP2))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                ui.label(
                    RichText::new(idx)
                        .color(ACCENT)
                        .size(SZ_SM)
                        .strong()
                        .monospace(),
                );
                ui.add_space(SP2);
                ui.vertical(|ui| {
                    ui.label(RichText::new(title).color(TX0).size(SZ_SM).strong());
                    ui.label(RichText::new(command).color(ACCENT).size(SZ_XS).monospace());
                    ui.label(RichText::new(detail).color(TX2).size(SZ_XS));
                });
                if let Some(label) = btn_label {
                    ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                        if btn_primary(ui, label).clicked() {
                            clicked = true;
                        }
                    });
                }
            });
        });
    clicked
}

// ═══════════════════════════════════════════════════════════════════════════
// EVENT STREAM
// ═══════════════════════════════════════════════════════════════════════════

fn draw_event_stream(ui: &mut Ui, state: &DashboardState, scroll_bottom: &mut bool) {
    // Inline reply box appears when the most recent event is "waiting" — agent
    // is paused for user input. User types here → POST /api/session/reply.
    let waiting = state
        .events
        .iter()
        .rev()
        .take(3)
        .any(|e| e.tag == EventTag::Waiting);

    let bottom_h = if waiting { 56.0 } else { 0.0 };

    egui::TopBottomPanel::bottom("event_reply_bar")
        .exact_height(bottom_h)
        .frame(
            Frame::none()
                .fill(BG1)
                .inner_margin(egui::Margin::symmetric(SP5, SP2)),
        )
        .show_separator_line(waiting)
        .show_inside(ui, |ui| {
            if !waiting {
                return;
            }
            ui.horizontal_centered(|ui| {
                ui.label(
                    RichText::new("Agent waiting →")
                        .color(YELLOW)
                        .size(SZ_XS)
                        .monospace(),
                );
                ui.add_space(SP2);

                let id = egui::Id::new("session_reply_text");
                let mut text: String = ui
                    .ctx()
                    .data_mut(|m| m.get_temp_mut_or_default::<String>(id).clone());
                let resp = ui.add(
                    egui::TextEdit::singleline(&mut text)
                        .desired_width(420.0)
                        .hint_text("type a reply…")
                        .font(egui::FontId::new(SZ_XS, egui::FontFamily::Monospace)),
                );
                ui.ctx().data_mut(|m| m.insert_temp(id, text.clone()));

                let enter = resp.lost_focus()
                    && ui.input(|i| i.key_pressed(egui::Key::Enter))
                    && !text.trim().is_empty();
                let send = btn_primary(ui, "Send").clicked() && !text.trim().is_empty();
                if enter || send {
                    crate::api::send_session_reply(text.trim().to_string());
                    ui.ctx()
                        .data_mut(|m| m.insert_temp::<String>(id, String::new()));
                }
            });
        });

    if state.events.is_empty() {
        empty_tab_note(
            ui,
            "No events yet. Events stream here once the active agent produces output.",
        );
        return;
    }
    ScrollArea::vertical()
        .id_salt("event_stream")
        .auto_shrink([false, false])
        .stick_to_bottom(*scroll_bottom)
        .show(ui, |ui| {
            ui.add_space(SP2);
            for ev in &state.events {
                event_line(ui, ev);
            }
            ui.add_space(SP2);
        });
    if ui.input(|i| i.raw_scroll_delta.y < -5.0) {
        *scroll_bottom = false;
    }
    if ui.input(|i| i.raw_scroll_delta.y > 5.0) {
        *scroll_bottom = true;
    }
}

fn event_line(ui: &mut Ui, ev: &AgentEventLine) {
    // .ev-row: grid 46px 68px 1fr, gap 11px, padding 3px 22px
    // hover: rgba(255,255,255,.025)
    // Multi-line messages get collapsed into a single line so rows don't overlap.
    let row_h = SZ_XS + 10.0;
    let (row_rect, row_resp) =
        ui.allocate_exact_size(Vec2::new(ui.available_width(), row_h), Sense::hover());
    if row_resp.hovered() {
        ui.painter()
            .rect_filled(row_rect, Rounding::ZERO, ROW_HOVER);
    }

    let gap = 11.0;
    let pad = 22.0;
    let ts_w = 46.0;
    let badge_w = 68.0;
    let y = row_rect.center().y;

    // Timestamp — 46px column
    let ts_x = row_rect.left() + pad;
    ui.painter().text(
        egui::pos2(ts_x, y),
        egui::Align2::LEFT_CENTER,
        &ev.ts,
        egui::FontId::new(SZ_XS, egui::FontFamily::Monospace),
        TX3,
    );

    // Badge — 68px column (pill shaped)
    let badge_x = ts_x + ts_w + gap;
    let (bg, fg, label) = tag_style(&ev.tag);
    let badge_rect = Rect::from_min_size(
        egui::pos2(badge_x, y - (row_h * 0.35)),
        Vec2::new(badge_w - 4.0, row_h * 0.7),
    );
    ui.painter().rect_filled(badge_rect, R_PILL, bg);
    ui.painter().text(
        badge_rect.center(),
        egui::Align2::CENTER_CENTER,
        label,
        egui::FontId::new(9.0, egui::FontFamily::Monospace),
        fg,
    );

    // Message — remaining width
    let msg_x = badge_x + badge_w + gap;
    let msg_col = match ev.tag {
        EventTag::Waiting => TX3,
        EventTag::Quota => YELLOW,
        EventTag::Result => TX0,
        _ => TX1,
    };
    let max_w = row_rect.right() - msg_x - pad;
    // Collapse newlines + tabs into single spaces so multi-line JSON blobs
    // don't render outside the row and overlap the next event.
    let msg: String = ev
        .msg
        .chars()
        .map(|c| {
            if c == '\n' || c == '\r' || c == '\t' {
                ' '
            } else {
                c
            }
        })
        .collect::<String>()
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ");
    // Hard-truncate to fit the column width (approx 7px per mono char @ 11pt).
    let max_chars = (max_w / 7.0) as usize;
    let msg = if msg.chars().count() > max_chars && max_chars > 4 {
        msg.chars().take(max_chars - 1).collect::<String>() + "…"
    } else {
        msg
    };
    ui.painter().text(
        egui::pos2(msg_x, y),
        egui::Align2::LEFT_CENTER,
        &msg,
        egui::FontId::new(SZ_XS, egui::FontFamily::Monospace),
        msg_col,
    );

    // Blinking cursor for wait events
    if ev.tag == EventTag::Waiting {
        let blink = ((ui.input(|i| i.time) * 1.5) as u64).is_multiple_of(2);
        if blink {
            let cursor_x = msg_x
                + ui.painter()
                    .layout_no_wrap(
                        msg.clone(),
                        egui::FontId::new(SZ_XS, egui::FontFamily::Monospace),
                        msg_col,
                    )
                    .size()
                    .x
                + 2.0;
            ui.painter().text(
                egui::pos2(cursor_x, y),
                egui::Align2::LEFT_CENTER,
                "▌",
                egui::FontId::new(SZ_XS, egui::FontFamily::Monospace),
                TX3,
            );
        }
    }
}

fn draw_files_tab(ui: &mut Ui, state: &DashboardState) {
    let Some(contract) = &state.contract else {
        empty_tab_note(ui, "No file manifest yet. Run a task or trigger a handoff.");
        return;
    };
    if contract.file_manifest.is_empty() {
        empty_tab_note(ui, "No files recorded in the latest contract.");
        return;
    }
    ScrollArea::vertical().id_salt("files_tab").show(ui, |ui| {
        ui.add_space(SP3);
        Frame::none()
            .inner_margin(egui::Margin::symmetric(SP5, 0.0))
            .show(ui, |ui| {
                egui::Grid::new("files_grid")
                    .num_columns(3)
                    .striped(true)
                    .spacing([SP4, SP1])
                    .show(ui, |ui| {
                        for h in &["FILE", "MODIFIED", "SHA-256"] {
                            ui.label(RichText::new(*h).color(TX3).size(9.0).monospace().strong());
                        }
                        ui.end_row();
                        for file in &contract.file_manifest {
                            ui.label(RichText::new(&file.path).color(TX0).size(SZ_XS).monospace());
                            let (mc, mt) = if file.modified {
                                (GREEN, "modified")
                            } else {
                                (TX3, "—")
                            };
                            ui.label(RichText::new(mt).color(mc).size(SZ_XS));
                            ui.label(
                                RichText::new(short_sha(&file.sha256))
                                    .color(TX2)
                                    .size(SZ_XS)
                                    .monospace(),
                            );
                            ui.end_row();
                        }
                    });
            });
    });
}

fn draw_decisions_tab(ui: &mut Ui, state: &DashboardState) {
    let Some(contract) = &state.contract else {
        empty_tab_note(
            ui,
            "No decisions yet. They appear after a session builds a continuation contract.",
        );
        return;
    };
    ScrollArea::vertical()
        .id_salt("decisions_tab")
        .show(ui, |ui| {
            ui.add_space(SP3);
            Frame::none()
                .inner_margin(egui::Margin::symmetric(SP5, 0.0))
                .show(ui, |ui| {
                    ui.label(
                        RichText::new("DECISIONS")
                            .color(TX3)
                            .size(9.0)
                            .monospace()
                            .strong(),
                    );
                    ui.add_space(SP2);
                    if contract.decisions.is_empty() {
                        ui.label(
                            RichText::new("No decisions recorded yet.")
                                .color(TX2)
                                .size(SZ_XS),
                        );
                    } else {
                        for d in &contract.decisions {
                            ui.add_space(2.0);
                            ui.label(RichText::new(&d.summary).color(TX0).size(SZ_XS).monospace());
                            if !d.rationale.is_empty() {
                                ui.label(RichText::new(&d.rationale).color(TX2).size(SZ_XS));
                            }
                            ui.add_space(SP2);
                            h_rule(ui);
                            ui.add_space(SP2);
                        }
                    }
                    ui.add_space(SP3);
                    ui.label(
                        RichText::new("CONSTRAINTS")
                            .color(TX3)
                            .size(9.0)
                            .monospace()
                            .strong(),
                    );
                    ui.add_space(SP2);
                    if contract.constraints.is_empty() {
                        ui.label(
                            RichText::new("No constraints recorded yet.")
                                .color(TX2)
                                .size(SZ_XS),
                        );
                    } else {
                        for c in &contract.constraints {
                            let src = if c.source.is_empty() {
                                String::new()
                            } else {
                                format!("[{}] ", c.source)
                            };
                            ui.label(
                                RichText::new(format!("{}{}", src, c.rule))
                                    .color(TX0)
                                    .size(SZ_XS)
                                    .monospace(),
                            );
                        }
                    }
                });
        });
}

fn draw_diff_tab(ui: &mut Ui, state: &DashboardState) {
    let Some(d) = &state.diff else {
        empty_tab_note(
            ui,
            "No diff yet. Diff appears once the agent modifies files in the per-session worktree.",
        );
        return;
    };
    ScrollArea::vertical()
        .id_salt("diff_tab")
        .auto_shrink([false, false])
        .show(ui, |ui| {
            ui.add_space(SP3);
            Frame::none()
                .inner_margin(egui::Margin::symmetric(SP5, 0.0))
                .show(ui, |ui| {
                    ui.label(
                        RichText::new("SUMMARY")
                            .color(TX3)
                            .size(9.0)
                            .monospace()
                            .strong(),
                    );
                    ui.add_space(SP1);
                    Frame::none()
                        .fill(BG3)
                        .stroke(Stroke::new(1.0, BORDER0))
                        .rounding(R_SM)
                        .inner_margin(egui::Margin::same(SP2))
                        .show(ui, |ui| {
                            if d.summary.trim().is_empty() {
                                ui.label(
                                    RichText::new("no changes yet")
                                        .color(TX3)
                                        .size(SZ_XS)
                                        .italics(),
                                );
                            } else {
                                ui.label(
                                    RichText::new(&d.summary).color(TX1).size(SZ_XS).monospace(),
                                );
                            }
                        });
                    ui.add_space(SP3);
                    ui.label(
                        RichText::new("UNIFIED DIFF")
                            .color(TX3)
                            .size(9.0)
                            .monospace()
                            .strong(),
                    );
                    ui.add_space(SP1);
                    // Render with +/- coloring
                    for line in d.diff.lines() {
                        let col = if line.starts_with("+++") || line.starts_with("---") {
                            TX2
                        } else if line.starts_with("@@") {
                            ACCENT
                        } else if line.starts_with('+') {
                            GREEN
                        } else if line.starts_with('-') {
                            RED
                        } else if line.starts_with("diff ") {
                            TX0
                        } else {
                            TX1
                        };
                        ui.label(RichText::new(line).color(col).size(SZ_XS).monospace());
                    }
                    if d.diff.trim().is_empty() {
                        ui.label(
                            RichText::new("clean — agent has not written any files yet")
                                .color(TX3)
                                .size(SZ_XS)
                                .italics(),
                        );
                    }
                });
        });
}

fn draw_contract_tab(ui: &mut Ui, state: &DashboardState) {
    let Some(c) = &state.contract else {
        empty_tab_note(ui, "No contract yet. Run a task first.");
        return;
    };
    ScrollArea::vertical()
        .id_salt("contract_tab")
        .show(ui, |ui| {
            ui.add_space(SP3);
            Frame::none()
                .inner_margin(egui::Margin::symmetric(SP5, 0.0))
                .show(ui, |ui| {
                    ui.horizontal(|ui| {
                        ui.label(
                            RichText::new("Continuation Contract")
                                .color(TX0)
                                .size(SZ_MD)
                                .strong(),
                        );
                        ui.add_space(SP2);
                        if c.signed {
                            dot(ui, GREEN, 5.0);
                            ui.label(RichText::new("signed").color(GREEN).size(9.5).monospace());
                        }
                    });
                    ui.add_space(SP3);

                    for (lbl, content, col) in [
                        ("DO NOT REDO", c.do_not_redo.join("\n"), YELLOW),
                        ("NEXT ACTION", c.next_action.clone(), TX0),
                    ] {
                        ui.label(RichText::new(lbl).color(TX3).size(8.5).monospace());
                        ui.add_space(2.0);
                        Frame::none()
                            .fill(BG3)
                            .stroke(Stroke::new(1.0, BORDER0))
                            .rounding(R_SM)
                            .inner_margin(egui::Margin::same(SP2))
                            .show(ui, |ui| {
                                ui.label(RichText::new(content).color(col).size(SZ_XS).monospace());
                            });
                        ui.add_space(SP2);
                    }

                    // ── Rich session intent (contract schema v2) ─────────────────────
                    if !c.initial_prompt.is_empty() {
                        contract_block(ui, "ORIGINAL PROMPT", &c.initial_prompt, TX1);
                    }
                    if !c.plan.is_empty() {
                        ui.label(RichText::new("PLAN").color(TX3).size(8.5).monospace());
                        ui.add_space(2.0);
                        Frame::none()
                            .fill(BG3)
                            .stroke(Stroke::new(1.0, BORDER0))
                            .rounding(R_SM)
                            .inner_margin(egui::Margin::same(SP2))
                            .show(ui, |ui| {
                                for step in &c.plan {
                                    let done = !c.tasks_remaining.contains(step);
                                    let (icon, col) =
                                        if done { ("✓", GREEN) } else { ("○", TX2) };
                                    ui.horizontal(|ui| {
                                        ui.label(RichText::new(icon).color(col).size(SZ_XS));
                                        ui.label(
                                            RichText::new(step).color(TX0).size(SZ_XS).monospace(),
                                        );
                                    });
                                }
                            });
                        ui.add_space(SP2);
                    } else if !c.tasks_remaining.is_empty() {
                        contract_block(ui, "TASKS REMAINING", &c.tasks_remaining.join("\n"), TX0);
                    }
                    let mut skill_parts: Vec<String> = Vec::new();
                    if !c.skills_in_use.is_empty() {
                        skill_parts.push(format!("in use: {}", c.skills_in_use.join(", ")));
                    }
                    if !c.skills_to_use.is_empty() {
                        skill_parts.push(format!("to use: {}", c.skills_to_use.join(", ")));
                    }
                    if !c.skills_loaded.is_empty() {
                        skill_parts.push(format!("loaded: {}", c.skills_loaded.join(", ")));
                    }
                    if !skill_parts.is_empty() {
                        contract_block(ui, "SKILLS", &skill_parts.join("\n"), BLUE);
                    }
                    if !c.in_flight_code.is_empty() {
                        ui.label(
                            RichText::new("IN-FLIGHT CODE")
                                .color(TX3)
                                .size(8.5)
                                .monospace(),
                        );
                        ui.add_space(2.0);
                        Frame::none()
                            .fill(BG3)
                            .stroke(Stroke::new(1.0, BORDER0))
                            .rounding(R_SM)
                            .inner_margin(egui::Margin::same(SP2))
                            .show(ui, |ui| {
                                for f in &c.in_flight_code {
                                    ui.label(
                                        RichText::new(&f.path)
                                            .color(ACCENT)
                                            .size(SZ_XS)
                                            .monospace()
                                            .strong(),
                                    );
                                    if !f.snippet.is_empty() {
                                        ui.label(
                                            RichText::new(&f.snippet)
                                                .color(TX1)
                                                .size(9.0)
                                                .monospace(),
                                        );
                                    }
                                }
                            });
                        ui.add_space(SP2);
                    }

                    ui.label(RichText::new("ACCEPTANCE").color(TX3).size(8.5).monospace());
                    ui.add_space(2.0);
                    Frame::none()
                        .fill(BG3)
                        .stroke(Stroke::new(1.0, BORDER0))
                        .rounding(R_SM)
                        .inner_margin(egui::Margin::same(SP2))
                        .show(ui, |ui| {
                            for (item, done) in c.acceptance.iter().zip(c.acceptance_done.iter()) {
                                let (icon, col) = if *done { ("✓", GREEN) } else { ("○", TX2) };
                                ui.horizontal(|ui| {
                                    ui.label(RichText::new(icon).color(col).size(SZ_XS));
                                    ui.label(
                                        RichText::new(item).color(TX0).size(SZ_XS).monospace(),
                                    );
                                });
                            }
                        });
                });
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// PIPELINE DESIGNER (pillar 4)
// ═══════════════════════════════════════════════════════════════════════════

const PIPELINE_TEMPLATE: &str = r#"[
  {
    "name": "build-feature",
    "nodes": [
      { "id": "design", "provider": "claude", "task": "Design the API and data model" },
      { "id": "impl", "provider": "codex", "task": "Implement per the design", "dependsOn": ["design"], "fallback": ["claude"], "verify": ["go build ./..."] },
      { "id": "test", "provider": "claude", "task": "Write and run the tests", "dependsOn": ["impl"], "verify": ["go test ./..."] }
    ]
  }
]"#;

fn trigger_pipeline_load(ui: &Ui, text_id: egui::Id, loading_id: egui::Id) {
    ui.ctx().data_mut(|m| m.insert_temp(loading_id, true));
    let ctx_clone = ui.ctx().clone();
    let (tx, rx) = std::sync::mpsc::channel();
    crate::api::send_list_pipelines(tx);
    std::thread::spawn(move || {
        if let Ok(result) = rx.recv() {
            let text = match result {
                Ok(ps) => serde_json::to_string_pretty(&ps).unwrap_or_else(|_| "[]".into()),
                Err(_) => "[]".to_string(),
            };
            ctx_clone.data_mut(|m| m.insert_temp(text_id, text));
            ctx_clone.data_mut(|m| m.insert_temp(loading_id, false));
            ctx_clone.request_repaint();
        }
    });
}

fn draw_pipeline_page(ui: &mut Ui, _state: &DashboardState) {
    page_header(
        ui,
        "Pipelines",
        "Design multi-agent DAGs — an agent per part, ordered, with fallback on snag",
    );

    let text_id = egui::Id::new("pipeline_text");
    let loaded_id = egui::Id::new("pipeline_loaded");
    let loading_id = egui::Id::new("pipeline_loading");
    let status_id = egui::Id::new("pipeline_status");

    let loaded: bool = ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(loaded_id));
    let loading: bool = ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(loading_id));
    if !loaded && !loading {
        ui.ctx().data_mut(|m| m.insert_temp(loaded_id, true));
        trigger_pipeline_load(ui, text_id, loading_id);
    }

    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0).inner_margin(egui::Margin::symmetric(SP5, SP3)))
        .show_inside(ui, |ui| {
            let mut text = ui.ctx().data_mut(|m| m.get_temp::<String>(text_id)).unwrap_or_default();

            ui.horizontal(|ui| {
                if btn_with_icon(ui, "graph", "Reload").clicked() {
                    trigger_pipeline_load(ui, text_id, loading_id);
                }
                ui.add_space(SP2);
                if btn_primary(ui, "Save").clicked() {
                    let ctx_clone = ui.ctx().clone();
                    let (tx, rx) = std::sync::mpsc::channel();
                    crate::api::send_save_pipelines(text.clone(), tx);
                    std::thread::spawn(move || {
                        if let Ok(result) = rx.recv() {
                            let s = match result {
                                Ok(()) => "OK::saved".to_string(),
                                Err(e) => format!("ERR::{}", e),
                            };
                            ctx_clone.data_mut(|m| m.insert_temp(status_id, s));
                            ctx_clone.request_repaint();
                        }
                    });
                }
                ui.add_space(SP2);
                if chip_select(ui, "Insert template", false).clicked() {
                    text = PIPELINE_TEMPLATE.to_string();
                    ui.ctx().data_mut(|m| m.insert_temp(text_id, text.clone()));
                }
                ui.add_space(SP2);
                ui.label(RichText::new("Edit the DAG as JSON. Save validates (cycles, unknown deps); Run executes in dependency order.")
                    .color(TX3).size(9.0));
            });

            if let Some(s) = ui.ctx().data_mut(|m| m.get_temp::<String>(status_id)) {
                ui.add_space(SP1);
                if s == "OK::saved" {
                    ui.horizontal(|ui| {
                        dot(ui, GREEN, 5.0);
                        ui.label(RichText::new("saved").color(GREEN).size(SZ_XS).monospace());
                    });
                } else if let Some(e) = s.strip_prefix("ERR::") {
                    ui.label(RichText::new(format!("save failed: {}", e)).color(RED).size(SZ_XS).monospace());
                }
            }

            ui.add_space(SP3);
            let resp = ui.add(
                egui::TextEdit::multiline(&mut text)
                    .font(egui::TextStyle::Monospace)
                    .desired_rows(12)
                    .desired_width(f32::INFINITY)
                    .code_editor(),
            );
            if resp.changed() {
                ui.ctx().data_mut(|m| m.insert_temp(text_id, text.clone()));
            }

            ui.add_space(SP3);
            h_rule(ui);
            ui.add_space(SP3);

            match serde_json::from_str::<Vec<PipelineDto>>(&text) {
                Ok(ps) if ps.is_empty() => {
                    empty_note(ui, "No pipelines yet. Press Insert template, then Save.");
                }
                Ok(ps) => {
                    ScrollArea::vertical().id_salt("pipeline_preview").show(ui, |ui| {
                        for p in &ps {
                            draw_pipeline_card(ui, p);
                            ui.add_space(SP3);
                        }
                    });
                }
                Err(e) => empty_note(ui, &format!("JSON error: {} — fix to enable preview + save", e)),
            }
        });
}

fn draw_pipeline_card(ui: &mut Ui, p: &PipelineDto) {
    Frame::none()
        .fill(BG2)
        .stroke(Stroke::new(1.0, BORDER0))
        .rounding(R_SM)
        .inner_margin(egui::Margin::same(SP3))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                ui.label(RichText::new(&p.name).color(TX0).size(SZ_SM).strong());
                ui.add_space(SP2);
                let n = p.nodes.len();
                ui.label(
                    RichText::new(format!("{} node{}", n, if n == 1 { "" } else { "s" }))
                        .color(TX3)
                        .size(9.0)
                        .monospace(),
                );
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if btn_primary(ui, "Run ▷").clicked() {
                        crate::api::send_run_pipeline(p.name.clone());
                    }
                });
            });
            ui.add_space(SP2);
            for node in &p.nodes {
                Frame::none()
                    .fill(BG3)
                    .stroke(Stroke::new(1.0, BORDER0))
                    .rounding(R_SM)
                    .inner_margin(egui::Margin::same(SP2))
                    .show(ui, |ui| {
                        ui.horizontal_wrapped(|ui| {
                            ui.label(
                                RichText::new(&node.id)
                                    .color(ACCENT)
                                    .size(SZ_XS)
                                    .monospace()
                                    .strong(),
                            );
                            ui.label(
                                RichText::new(&node.provider)
                                    .color(BLUE)
                                    .size(SZ_XS)
                                    .monospace(),
                            );
                            if !node.depends_on.is_empty() {
                                ui.label(
                                    RichText::new(format!("after {}", node.depends_on.join(", ")))
                                        .color(TX2)
                                        .size(9.0)
                                        .monospace(),
                                );
                            }
                            if !node.fallback.is_empty() {
                                ui.label(
                                    RichText::new(format!("fallback {}", node.fallback.join(", ")))
                                        .color(YELLOW)
                                        .size(9.0)
                                        .monospace(),
                                );
                            }
                        });
                        if !node.task.is_empty() {
                            ui.label(RichText::new(&node.task).color(TX1).size(9.5).monospace());
                        }
                    });
                ui.add_space(SP1);
            }
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// QUOTA WALLET
// ═══════════════════════════════════════════════════════════════════════════

fn trigger_wallet_load(ui: &Ui, cache: egui::Id, loading: egui::Id) {
    ui.ctx().data_mut(|m| m.insert_temp(loading, true));
    let ctx = ui.ctx().clone();
    let (tx, rx) = std::sync::mpsc::channel();
    crate::api::send_wallet(tx);
    std::thread::spawn(move || {
        if let Ok(r) = rx.recv() {
            let s = match r {
                Ok(w) => format!("OK::{}", serde_json::to_string(&w).unwrap_or_default()),
                Err(e) => format!("ERR::{}", e),
            };
            ctx.data_mut(|m| m.insert_temp(cache, s));
            ctx.data_mut(|m| m.insert_temp(loading, false));
            ctx.request_repaint();
        }
    });
}

fn draw_wallet_page(ui: &mut Ui, _state: &DashboardState) {
    page_header(
        ui,
        "Quota wallet",
        "Remaining quota, reset, and burn-rate forecast across every provider and account",
    );
    let cache = egui::Id::new("wallet_cache");
    let loaded = egui::Id::new("wallet_loaded");
    let loading = egui::Id::new("wallet_loading");
    if !ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(loaded))
    {
        ui.ctx().data_mut(|m| m.insert_temp(loaded, true));
        trigger_wallet_load(ui, cache, loading);
    }
    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0).inner_margin(egui::Margin::symmetric(SP5, SP3)))
        .show_inside(ui, |ui| {
            let busy = ui.ctx().data_mut(|m| *m.get_temp_mut_or_default::<bool>(loading));
            ui.horizontal(|ui| {
                if btn_with_icon(ui, "dashboard", if busy { "Refreshing…" } else { "Refresh" }).clicked() {
                    trigger_wallet_load(ui, cache, loading);
                }
                ui.add_space(SP2);
                ui.label(RichText::new("Fills as sessions run. ETA = forecast at the current burn rate; switch accounts before it hits zero.")
                    .color(TX3).size(9.0));
            });
            ui.add_space(SP3);
            h_rule(ui);
            ui.add_space(SP3);
            match ui.ctx().data_mut(|m| m.get_temp::<String>(cache)) {
                Some(s) if s.starts_with("OK::") => {
                    match serde_json::from_str::<Vec<crate::types::WalletEntryDto>>(&s[4..]) {
                        Ok(rows) if rows.is_empty() => {
                            empty_note(ui, "No quota data yet. Run a task; the wallet fills as providers report usage.");
                        }
                        Ok(rows) => {
                            ScrollArea::vertical().id_salt("wallet_scroll").show(ui, |ui| {
                                for w in &rows {
                                    draw_wallet_row(ui, w);
                                    ui.add_space(SP2);
                                }
                            });
                        }
                        Err(e) => empty_note(ui, &format!("parse error: {}", e)),
                    }
                }
                Some(s) if s.starts_with("ERR::") => {
                    empty_note(ui, &format!("wallet failed: {} (is the daemon running?)", &s[5..]));
                }
                _ => {
                    ui.label(RichText::new("Loading…").color(TX2).size(SZ_XS));
                }
            }
        });
}

fn draw_wallet_row(ui: &mut Ui, w: &crate::types::WalletEntryDto) {
    Frame::none()
        .fill(BG2)
        .stroke(Stroke::new(1.0, BORDER0))
        .rounding(R_SM)
        .inner_margin(egui::Margin::same(SP3))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                let name = if w.account.is_empty() {
                    w.provider.clone()
                } else {
                    format!("{} / {}", w.provider, w.account)
                };
                ui.label(RichText::new(name).color(TX0).size(SZ_SM).strong());
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if w.eta_minutes >= 0.0 {
                        ui.label(
                            RichText::new(format!("~{} left", fmt_minutes(w.eta_minutes)))
                                .color(eta_color(w.eta_minutes))
                                .size(SZ_XS)
                                .monospace()
                                .strong(),
                        );
                    } else if let Some(rs) = &w.resets_at {
                        ui.label(
                            RichText::new(format!("resets {}", short_ts(rs)))
                                .color(TX2)
                                .size(SZ_XS)
                                .monospace(),
                        );
                    }
                });
            });
            ui.add_space(SP1);
            let frac = if w.fraction_used >= 0.0 {
                (w.fraction_used as f32).clamp(0.0, 1.0)
            } else {
                0.0
            };
            let (rect, _) =
                ui.allocate_exact_size(Vec2::new(ui.available_width(), 6.0), Sense::hover());
            ui.painter().rect_filled(rect, Rounding::same(3.0), BG3);
            let fill = Rect::from_min_size(rect.min, Vec2::new(rect.width() * frac, rect.height()));
            let col = if frac > 0.85 {
                RED
            } else if frac > 0.6 {
                YELLOW
            } else {
                GREEN
            };
            ui.painter().rect_filled(fill, Rounding::same(3.0), col);
            ui.add_space(SP1);
            ui.horizontal(|ui| {
                let pct = if w.fraction_used >= 0.0 {
                    format!("{:.0}% used", w.fraction_used * 100.0)
                } else {
                    "usage unknown".to_string()
                };
                ui.label(RichText::new(pct).color(TX2).size(9.0).monospace());
                if w.total > 0 {
                    ui.label(
                        RichText::new(format!("· {}/{}", w.remaining.max(0), w.total))
                            .color(TX3)
                            .size(9.0)
                            .monospace(),
                    );
                }
                ui.label(
                    RichText::new(format!("· {}", w.source))
                        .color(TX3)
                        .size(9.0)
                        .monospace(),
                );
                if w.burn_per_min > 0.0 {
                    ui.label(
                        RichText::new(format!("· {:.0}/min", w.burn_per_min))
                            .color(TX3)
                            .size(9.0)
                            .monospace(),
                    );
                }
            });
        });
}

fn fmt_minutes(m: f64) -> String {
    if m >= 60.0 {
        format!("{:.0}h {:.0}m", (m / 60.0).floor(), m % 60.0)
    } else {
        format!("{:.0}m", m)
    }
}

fn eta_color(m: f64) -> egui::Color32 {
    if m < 10.0 {
        RED
    } else if m < 60.0 {
        YELLOW
    } else {
        GREEN
    }
}

fn short_ts(ts: &str) -> String {
    ts.replace('T', " ").chars().take(16).collect()
}

// ═══════════════════════════════════════════════════════════════════════════
// TIME MACHINE
// ═══════════════════════════════════════════════════════════════════════════

fn trigger_history_load(ui: &Ui, hcache: egui::Id, ccache: egui::Id) {
    let ctx = ui.ctx().clone();
    let (tx, rx) = std::sync::mpsc::channel();
    crate::api::send_history(tx);
    std::thread::spawn(move || {
        if let Ok(Ok(h)) = rx.recv() {
            ctx.data_mut(|m| m.insert_temp(hcache, serde_json::to_string(&h).unwrap_or_default()));
            ctx.request_repaint();
        }
    });
    let ctx2 = ui.ctx().clone();
    let (tx2, rx2) = std::sync::mpsc::channel();
    crate::api::send_commits(tx2);
    std::thread::spawn(move || {
        if let Ok(Ok(c)) = rx2.recv() {
            ctx2.data_mut(|m| m.insert_temp(ccache, serde_json::to_string(&c).unwrap_or_default()));
            ctx2.request_repaint();
        }
    });
}

fn draw_history_page(ui: &mut Ui, _state: &DashboardState) {
    page_header(
        ui,
        "Time machine",
        "Handoff timeline + snapshot commits — diff or non-destructively rewind any point",
    );
    let hcache = egui::Id::new("hist_cache");
    let ccache = egui::Id::new("commits_cache");
    let loaded = egui::Id::new("hist_loaded");
    if !ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(loaded))
    {
        ui.ctx().data_mut(|m| m.insert_temp(loaded, true));
        trigger_history_load(ui, hcache, ccache);
    }
    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0).inner_margin(egui::Margin::symmetric(SP5, SP3)))
        .show_inside(ui, |ui| {
            if btn_with_icon(ui, "detect", "Refresh").clicked() {
                trigger_history_load(ui, hcache, ccache);
            }
            ui.add_space(SP3);
            ScrollArea::vertical().id_salt("history_scroll").show(ui, |ui| {
                // Handoff timeline (from the audit log)
                ui.label(RichText::new("HANDOFF TIMELINE").color(TX3).size(8.5).monospace());
                ui.add_space(SP1);
                let hist: Vec<crate::types::HistoryItemDto> = ui.ctx()
                    .data_mut(|m| m.get_temp::<String>(hcache))
                    .and_then(|s| serde_json::from_str(&s).ok())
                    .unwrap_or_default();
                if hist.is_empty() {
                    empty_note(ui, "No handoff history yet. It records as sessions hand off between agents.");
                } else {
                    for h in hist.iter().rev() {
                        ui.horizontal_wrapped(|ui| {
                            dot(ui, if h.event == "handoff" { ACCENT } else { TX2 }, 4.0);
                            ui.label(RichText::new(short_ts(&h.ts)).color(TX3).size(9.0).monospace());
                            if !h.provider.is_empty() {
                                ui.label(RichText::new(&h.provider).color(BLUE).size(9.0).monospace());
                            }
                            ui.label(RichText::new(&h.summary).color(TX1).size(9.5).monospace());
                        });
                    }
                }
                ui.add_space(SP3);
                h_rule(ui);
                ui.add_space(SP3);
                // Snapshot commit trail (git)
                ui.label(RichText::new("SNAPSHOTS (git)").color(TX3).size(8.5).monospace());
                ui.add_space(SP1);
                let commits: Vec<crate::types::CommitDto> = ui.ctx()
                    .data_mut(|m| m.get_temp::<String>(ccache))
                    .and_then(|s| serde_json::from_str(&s).ok())
                    .unwrap_or_default();
                if commits.is_empty() {
                    empty_note(ui, "No commits (working dir isn't a git repo, or no snapshots yet).");
                } else {
                    for c in &commits {
                        draw_commit_row(ui, c);
                        ui.add_space(SP1);
                    }
                }
                // Diff / rewind output
                if let Some(d) = ui.ctx().data_mut(|m| m.get_temp::<String>(egui::Id::new("hist_diff"))) {
                    ui.add_space(SP3);
                    Frame::none().fill(BG3).stroke(Stroke::new(1.0, BORDER0)).rounding(R_SM)
                        .inner_margin(egui::Margin::same(SP2)).show(ui, |ui| {
                            ScrollArea::vertical().id_salt("diff_scroll").max_height(260.0).show(ui, |ui| {
                                ui.label(RichText::new(d).color(TX1).size(9.0).monospace());
                            });
                        });
                }
                if let Some(msg) = ui.ctx().data_mut(|m| m.get_temp::<String>(egui::Id::new("hist_rewind"))) {
                    ui.add_space(SP2);
                    ui.horizontal(|ui| {
                        dot(ui, GREEN, 5.0);
                        ui.label(RichText::new(msg).color(GREEN).size(SZ_XS).monospace());
                    });
                }
            });
        });
}

fn draw_commit_row(ui: &mut Ui, c: &crate::types::CommitDto) {
    Frame::none()
        .fill(BG2)
        .stroke(Stroke::new(1.0, BORDER0))
        .rounding(R_SM)
        .inner_margin(egui::Margin::same(SP2))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                ui.label(
                    RichText::new(&c.short)
                        .color(ACCENT)
                        .size(SZ_XS)
                        .monospace()
                        .strong(),
                );
                ui.label(RichText::new(&c.subject).color(TX1).size(SZ_XS).monospace());
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if chip_select(ui, "rewind", false).clicked() {
                        let ctx = ui.ctx().clone();
                        let (tx, rx) = std::sync::mpsc::channel();
                        crate::api::send_rewind(c.sha.clone(), tx);
                        std::thread::spawn(move || {
                            if let Ok(r) = rx.recv() {
                                let msg = match r {
                                    Ok(h) => h,
                                    Err(e) => format!("rewind failed: {}", e),
                                };
                                ctx.data_mut(|m| m.insert_temp(egui::Id::new("hist_rewind"), msg));
                                ctx.request_repaint();
                            }
                        });
                    }
                    ui.add_space(SP1);
                    if chip_select(ui, "diff", false).clicked() {
                        let ctx = ui.ctx().clone();
                        let (tx, rx) = std::sync::mpsc::channel();
                        crate::api::send_diff(c.sha.clone(), tx);
                        std::thread::spawn(move || {
                            if let Ok(r) = rx.recv() {
                                let d = match r {
                                    Ok(d) => d,
                                    Err(e) => format!("diff failed: {}", e),
                                };
                                ctx.data_mut(|m| m.insert_temp(egui::Id::new("hist_diff"), d));
                                ctx.request_repaint();
                            }
                        });
                    }
                });
            });
        });
}

fn empty_tab_note(ui: &mut Ui, text: &str) {
    ui.vertical_centered(|ui| {
        ui.add_space(60.0);
        ui.label(RichText::new(text).color(TX2).size(SZ_SM));
    });
}

// contract_block renders one labelled, boxed text section in the Contract tab.
fn contract_block(ui: &mut Ui, lbl: &str, content: &str, col: egui::Color32) {
    ui.label(RichText::new(lbl).color(TX3).size(8.5).monospace());
    ui.add_space(2.0);
    Frame::none()
        .fill(BG3)
        .stroke(Stroke::new(1.0, BORDER0))
        .rounding(R_SM)
        .inner_margin(egui::Margin::same(SP2))
        .show(ui, |ui| {
            ui.label(RichText::new(content).color(col).size(SZ_XS).monospace());
        });
    ui.add_space(SP2);
}

// ═══════════════════════════════════════════════════════════════════════════
// GRAPH PAGE
// ═══════════════════════════════════════════════════════════════════════════

#[allow(clippy::too_many_arguments)]
fn draw_graph_page(
    ui: &mut Ui,
    state: &DashboardState,
    graph_pos: &HashMap<String, Vec2>,
    graph_pan: &mut Vec2,
    graph_scale: &mut f32,
    active_project: &mut Option<String>,
    project_graph_nodes: &[GraphNode],
    project_graph_edges: &[GraphEdge],
) {
    // Source: a selected project's graph if one is active, else the live session's
    // global knowledge graph. Falling back to the global graph means the page is
    // never blank when there's data — it shows the session graph + controls.
    let (nodes, edges, title): (&[GraphNode], &[GraphEdge], String) = if let Some(proj) =
        active_project
    {
        (
            project_graph_nodes,
            project_graph_edges,
            format!("Knowledge Graph ({})", proj),
        )
    } else if !state.graph_nodes.is_empty() {
        (
            state.graph_nodes.as_slice(),
            state.graph_edges.as_slice(),
            "Knowledge Graph (session)".to_string(),
        )
    } else {
        page_header(ui, "Knowledge Graph", "scroll=zoom · drag=pan");
        empty_tab_note(ui, "No graph yet. Start a task, or pick a project in Projects, to populate the knowledge graph.");
        return;
    };
    let subtitle = format!(
        "{} nodes · {} edges · scroll=zoom · drag=pan",
        nodes.len(),
        edges.len()
    );
    page_header(ui, &title, &subtitle);
    let (project_graph_nodes, project_graph_edges) = (nodes, edges);

    // Legend bar
    Frame::none()
        .fill(BG2)
        .stroke(Stroke::new(1.0, BORDER0))
        .inner_margin(egui::Margin::symmetric(SP5, 5.0))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                for (t, c) in NODE_TYPE_LEGEND {
                    dot(ui, **c, 5.0);
                    ui.add_space(2.0);
                    ui.label(RichText::new(*t).color(TX2).size(SZ_XS));
                    ui.add_space(SP2);
                }
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    ui.label(
                        RichText::new("scroll to zoom · drag to pan")
                            .color(TX2)
                            .size(SZ_XS),
                    );
                });
            });
        });

    let canvas_size = ui.available_size();
    let (canvas_rect, resp) = ui.allocate_exact_size(canvas_size, Sense::click_and_drag());

    if resp.dragged() {
        *graph_pan += resp.drag_delta();
    }
    let scroll = ui.input(|i| i.smooth_scroll_delta.y);
    if scroll != 0.0 {
        *graph_scale = (*graph_scale * (1.0 + scroll * 0.002)).clamp(0.15, 4.0);
    }
    if resp.double_clicked() {
        *graph_pan = Vec2::ZERO;
        *graph_scale = 1.0;
    }

    draw_graph_obsidian(
        ui.painter(),
        canvas_rect,
        project_graph_nodes,
        project_graph_edges,
        graph_pos,
        *graph_pan,
        *graph_scale,
    );
}

const NODE_TYPE_LEGEND: &[(&str, &Color32)] = &[
    ("decision", &ACCENT),
    ("constraint", &RED),
    ("file", &BLUE),
    ("do_not_redo", &YELLOW),
    ("acceptance", &GREEN),
    ("tool_use", &TX1),
    ("tool_result", &TX2),
];

fn node_type_color(t: &str) -> Color32 {
    match t {
        "decision" => ACCENT,
        "constraint" => RED,
        "file" => BLUE,
        "do_not_redo" => YELLOW,
        "acceptance" => GREEN,
        "tool_use" => TX1,
        "tool_result" => TX2,
        _ => BORDER2,
    }
}

// fallback_node_pos scatters node i in a deterministic phyllotaxis (sunflower)
// pattern around the origin, so the graph renders evenly even before/without the
// force-layout simulation.
fn fallback_node_pos(i: usize) -> Vec2 {
    let golden = 2.399963_f32; // golden angle (radians)
    let a = i as f32 * golden;
    let r = 16.0 * (i as f32).sqrt();
    Vec2::new(r * a.cos(), r * a.sin())
}

// short_graph_label trims a fully-qualified node id down to its leaf so labels
// stay legible: "symbol:packages/daemon-go/internal/redact/redact.go:Scrub" → "Scrub",
// "module:packages/daemon-go/internal/redact/redact.go" → "redact.go".
fn short_graph_label(raw: &str) -> String {
    let body = raw
        .strip_prefix("symbol:")
        .or_else(|| raw.strip_prefix("module:"))
        .unwrap_or(raw);
    let leaf = body.rsplit(':').next().unwrap_or(body);
    let leaf = leaf.rsplit(['/', '\\']).next().unwrap_or(leaf).trim();
    if leaf.chars().count() > 28 {
        format!("{}…", leaf.chars().take(27).collect::<String>())
    } else {
        leaf.to_string()
    }
}

fn draw_graph_obsidian(
    painter: &egui::Painter,
    rect: egui::Rect,
    nodes: &[GraphNode],
    edges: &[GraphEdge],
    pos: &HashMap<String, Vec2>,
    pan: Vec2,
    scale: f32,
) {
    painter.rect_filled(rect, Rounding::ZERO, Color32::from_rgb(0x06, 0x06, 0x08));

    // Grid
    let grid_step = 40.0 * scale;
    let grid_color = Color32::from_rgba_premultiplied(8, 8, 10, 70);
    if grid_step > 8.0 {
        let ox = rect.left() + ((pan.x % grid_step) + grid_step) % grid_step;
        let oy = rect.top() + ((pan.y % grid_step) + grid_step) % grid_step;
        let mut x = ox;
        while x < rect.right() {
            painter.line_segment(
                [egui::pos2(x, rect.top()), egui::pos2(x, rect.bottom())],
                Stroke::new(0.5, grid_color),
            );
            x += grid_step;
        }
        let mut y = oy;
        while y < rect.bottom() {
            painter.line_segment(
                [egui::pos2(rect.left(), y), egui::pos2(rect.right(), y)],
                Stroke::new(0.5, grid_color),
            );
            y += grid_step;
        }
    }

    if nodes.is_empty() {
        painter.text(
            rect.center(),
            egui::Align2::CENTER_CENTER,
            "No graph data — run a task to build the knowledge graph",
            egui::FontId::new(SZ_XS, egui::FontFamily::Proportional),
            TX2,
        );
        return;
    }

    let cx = rect.center().x + pan.x;
    let cy = rect.center().y + pan.y;
    let to_screen = |w: Vec2| egui::pos2(cx + w.x * scale, cy + w.y * scale);

    let mut node_colors = HashMap::new();
    for node in nodes {
        node_colors.insert(node.id.as_str(), node_type_color(&node.node_type));
    }

    // Resolve a position for every node: the live force-layout position if present,
    // else a deterministic phyllotaxis fallback. Without this the Graph page is
    // blank whenever the layout sim hasn't run (it only runs on the Dashboard).
    let mut node_pos: HashMap<&str, Vec2> = HashMap::with_capacity(nodes.len());
    for (i, node) in nodes.iter().enumerate() {
        let p = pos
            .get(&node.id)
            .copied()
            .unwrap_or_else(|| fallback_node_pos(i));
        node_pos.insert(node.id.as_str(), p);
    }

    for edge in edges {
        let (Some(&pi), Some(&pj)) = (
            node_pos.get(edge.from_id.as_str()),
            node_pos.get(edge.to_id.as_str()),
        ) else {
            continue;
        };
        let (a, b) = (to_screen(pi), to_screen(pj));
        if rect.contains(a) || rect.contains(b) {
            let base_col = node_colors
                .get(edge.from_id.as_str())
                .copied()
                .unwrap_or(Color32::GRAY);
            let edge_col = Color32::from_rgba_premultiplied(
                base_col.r() / 2,
                base_col.g() / 2,
                base_col.b() / 2,
                70,
            );
            painter.line_segment([a, b], Stroke::new(1.2 * scale.sqrt().max(0.5), edge_col));
        }
    }

    for node in nodes {
        let p = node_pos.get(node.id.as_str()).copied().unwrap_or_default();
        let s = to_screen(p);
        if !rect.expand(40.0).contains(s) {
            continue;
        }
        let color = node_colors
            .get(node.id.as_str())
            .copied()
            .unwrap_or(Color32::GRAY);
        let radius = ((4.0 + node.weight * 2.5) * scale.sqrt()).max(2.5);

        // Obsidian-style multi-layer glow
        painter.circle_filled(
            s,
            radius * 3.5,
            Color32::from_rgba_premultiplied(color.r(), color.g(), color.b(), 12),
        );
        painter.circle_filled(
            s,
            radius * 1.8,
            Color32::from_rgba_premultiplied(color.r(), color.g(), color.b(), 40),
        );

        // Solid core
        painter.circle_filled(
            s,
            radius,
            Color32::from_rgba_premultiplied(color.r(), color.g(), color.b(), 240),
        );
        painter.circle_stroke(
            s,
            radius,
            Stroke::new(1.0, Color32::WHITE.linear_multiply(0.3)),
        );

        // Labels: drawing all of them at once turns a dense graph into an
        // unreadable wall of overlapping text. Show every label only for small
        // graphs; for large ones surface labels for prominent (high-weight)
        // nodes, and reveal the rest progressively as the user zooms in.
        let show_all = nodes.len() <= 40;
        let label_this = (show_all && scale > 0.6) || scale >= 1.3 || node.weight >= 0.9;
        if label_this {
            let text = short_graph_label(node.label.as_deref().unwrap_or(&node.id));
            painter.text(
                egui::pos2(s.x, s.y + radius + 4.0 * scale),
                egui::Align2::CENTER_TOP,
                text,
                egui::FontId::new(
                    (SZ_XS - 1.0) * scale.min(1.4),
                    egui::FontFamily::Proportional,
                ),
                TX1,
            );
        }
    }
}

// ═══════════════════════════════════════════════════════════════════════════
// PROFILES PAGE
// ═══════════════════════════════════════════════════════════════════════════

fn draw_profiles_page_live(ui: &mut Ui, state: &DashboardState) {
    page_header(
        ui,
        "Agent Profiles",
        "Route task kinds to provider chains. Reorder = handoff priority.",
    );
    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0).inner_margin(egui::Margin::symmetric(SP4, SP3)))
        .show_inside(ui, |ui| {
            ScrollArea::vertical()
                .id_salt("profiles_scroll")
                .auto_shrink([false, false])
                .drag_to_scroll(true)
                .show(ui, |ui| {
                    if state.profiles.is_empty() {
                        empty_tab_note(ui, "No profiles defined. Profiles will appear after `relay init` writes the default config.");
                        return;
                    }

                    // Summary row
                    ui.horizontal(|ui| {
                        ui.label(RichText::new(state.profiles.len().to_string())
                            .color(ACCENT).size(28.0).strong().monospace());
                        ui.add_space(2.0);
                        ui.vertical(|ui| {
                            ui.add_space(SP1);
                            ui.label(RichText::new("profiles defined").color(TX1).size(SZ_XS));
                            let providers: Vec<&str> = state.provider_details.iter()
                                .filter(|p| p.probe_status == "available")
                                .map(|p| p.name.as_str()).collect();
                            ui.label(RichText::new(format!("{} providers available", providers.len()))
                                .color(TX3).size(9.5));
                        });
                        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                            if btn_primary_with_icon(ui, "plus", "New profile").clicked() {
                                let p = Profile {
                                    name: format!("profile-{}", state.profiles.len() + 1),
                                    chain: vec![],
                                    kinds: vec![],
                                    skills: vec![],
                                    context_hint: String::new(),
                                };
                                crate::api::send_update_profile(p);
                            }
                        });
                    });
                    ui.add_space(SP3);
                    h_rule(ui);
                    ui.add_space(SP3);

                    let available_providers: Vec<String> = state.provider_details.iter()
                        .map(|p| p.name.clone()).collect();

                    for p in &state.profiles {
                        profile_row(ui, p, &available_providers);
                        ui.add_space(SP2);
                    }
                });
        });
}

/// Single profile editor row. Chain reorders via ▲▼ buttons. Add provider from
/// a dropdown of available providers. Kinds + skills + context inline.
fn profile_row(ui: &mut Ui, p: &Profile, available_providers: &[String]) {
    Frame::none()
        .fill(BG3)
        .stroke(Stroke::new(1.0, BORDER1))
        .rounding(R_LG)
        .inner_margin(egui::Margin::same(SP3))
        .show(ui, |ui| {
            // Header: profile name + delete button
            ui.horizontal(|ui| {
                ui.label(
                    RichText::new(&p.name)
                        .color(TX0)
                        .size(SZ_MD)
                        .strong()
                        .monospace(),
                );
                ui.label(
                    RichText::new(format!("· {} step chain", p.chain.len()))
                        .color(TX3)
                        .size(SZ_XS),
                );
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if btn(ui, "Delete").clicked() {
                        crate::api::send_delete_profile(p.name.clone());
                    }
                });
            });

            if !p.context_hint.is_empty() {
                ui.label(RichText::new(&p.context_hint).color(TX2).size(SZ_XS));
            }
            ui.add_space(SP2);

            // ── Chain editor — reorderable list ────────────────────────
            ui.label(
                RichText::new("HANDOFF CHAIN")
                    .color(TX3)
                    .size(8.5)
                    .monospace(),
            );
            ui.add_space(SP1);

            if p.chain.is_empty() {
                ui.label(
                    RichText::new("(empty — add a provider below)")
                        .color(TX3)
                        .size(SZ_XS)
                        .italics(),
                );
            } else {
                let mut new_chain = p.chain.clone();
                let mut changed = false;
                let mut remove_idx: Option<usize> = None;

                for (i, prov) in p.chain.iter().enumerate() {
                    ui.horizontal(|ui| {
                        ui.label(
                            RichText::new(format!("{}.", i + 1))
                                .color(TX3)
                                .size(SZ_XS)
                                .monospace(),
                        );
                        // Provider name
                        let prov_col = if available_providers.contains(prov) {
                            TX0
                        } else {
                            TX2
                        };
                        ui.label(RichText::new(prov).color(prov_col).size(SZ_XS).monospace());
                        if !available_providers.contains(prov) {
                            ui.label(RichText::new("(unknown)").color(TX3).size(9.5));
                        }

                        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                            if icon_btn(ui, "x", "Remove").clicked() {
                                remove_idx = Some(i);
                            }
                            ui.add_space(SP1);
                            if i < p.chain.len() - 1 && icon_btn(ui, "down", "Move down").clicked()
                            {
                                new_chain.swap(i, i + 1);
                                changed = true;
                            }
                            if i > 0 && icon_btn(ui, "up", "Move up").clicked() {
                                new_chain.swap(i, i - 1);
                                changed = true;
                            }
                        });
                    });
                }

                if let Some(idx) = remove_idx {
                    new_chain.remove(idx);
                    changed = true;
                }

                if changed {
                    let mut updated = p.clone();
                    updated.chain = new_chain;
                    crate::api::send_update_profile(updated);
                }
            }

            // Add-provider dropdown row
            ui.add_space(SP1);
            ui.horizontal(|ui| {
                let id = egui::Id::new(("add_prov", &p.name));
                let mut open = ui
                    .ctx()
                    .data_mut(|m| *m.get_temp_mut_or_default::<bool>(id));
                let clicked = if open {
                    btn_with_icon(ui, "x", "Cancel").clicked()
                } else {
                    btn_with_icon(ui, "plus", "Add provider").clicked()
                };
                if clicked {
                    open = !open;
                    ui.ctx().data_mut(|m| m.insert_temp(id, open));
                }
                if open {
                    for prov_name in available_providers {
                        if p.chain.contains(prov_name) {
                            continue;
                        }
                        if btn(ui, prov_name).clicked() {
                            let mut updated = p.clone();
                            updated.chain.push(prov_name.clone());
                            crate::api::send_update_profile(updated);
                            ui.ctx().data_mut(|m| m.insert_temp(id, false));
                        }
                    }
                }
            });

            // ── Kinds / skills (read-only display for now) ─────────────
            if !p.kinds.is_empty() {
                ui.add_space(SP2);
                ui.horizontal_wrapped(|ui| {
                    ui.label(RichText::new("KINDS").color(TX3).size(8.5).monospace());
                    ui.add_space(SP1);
                    for k in &p.kinds {
                        Frame::none()
                            .fill(ACCENT_BG)
                            .rounding(R_PILL)
                            .inner_margin(egui::Margin::symmetric(SP2, 1.5))
                            .show(ui, |ui| {
                                ui.label(RichText::new(k).color(ACCENT).size(9.5).monospace());
                            });
                    }
                });
            }
            if !p.skills.is_empty() {
                ui.add_space(SP1);
                ui.horizontal_wrapped(|ui| {
                    ui.label(RichText::new("SKILLS").color(TX3).size(8.5).monospace());
                    ui.add_space(SP1);
                    for s in &p.skills {
                        Frame::none()
                            .fill(BG4)
                            .rounding(R_PILL)
                            .inner_margin(egui::Margin::symmetric(SP2, 1.5))
                            .show(ui, |ui| {
                                ui.label(RichText::new(s).color(TX1).size(9.5).monospace());
                            });
                    }
                });
            }
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// AUDIT PAGE
// ═══════════════════════════════════════════════════════════════════════════

fn draw_audit_page(ui: &mut Ui, state: &DashboardState) {
    page_header(ui, "Audit Log", "Hash-chained JSONL · HMAC-SHA256 · SEC-10");
    egui::CentralPanel::default()
        .frame(
            Frame::none()
                .fill(BG0)
                .inner_margin(egui::Margin::symmetric(SP5, SP3)),
        )
        .show_inside(ui, |ui| {
            ui.horizontal(|ui| {
                ui.label(
                    RichText::new("relay audit verify")
                        .color(ACCENT)
                        .size(SZ_XS)
                        .monospace(),
                );
                ui.add_space(SP2);
                ui.label(
                    RichText::new("— run in terminal to verify hash chain integrity")
                        .color(TX2)
                        .size(SZ_XS),
                );
            });
            ui.add_space(SP3);
            h_rule(ui);
            ui.add_space(SP3);
            ui.label(
                RichText::new("RECENT EVENTS")
                    .color(TX3)
                    .size(9.0)
                    .monospace()
                    .strong(),
            );
            ui.add_space(SP2);
            ScrollArea::vertical()
                .id_salt("audit_scroll")
                .stick_to_bottom(true)
                .show(ui, |ui| {
                    for ev in state.events.iter().rev().take(200).rev() {
                        ui.horizontal(|ui| {
                            ui.add_sized(
                                [50.0, SZ_XS + 2.0],
                                egui::Label::new(
                                    RichText::new(&ev.ts).color(TX3).size(SZ_XS).monospace(),
                                ),
                            );
                            let (bg, fg, label) = tag_style(&ev.tag);
                            Frame::none()
                                .fill(bg)
                                .rounding(R_PILL)
                                .inner_margin(egui::Margin::symmetric(6.0, 2.0))
                                .show(ui, |ui| {
                                    ui.label(
                                        RichText::new(label)
                                            .color(fg)
                                            .size(9.0)
                                            .strong()
                                            .monospace(),
                                    );
                                });
                            ui.label(RichText::new(&ev.msg).color(TX1).size(SZ_XS).monospace());
                        });
                    }
                });
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// DETECTED AGENTS PAGE — find agents already running, port their work
// ═══════════════════════════════════════════════════════════════════════════

fn draw_detect_page(ui: &mut Ui, state: &DashboardState) {
    page_header(
        ui,
        "Detected Agents",
        "Agents already running on this machine · adopt to port their work",
    );

    let cache_id = egui::Id::new("detect_cache"); // String: "OK::<json>" | "ERR::<msg>"
    let loading_id = egui::Id::new("detect_loading"); // bool
    let since_id = egui::Id::new("detect_since_hours"); // i64; 0/24 = default day

    let cached: Option<String> = ui.ctx().data_mut(|m| m.get_temp::<String>(cache_id));
    let loading: bool = ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(loading_id));
    let since: i64 = ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_insert_with::<i64>(since_id, || 24));

    // Auto-scan once on first open (detection shells out to the OS, so it is
    // on-demand rather than part of the 1.5s poll loop).
    if cached.is_none() && !loading {
        trigger_detect_scan(ui, cache_id, loading_id, since);
    }

    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0).inner_margin(egui::Margin::symmetric(SP5, SP3)))
        .show_inside(ui, |ui| {
            ui.horizontal(|ui| {
                let label = if loading { "Scanning…" } else { "Rescan" };
                if btn_with_icon(ui, "detect", label).clicked() && !loading {
                    trigger_detect_scan(ui, cache_id, loading_id, since);
                }
                ui.add_space(SP3);
                ui.label(RichText::new("active within").color(TX2).size(SZ_XS));
                // Recency window: widen to surface sessions paused more than a day ago.
                for (lbl, hrs) in [("24h", 24_i64), ("7d", 168), ("30d", 720), ("all", 200_000)] {
                    if chip_select(ui, lbl, since == hrs).clicked() {
                        ui.ctx().data_mut(|m| m.insert_temp(since_id, hrs));
                        ui.ctx().data_mut(|m| m.remove::<String>(cache_id)); // show "scanning" while refetching
                        trigger_detect_scan(ui, cache_id, loading_id, hrs);
                    }
                }
            });
            ui.add_space(SP1);
            ui.label(RichText::new("Reads running processes + on-disk session transcripts. Nothing is launched.")
                .color(TX2).size(SZ_XS));
            ui.add_space(SP3);
            h_rule(ui);
            ui.add_space(SP3);

            match cached.as_deref() {
                Some(s) if s.starts_with("OK::") => match serde_json::from_str::<Vec<DetectedAgent>>(&s[4..]) {
                    Ok(agents) if agents.is_empty() => {
                        empty_note(ui, "No agents detected. Start Claude Code, Codex, Ollama, etc. then press Rescan.");
                    }
                    Ok(agents) => {
                        let n = agents.len();
                        ui.label(RichText::new(format!("{} agent{} found", n, if n == 1 { "" } else { "s" }))
                            .color(TX3).size(9.0).monospace().strong());
                        ui.add_space(SP2);
                        ScrollArea::vertical().id_salt("detect_scroll").show(ui, |ui| {
                            for a in &agents {
                                draw_agent_card(ui, a, state);
                                ui.add_space(SP3);
                            }
                        });
                    }
                    Err(e) => empty_note(ui, &format!("Could not parse scan result: {}", e)),
                },
                Some(s) if s.starts_with("ERR::") => {
                    empty_note(ui, &format!("Scan failed: {} (is the daemon running?)", &s[5..]));
                }
                _ => {
                    ui.label(RichText::new("Scanning your machine…").color(TX2).size(SZ_XS));
                }
            }
        });
}

/// Kick off an async /api/detect scan, stashing the result in egui temp memory.
fn trigger_detect_scan(ui: &Ui, cache_id: egui::Id, loading_id: egui::Id, since_hours: i64) {
    ui.ctx().data_mut(|m| m.insert_temp(loading_id, true));
    let ctx_clone = ui.ctx().clone();
    let (tx, rx) = std::sync::mpsc::channel();
    crate::api::send_detect_scan(since_hours, tx);
    std::thread::spawn(move || {
        if let Ok(result) = rx.recv() {
            let stored = match result {
                Ok(agents) => format!("OK::{}", serde_json::to_string(&agents).unwrap_or_default()),
                Err(e) => format!("ERR::{}", e),
            };
            ctx_clone.data_mut(|m| m.insert_temp(cache_id, stored));
            ctx_clone.data_mut(|m| m.insert_temp(loading_id, false));
            ctx_clone.request_repaint();
        }
    });
}

fn draw_agent_card(ui: &mut Ui, a: &DetectedAgent, state: &DashboardState) {
    Frame::none()
        .fill(BG1)
        .stroke(Stroke::new(1.0, BORDER1))
        .rounding(R_SM)
        .inner_margin(egui::Margin::same(SP3))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                let (col, status) = if a.running && a.pid > 0 {
                    (GREEN, format!("running · pid {}", a.pid))
                } else if a.running {
                    (GREEN, "active recently".to_string())
                } else if a.session.is_some() {
                    (YELLOW, "idle session".to_string())
                } else {
                    (TX3, "process only".to_string())
                };
                dot(ui, col, 6.0);
                ui.add_space(SP1);
                ui.label(
                    RichText::new(&a.display_name)
                        .color(TX0)
                        .size(13.0)
                        .strong(),
                );
                ui.label(
                    RichText::new(format!("· {}", status))
                        .color(col)
                        .size(SZ_XS)
                        .monospace(),
                );
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if !a.surface.is_empty() {
                        ui.label(RichText::new(&a.surface).color(TX3).size(9.0).monospace());
                    }
                });
            });
            if !a.work_dir.is_empty() {
                ui.label(
                    RichText::new(&a.work_dir)
                        .color(TX2)
                        .size(SZ_XS)
                        .monospace(),
                );
            }

            if let Some(s) = &a.session {
                ui.add_space(SP2);
                let goal = if !s.initial_prompt.is_empty() {
                    &s.initial_prompt
                } else {
                    &s.last_prompt
                };
                if !goal.is_empty() {
                    field_label(ui, "GOAL");
                    ui.label(RichText::new(goal).color(TX1).size(SZ_XS));
                }
                if !s.tasks_remaining.is_empty() {
                    ui.add_space(SP1);
                    field_label(ui, &format!("REMAINING ({})", s.tasks_remaining.len()));
                    for t in s.tasks_remaining.iter().take(8) {
                        ui.label(RichText::new(format!("   · {}", t)).color(TX1).size(SZ_XS));
                    }
                }
                ui.add_space(SP2);
                ui.horizontal_wrapped(|ui| {
                    meta_chip(ui, &format!("{} msgs", s.message_count));
                    meta_chip(
                        ui,
                        &format!("{} in / {} out tok", s.tokens_in, s.tokens_out),
                    );
                    if !s.model.is_empty() {
                        meta_chip(ui, &s.model);
                    }
                    if !s.files_touched.is_empty() {
                        meta_chip(ui, &format!("{} files", s.files_touched.len()));
                    }
                    for sk in s.skills.iter().take(6) {
                        meta_chip(ui, sk);
                    }
                    for mc in s.mcps.iter().take(6) {
                        meta_chip(ui, &format!("mcp:{}", mc));
                    }
                });
            } else if !a.cmdline.is_empty() {
                ui.add_space(SP1);
                ui.label(RichText::new(&a.cmdline).color(TX3).size(9.5).monospace());
            }

            ui.add_space(SP2);
            h_rule(ui);
            ui.add_space(SP2);
            draw_adopt_row(ui, a, state);
        });
}

fn draw_adopt_row(ui: &mut Ui, a: &DetectedAgent, state: &DashboardState) {
    // Candidate targets: enabled providers other than the source's own.
    let mut targets: Vec<String> = state
        .provider_details
        .iter()
        .filter(|p| p.enabled && p.name != a.provider)
        .map(|p| p.name.clone())
        .collect();
    if targets.is_empty() {
        targets = ["claude", "codex", "ollama", "opencode", "copilot"]
            .iter()
            .filter(|n| **n != a.provider)
            .map(|s| s.to_string())
            .collect();
    }

    let target_id = egui::Id::new(("adopt_target", a.id.as_str()));
    let mut target: String = ui.ctx().data_mut(|m| {
        m.get_temp_mut_or_insert_with::<String>(target_id, || {
            targets.first().cloned().unwrap_or_default()
        })
        .clone()
    });

    ui.horizontal_wrapped(|ui| {
        ui.label(RichText::new("Port to").color(TX2).size(SZ_XS));
        for t in &targets {
            if chip_select(ui, t, &target == t).clicked() {
                target = t.clone();
                ui.ctx()
                    .data_mut(|m| m.insert_temp(target_id, target.clone()));
            }
        }
    });

    ui.add_space(SP2);
    let busy_id = egui::Id::new(("adopt_busy", a.id.as_str()));
    let result_id = egui::Id::new(("adopt_result", a.id.as_str()));
    let started_id = egui::Id::new(("adopt_started", a.id.as_str()));
    let busy: bool = ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(busy_id));

    // POST adopt then stash the brief (result_id) and, for an "Adopt & start",
    // the session-start status (started_id). `start` distinguishes the two buttons.
    let launch = |ui: &Ui, start: bool| {
        ui.ctx().data_mut(|m| m.insert_temp(busy_id, true));
        let ctx_clone = ui.ctx().clone();
        let (tx, rx) = std::sync::mpsc::channel();
        crate::api::send_adopt(a.id.clone(), target.clone(), start, tx);
        std::thread::spawn(move || {
            if let Ok(result) = rx.recv() {
                match result {
                    Ok(out) => {
                        let status = if out.started {
                            "started".to_string()
                        } else if let Some(e) = out.start_error {
                            format!("starterr::{}", e)
                        } else {
                            String::new()
                        };
                        ctx_clone.data_mut(|m| {
                            m.insert_temp(result_id, format!("OK::{}", out.markdown))
                        });
                        ctx_clone.data_mut(|m| m.insert_temp(started_id, status));
                    }
                    Err(e) => {
                        ctx_clone.data_mut(|m| m.insert_temp(result_id, format!("ERR::{}", e)));
                        ctx_clone.data_mut(|m| m.insert_temp(started_id, String::new()));
                    }
                }
                ctx_clone.data_mut(|m| m.insert_temp(busy_id, false));
                ctx_clone.request_repaint();
            }
        });
    };

    ui.horizontal(|ui| {
        let plabel = if busy {
            "Working…".to_string()
        } else {
            format!(
                "Adopt & start in {} →",
                if target.is_empty() {
                    "agent"
                } else {
                    target.as_str()
                }
            )
        };
        if btn_primary(ui, &plabel).clicked() && !busy && !target.is_empty() {
            launch(ui, true);
        }
        ui.add_space(SP2);
        if chip_select(ui, "Brief only", false).clicked() && !busy {
            launch(ui, false);
        }
    });
    ui.add_space(SP1);
    ui.label(RichText::new("Adopt & start writes a brief to .relay/adopted/ AND launches a Relay session on the target. Brief only just stages the brief — nothing runs.")
        .color(TX3).size(9.0));

    if let Some(stored) = ui.ctx().data_mut(|m| m.get_temp::<String>(result_id)) {
        ui.add_space(SP2);
        // Session-start status (present only after an "Adopt & start").
        let status = ui
            .ctx()
            .data_mut(|m| m.get_temp::<String>(started_id))
            .unwrap_or_default();
        if status == "started" {
            ui.horizontal(|ui| {
                dot(ui, BLUE, 5.0);
                ui.label(
                    RichText::new("session started — open the Dashboard to watch it run")
                        .color(BLUE)
                        .size(SZ_XS)
                        .monospace()
                        .strong(),
                );
            });
            ui.add_space(SP1);
        } else if let Some(msg) = status.strip_prefix("starterr::") {
            ui.horizontal_wrapped(|ui| {
                dot(ui, YELLOW, 5.0);
                ui.label(
                    RichText::new(format!("brief saved, but session did not start: {}", msg))
                        .color(YELLOW)
                        .size(SZ_XS)
                        .monospace(),
                );
            });
            ui.add_space(SP1);
        }
        if let Some(md) = stored.strip_prefix("OK::") {
            Frame::none()
                .fill(BG3)
                .stroke(Stroke::new(1.0, GREEN))
                .rounding(R_SM)
                .inner_margin(egui::Margin::same(SP3))
                .show(ui, |ui| {
                    ui.horizontal(|ui| {
                        dot(ui, GREEN, 5.0);
                        ui.label(
                            RichText::new("brief staged for handoff")
                                .color(GREEN)
                                .size(SZ_XS)
                                .monospace()
                                .strong(),
                        );
                    });
                    ui.add_space(SP1);
                    ScrollArea::vertical()
                        .id_salt(egui::Id::new(("brief", a.id.as_str())))
                        .max_height(180.0)
                        .show(ui, |ui| {
                            ui.label(RichText::new(md).color(TX1).size(9.5).monospace());
                        });
                });
        } else if let Some(err) = stored.strip_prefix("ERR::") {
            Frame::none()
                .fill(BG3)
                .stroke(Stroke::new(1.0, RED))
                .rounding(R_SM)
                .inner_margin(egui::Margin::same(SP3))
                .show(ui, |ui| {
                    ui.label(RichText::new(err).color(RED).size(SZ_XS).monospace());
                });
        }
    }
}

fn field_label(ui: &mut Ui, s: &str) {
    ui.label(RichText::new(s).color(TX3).size(8.5).monospace().strong());
}

fn meta_chip(ui: &mut Ui, s: &str) {
    Frame::none()
        .fill(BG3)
        .rounding(R_PILL)
        .inner_margin(egui::Margin::symmetric(6.0, 2.0))
        .show(ui, |ui| {
            ui.label(RichText::new(s).color(TX2).size(9.0).monospace());
        });
}

fn empty_note(ui: &mut Ui, s: &str) {
    ui.add_space(SP3);
    ui.label(RichText::new(s).color(TX2).size(SZ_XS));
}

/// Selectable chip (provider target picker). Accent fill when selected.
fn chip_select(ui: &mut Ui, label: &str, selected: bool) -> egui::Response {
    let font = egui::FontId::new(SZ_XS, egui::FontFamily::Monospace);
    let galley = ui
        .painter()
        .layout_no_wrap(label.to_owned(), font.clone(), TX0);
    let desired = galley.size() + Vec2::new(SP2 * 2.0, 5.0);
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
    if ui.is_rect_visible(rect) {
        let (fill, border, fg) = if selected {
            (ACCENT, ACCENT, Color32::from_rgb(0x0a, 0x0a, 0x0a))
        } else if resp.hovered() {
            (BTN_HOVER, BORDER2, TX0)
        } else {
            (BTN_BG, BORDER1, TX1)
        };
        ui.painter().rect_filled(rect, R, fill);
        ui.painter().rect_stroke(rect, R, Stroke::new(1.0, border));
        ui.painter()
            .text(rect.center(), egui::Align2::CENTER_CENTER, label, font, fg);
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
    resp
}

// ═══════════════════════════════════════════════════════════════════════════
// PROJECTS PAGE
// ═══════════════════════════════════════════════════════════════════════════

fn draw_projects_page(
    ui: &mut Ui,
    state: &DashboardState,
    active_project: &mut Option<String>,
    project_task_text: &mut String,
    _new_task_open: &mut bool,
    nav_out: &mut Option<NavPage>,
) {
    page_header(
        ui,
        "Projects",
        "Open a project folder to launch tasks without the terminal",
    );

    egui::CentralPanel::default()
        .frame(
            Frame::none()
                .fill(BG0)
                .inner_margin(egui::Margin::symmetric(SP4, SP4)),
        )
        .show_inside(ui, |ui| {
            // If a project is selected: task input
            if let Some(proj) = active_project.clone() {
                ui.vertical_centered(|ui| {
                    ui.set_width(ui.available_width().min(540.0));
                    ui.add_space(SP4);
                    ui.label(RichText::new(&proj).color(ACCENT).size(SZ_XS).monospace());
                    ui.add_space(SP2);
                    ui.label(
                        RichText::new("What should the agent work on?")
                            .color(TX0)
                            .size(15.0)
                            .strong(),
                    );
                    ui.add_space(4.0);
                    ui.label(
                        RichText::new("Relay will launch claude and begin the session")
                            .color(TX2)
                            .size(SZ_XS),
                    );
                    ui.add_space(SP3);

                    let resp = ui.add(
                        egui::TextEdit::singleline(project_task_text)
                            .desired_width(520.0)
                            .hint_text("e.g. Add refund flow to orders service")
                            .font(egui::FontId::new(SZ_MD, egui::FontFamily::Proportional)),
                    );
                    let can_start = !project_task_text.trim().is_empty();
                    if resp.lost_focus()
                        && ui.input(|i| i.key_pressed(egui::Key::Enter))
                        && can_start
                    {
                        crate::api::send_run_task(project_task_text.trim().to_string());
                        project_task_text.clear();
                        *nav_out = Some(NavPage::Dashboard);
                    }
                    ui.add_space(SP2);
                    ui.label(
                        RichText::new(format!(
                            "relay run \"{}\"",
                            if project_task_text.is_empty() {
                                "…"
                            } else {
                                project_task_text.as_str()
                            }
                        ))
                        .color(TX3)
                        .size(9.5)
                        .monospace(),
                    );
                    ui.add_space(SP3);
                    ui.horizontal(|ui| {
                        let start = btn_primary(ui, "Start task");
                        if start.clicked() && can_start {
                            crate::api::send_run_task(project_task_text.trim().to_string());
                            project_task_text.clear();
                            *nav_out = Some(NavPage::Dashboard);
                        }
                        ui.add_space(SP2);
                        if btn(ui, "Change project").clicked() {
                            *active_project = None;
                        }
                    });
                });
                return;
            }

            // No project selected
            ui.vertical_centered(|ui| {
                ui.set_width(ui.available_width().min(400.0));
                ui.add_space(SP4);
                ui.label(
                    RichText::new("No active session")
                        .color(TX1)
                        .size(15.0)
                        .strong(),
                );
                ui.add_space(SP2);
                ui.label(
                    RichText::new("Open a project folder to get started")
                        .color(TX2)
                        .size(SZ_XS),
                );
                ui.add_space(SP4);

                // Open project button — styled like design's "Open project folder" btn
                let (rect, resp) = ui.allocate_exact_size(Vec2::new(288.0, 48.0), Sense::click());
                let fill = if resp.hovered() { BTN_HOVER } else { BTN_BG };
                ui.painter().rect_filled(rect, R_LG, fill);
                ui.painter().rect_stroke(
                    rect,
                    R_LG,
                    Stroke::new(1.0, if resp.hovered() { BORDER2 } else { BORDER1 }),
                );
                // Folder icon + text centered
                let icon_x = rect.center().x - 60.0;
                paint_icon(
                    ui.painter(),
                    egui::pos2(icon_x, rect.center().y),
                    14.0,
                    "folder",
                    TX1,
                );
                ui.painter().text(
                    egui::pos2(icon_x + 12.0, rect.center().y),
                    egui::Align2::LEFT_CENTER,
                    "Open project folder…",
                    egui::FontId::new(SZ_MD, egui::FontFamily::Proportional),
                    TX0,
                );
                if resp.hovered() {
                    ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                }
                if resp.clicked() {
                    if let Some(picked) = crate::api::pick_project_folder() {
                        *active_project = Some(picked);
                    }
                }

                ui.add_space(SP3);

                // Recent: show current session if any
                if let Some(s) = &state.session {
                    ui.label(
                        RichText::new("RECENT")
                            .color(TX3)
                            .size(9.0)
                            .monospace()
                            .strong(),
                    );
                    ui.add_space(SP2);
                    let (rect, resp) =
                        ui.allocate_exact_size(Vec2::new(300.0, 32.0), Sense::click());
                    let fill = if resp.hovered() {
                        BG3
                    } else {
                        Color32::TRANSPARENT
                    };
                    ui.painter().rect_filled(rect, R_SM, fill);
                    ui.painter().text(
                        egui::pos2(rect.left() + SP3, rect.center().y),
                        egui::Align2::LEFT_CENTER,
                        &s.task_goal,
                        egui::FontId::new(SZ_XS, egui::FontFamily::Proportional),
                        TX1,
                    );
                    ui.painter().text(
                        egui::pos2(rect.right() - SP3, rect.center().y),
                        egui::Align2::RIGHT_CENTER,
                        "now",
                        egui::FontId::new(9.5, egui::FontFamily::Monospace),
                        GREEN,
                    );
                    if resp.clicked() {
                        *nav_out = Some(NavPage::Dashboard);
                    }
                    if resp.hovered() {
                        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                    }
                    ui.add_space(SP3);
                }

                // Quick start
                ui.label(
                    RichText::new("QUICK START")
                        .color(TX3)
                        .size(9.0)
                        .monospace()
                        .strong(),
                );
                ui.add_space(SP2);
            });

            ui.vertical_centered(|ui| {
                ui.set_width(ui.available_width().min(480.0));
                if setup_step(
                    ui,
                    "1",
                    "relay init",
                    "relay init",
                    "Creates .relay/ directory, signing key, and audit log.",
                    Some("Run init"),
                ) {
                    crate::api::send_init();
                }
                ui.add_space(SP2);
                setup_step(
                    ui,
                    "2",
                    "Configure providers",
                    ".relay/relay.toml",
                    "Enable providers, set declared caps, choose handoff order.",
                    None,
                );
                ui.add_space(SP2);
                if setup_step(
                    ui,
                    "3",
                    "Open project + start task",
                    "Open above, then describe your task",
                    "Relay launches the agent and streams output here.",
                    None,
                ) {}
            });
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// SETTINGS PAGE
// ═══════════════════════════════════════════════════════════════════════════

fn draw_settings_page(
    ui: &mut Ui,
    state: &DashboardState,
    settings_tab: &mut SettingsTab,
    settings_hitl: &mut bool,
    settings_telem: &mut bool,
    settings_thresh: &mut u32,
) {
    page_header(ui, "Settings", "Daemon, providers, security");

    egui::CentralPanel::default()
        .frame(Frame::none().fill(BG0))
        .show_inside(ui, |ui| {
            // Left nav — must be at top level of show_inside, never inside horizontal_centered
            egui::SidePanel::left("settings_nav")
                .exact_width(148.0)
                .resizable(false)
                .frame(Frame::none().fill(BG1).inner_margin(egui::Margin::same(0.0)))
                .show_separator_line(true)
                .show_inside(ui, |ui| {
                    ui.add_space(SP2);
                    let tabs = [
                        (SettingsTab::General,   "General"),
                        (SettingsTab::Providers, "Providers"),
                        (SettingsTab::Vision,    "Vision"),
                        (SettingsTab::Security,  "Security"),
                        (SettingsTab::About,     "About"),
                    ];
                    for (tab, label) in &tabs {
                        let active = *settings_tab == *tab;
                        let desired = Vec2::new(ui.available_width(), 30.0);
                        let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
                        let fill = if active || resp.hovered() { BG2 } else { Color32::TRANSPARENT };
                        ui.painter().rect_filled(rect, Rounding::ZERO, fill);
                        if active {
                            let bar = Rect::from_min_size(rect.min, Vec2::new(2.0, rect.height()));
                            ui.painter().rect_filled(bar, Rounding::ZERO, ACCENT);
                        }
                        let col = if active { TX0 } else { TX1 };
                        ui.painter().text(egui::pos2(rect.left() + SP3, rect.center().y),
                            egui::Align2::LEFT_CENTER, label,
                            egui::FontId::new(SZ_SM, egui::FontFamily::Proportional), col);
                        if resp.hovered() { ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand); }
                        if resp.clicked() { *settings_tab = tab.clone(); }
                    }
                });

            // Content
            egui::CentralPanel::default()
                .frame(Frame::none().fill(BG0).inner_margin(egui::Margin::symmetric(SP4 + SP2, SP4)))
                .show_inside(ui, |ui| {
                    // Smooth scroll: enable wheel input + always-show velocity-tracked bar
                    ScrollArea::vertical()
                        .id_salt("settings_content")
                        .auto_shrink([false, false])
                        .drag_to_scroll(true)
                        .show(ui, |ui| {
                        match settings_tab {
                            SettingsTab::General => {
                                ui.label(RichText::new("Daemon and session defaults").color(TX2).size(SZ_XS));
                                ui.add_space(SP3);
                                srow(ui, "Daemon address", Some("Local HTTP endpoint"), |ui| {
                                    ui.label(RichText::new("http://localhost:4748").color(ACCENT).size(SZ_XS).monospace());
                                });
                                srow(ui, "Auto-handoff threshold",
                                    Some(&format!("Trigger handoff at {}% quota usage", settings_thresh)), |ui| {
                                    ui.label(RichText::new(format!("{}%", settings_thresh))
                                        .color(ACCENT).size(SZ_SM).monospace().strong());
                                    if icon_btn(ui, "minus", "Decrease").clicked() && *settings_thresh >= 55 {
                                        *settings_thresh -= 5;
                                    }
                                    if icon_btn(ui, "plus", "Increase").clicked() && *settings_thresh < 95 {
                                        *settings_thresh += 5;
                                    }
                                });
                                srow(ui, "Telemetry", Some("Anonymous usage events — opt-in only"), |ui| {
                                    toggle(ui, settings_telem);
                                });
                            }
                            SettingsTab::Providers => {
                                draw_providers_tab(ui, state);
                            }
                            SettingsTab::Vision => {
                                draw_vision_tab(ui, state);
                            }
                            SettingsTab::Security => {
                                ui.label(RichText::new("Access control, encryption, and data policy").color(TX2).size(SZ_XS));
                                ui.add_space(SP3);
                                srow(ui, "HITL gate", Some("Require confirmation before each handoff"), |ui| {
                                    toggle(ui, settings_hitl);
                                });
                                srow(ui, "Data isolation", Some("Never transmit one vendor's raw output to another"), |ui| {
                                    ui.label(RichText::new("enforced").color(GREEN).size(SZ_XS).monospace());
                                    dot(ui, GREEN, 4.0);
                                });
                                srow(ui, "At-rest encryption", Some("graph.db and snapshots — AES-256-GCM"), |ui| {
                                    ui.label(RichText::new("active").color(GREEN).size(SZ_XS).monospace());
                                    dot(ui, GREEN, 4.0);
                                });
                                srow(ui, "Audit log", Some(".relay/audit.jsonl · HMAC-SHA256 hash chain"), |ui| {
                                    ui.label(RichText::new("intact").color(GREEN).size(SZ_XS).monospace());
                                    dot(ui, GREEN, 4.0);
                                });
                            }
                            SettingsTab::About => {
                                ui.label(RichText::new("relay").color(TX0).size(17.0).monospace().strong());
                                ui.add_space(2.0);
                                ui.label(RichText::new("v0.3.0-alpha · Apache-2.0").color(TX2).size(SZ_XS).monospace());
                                ui.add_space(SP3);
                                ui.label(RichText::new(
                                    "A vendor-neutral orchestrator that keeps a coding session alive across multiple AI agents, handing work off from one model to the next before quotas run out. State and intent travel as a compact, signed continuation contract."
                                ).color(TX1).size(SZ_SM));
                                ui.add_space(SP4);
                                ui.horizontal(|ui| {
                                    let _ = btn(ui, "Changelog");
                                    let _ = btn(ui, "GitHub");
                                    let _ = btn(ui, "Docs");
                                });
                            }
                        }
                    });
                });
        });
}

// ─── Vision tab ─────────────────────────────────────────────────────────────

fn draw_vision_tab(ui: &mut Ui, state: &DashboardState) {
    let cfg = match &state.vision_config {
        Some(c) => c.clone(),
        None => {
            ui.label(
                RichText::new("Loading vision config…")
                    .color(TX2)
                    .size(SZ_XS),
            );
            return;
        }
    };

    // Summary header
    ui.horizontal(|ui| {
        ui.vertical(|ui| {
            ui.label(RichText::new("Vision fallback").color(TX0).size(SZ_MD).strong());
            ui.label(RichText::new(
                "Screenshot-based observation for IDE & extension providers that can't be hooked programmatically."
            ).color(TX2).size(SZ_XS));
        });
        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
            let mut enabled = cfg.enabled;
            toggle(ui, &mut enabled);
            if enabled != cfg.enabled {
                let mut new_cfg = cfg.clone();
                new_cfg.enabled = enabled;
                crate::api::send_update_vision_config(new_cfg);
            }
        });
    });

    ui.add_space(SP3);
    h_rule(ui);
    ui.add_space(SP3);

    // ── Provider picker ───────────────────────────────────────────────
    ui.label(
        RichText::new("VISION PROVIDER")
            .color(TX3)
            .size(8.5)
            .monospace(),
    );
    ui.add_space(SP1);

    let providers = [
        (
            "ollama",
            "Ollama (local)",
            "qwen2.5-vl:7b · llava · llama3.2-vision",
            None,
        ),
        (
            "gemini",
            "Google Gemini",
            "gemini-1.5-pro · gemini-2.0-flash",
            Some("GEMINI_API_KEY"),
        ),
        (
            "openai",
            "OpenAI",
            "gpt-4o · gpt-4o-mini",
            Some("OPENAI_API_KEY"),
        ),
        (
            "anthropic",
            "Anthropic Claude",
            "claude-3-5-sonnet · claude-opus-4",
            Some("ANTHROPIC_API_KEY"),
        ),
    ];

    for (key, label, models, env_var) in providers {
        let selected = cfg.provider == key;
        let desired = Vec2::new(ui.available_width(), 38.0);
        let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
        let fill = if selected {
            ACCENT_BG
        } else if resp.hovered() {
            BG3
        } else {
            Color32::TRANSPARENT
        };
        let border_col = if selected { ACCENT } else { BORDER0 };
        ui.painter().rect_filled(rect, R_SM, fill);
        ui.painter()
            .rect_stroke(rect, R_SM, Stroke::new(1.0, border_col));

        let lx = rect.left() + SP3;
        let ly = rect.center().y;
        ui.painter().text(
            egui::pos2(lx, ly - 7.0),
            egui::Align2::LEFT_CENTER,
            label,
            egui::FontId::new(SZ_SM, egui::FontFamily::Proportional),
            if selected { TX0 } else { TX1 },
        );
        ui.painter().text(
            egui::pos2(lx, ly + 7.0),
            egui::Align2::LEFT_CENTER,
            models,
            egui::FontId::new(9.5, egui::FontFamily::Monospace),
            TX3,
        );

        // Right-side: kind tag
        let kind_text = if key == "ollama" {
            "LOCAL"
        } else {
            "CLOUD · KEY"
        };
        ui.painter().text(
            egui::pos2(rect.right() - SP3, ly),
            egui::Align2::RIGHT_CENTER,
            kind_text,
            egui::FontId::new(9.0, egui::FontFamily::Monospace),
            if selected { ACCENT } else { TX3 },
        );

        if resp.clicked() && !selected {
            let mut new_cfg = cfg.clone();
            new_cfg.provider = key.to_string();
            if let Some(ev) = env_var {
                new_cfg.api_key_env = ev.to_string();
            }
            crate::api::send_update_vision_config(new_cfg);
        }
        if resp.hovered() {
            ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
        }
        ui.add_space(4.0);
    }

    ui.add_space(SP3);

    // ── Model selection ────────────────────────────────────────────────
    if cfg.provider == "ollama" {
        ollama_model_picker(ui, &cfg);
    } else {
        // Cloud providers: free-text input
        ui.label(RichText::new("MODEL").color(TX3).size(8.5).monospace());
        ui.add_space(SP1);
        let model_id = egui::Id::new("vision_model_edit");
        let mut model_text: String = ui.ctx().data_mut(|m| {
            m.get_temp_mut_or_insert_with::<String>(model_id, || cfg.model.clone())
                .clone()
        });
        if ui
            .add(
                egui::TextEdit::singleline(&mut model_text)
                    .desired_width(360.0)
                    .font(egui::FontId::new(SZ_XS, egui::FontFamily::Monospace)),
            )
            .changed()
        {
            ui.ctx()
                .data_mut(|m| m.insert_temp(model_id, model_text.clone()));
        }
        if model_text != cfg.model {
            ui.add_space(SP1);
            if btn_primary(ui, "Save model").clicked() {
                let mut new_cfg = cfg.clone();
                new_cfg.model = model_text.trim().to_string();
                crate::api::send_update_vision_config(new_cfg);
            }
        }
    }

    if cfg.provider == "ollama" {
        ui.add_space(SP2);
        ui.label(RichText::new("OLLAMA URL").color(TX3).size(8.5).monospace());
        ui.add_space(SP1);
        let url_id = egui::Id::new("vision_url_edit");
        let mut url_text: String = ui.ctx().data_mut(|m| {
            m.get_temp_mut_or_insert_with::<String>(url_id, || cfg.base_url.clone())
                .clone()
        });
        if ui
            .add(
                egui::TextEdit::singleline(&mut url_text)
                    .desired_width(360.0)
                    .font(egui::FontId::new(SZ_XS, egui::FontFamily::Monospace)),
            )
            .changed()
        {
            ui.ctx()
                .data_mut(|m| m.insert_temp(url_id, url_text.clone()));
        }
        if url_text != cfg.base_url {
            ui.add_space(SP1);
            if btn_primary(ui, "Save URL").clicked() {
                let mut new_cfg = cfg.clone();
                new_cfg.base_url = url_text.trim().to_string();
                crate::api::send_update_vision_config(new_cfg);
            }
        }
    }

    // Cloud: API key status
    if cfg.provider != "ollama" {
        ui.add_space(SP2);
        ui.horizontal(|ui| {
            ui.label(RichText::new("API KEY").color(TX3).size(8.5).monospace());
            ui.add_space(SP1);
            ui.label(
                RichText::new(format!("env: {}", cfg.api_key_env))
                    .color(TX1)
                    .size(SZ_XS)
                    .monospace(),
            );
            if cfg.api_key_set {
                ui.add_space(SP2);
                dot(ui, GREEN, 4.0);
                ui.label(RichText::new("set").color(GREEN).size(9.5).monospace());
            } else {
                ui.add_space(SP2);
                dot(ui, RED, 4.0);
                ui.label(
                    RichText::new("missing — set env var or use Providers tab to save")
                        .color(TX3)
                        .size(9.5),
                );
            }
        });
    }

    // ── Poll interval ─────────────────────────────────────────────────
    ui.add_space(SP3);
    ui.label(
        RichText::new("POLL INTERVAL")
            .color(TX3)
            .size(8.5)
            .monospace(),
    );
    ui.add_space(SP1);
    ui.horizontal(|ui| {
        ui.label(
            RichText::new(format!(
                "{} ms ({:.1}s)",
                cfg.poll_ms,
                cfg.poll_ms as f32 / 1000.0
            ))
            .color(TX0)
            .size(SZ_SM)
            .monospace(),
        );
        if icon_btn(ui, "minus", "Slower").clicked() {
            let mut new_cfg = cfg.clone();
            new_cfg.poll_ms = (cfg.poll_ms - 500).max(500);
            crate::api::send_update_vision_config(new_cfg);
        }
        if icon_btn(ui, "plus", "Faster").clicked() {
            let mut new_cfg = cfg.clone();
            new_cfg.poll_ms = (cfg.poll_ms + 500).min(30_000);
            crate::api::send_update_vision_config(new_cfg);
        }
    });

    // ── Window match regex ────────────────────────────────────────────
    ui.add_space(SP2);
    ui.label(
        RichText::new("WINDOW MATCH (regex)")
            .color(TX3)
            .size(8.5)
            .monospace(),
    );
    ui.add_space(SP1);
    let win_id = egui::Id::new("vision_window_edit");
    let mut win_text: String = ui.ctx().data_mut(|m| {
        m.get_temp_mut_or_insert_with::<String>(win_id, || cfg.window_match.clone())
            .clone()
    });
    if ui
        .add(
            egui::TextEdit::singleline(&mut win_text)
                .desired_width(420.0)
                .font(egui::FontId::new(SZ_XS, egui::FontFamily::Monospace)),
        )
        .changed()
    {
        ui.ctx()
            .data_mut(|m| m.insert_temp(win_id, win_text.clone()));
    }
    if win_text != cfg.window_match {
        ui.add_space(SP1);
        if btn_primary(ui, "Save regex").clicked() {
            let mut new_cfg = cfg.clone();
            new_cfg.window_match = win_text.clone();
            crate::api::send_update_vision_config(new_cfg);
        }
    }

    // ── Test probe ────────────────────────────────────────────────────
    ui.add_space(SP3);
    h_rule(ui);
    ui.add_space(SP3);
    ui.label(RichText::new("TEST").color(TX3).size(8.5).monospace());
    ui.add_space(SP1);
    ui.label(RichText::new(
        "Capture your current screen and send it to the configured vision model. Returns what the model 'sees'."
    ).color(TX2).size(SZ_XS));
    ui.add_space(SP2);

    let obs_id = egui::Id::new("vision_last_obs");
    let busy_id = egui::Id::new("vision_probe_busy");
    let busy: bool = ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(busy_id));

    ui.horizontal(|ui| {
        let label = if busy {
            "Probing…"
        } else {
            "Probe now (captures screen)"
        };
        if btn_primary(ui, label).clicked() && !busy {
            ui.ctx().data_mut(|m| m.insert_temp(busy_id, true));
            let ctx_clone = ui.ctx().clone();
            let (tx, rx) = std::sync::mpsc::channel();
            crate::api::send_vision_probe(tx);
            std::thread::spawn(move || {
                if let Ok(result) = rx.recv() {
                    let stored = match result {
                        Ok(obs) => {
                            format!("OK::{}", serde_json::to_string(&obs).unwrap_or_default())
                        }
                        Err(e) => format!("ERR::{}", e),
                    };
                    ctx_clone.data_mut(|m| m.insert_temp(obs_id, stored));
                    ctx_clone.data_mut(|m| m.insert_temp(busy_id, false));
                    ctx_clone.request_repaint();
                }
            });
        }
    });

    // Show last observation
    if let Some(stored) = ui.ctx().data_mut(|m| m.get_temp::<String>(obs_id)) {
        ui.add_space(SP2);
        if let Some(json_str) = stored.strip_prefix("OK::") {
            if let Ok(obs) = serde_json::from_str::<VisionObservation>(json_str) {
                Frame::none()
                    .fill(BG3)
                    .stroke(Stroke::new(1.0, BORDER1))
                    .rounding(R_SM)
                    .inner_margin(egui::Margin::same(SP3))
                    .show(ui, |ui| {
                        ui.horizontal(|ui| {
                            let (col, txt) = if obs.needs_input {
                                (YELLOW, "needs input")
                            } else {
                                (GREEN, "no input needed")
                            };
                            dot(ui, col, 5.0);
                            ui.label(RichText::new(txt).color(col).size(SZ_XS).monospace());
                        });
                        if !obs.summary.is_empty() {
                            ui.add_space(SP1);
                            ui.label(RichText::new(&obs.summary).color(TX1).size(SZ_XS));
                        }
                        if !obs.question.is_empty() {
                            ui.add_space(SP1);
                            ui.label(RichText::new("Question:").color(TX3).size(9.5).monospace());
                            ui.label(
                                RichText::new(&obs.question)
                                    .color(TX0)
                                    .size(SZ_XS)
                                    .monospace(),
                            );
                        }
                        if !obs.choices.is_empty() {
                            ui.add_space(SP1);
                            ui.label(RichText::new("Choices:").color(TX3).size(9.5).monospace());
                            for c in &obs.choices {
                                ui.label(
                                    RichText::new(format!("  · {}", c)).color(TX1).size(SZ_XS),
                                );
                            }
                        }
                    });
            }
        } else if let Some(err) = stored.strip_prefix("ERR::") {
            Frame::none()
                .fill(BG3)
                .stroke(Stroke::new(1.0, RED))
                .rounding(R_SM)
                .inner_margin(egui::Margin::same(SP3))
                .show(ui, |ui| {
                    ui.label(RichText::new(err).color(RED).size(SZ_XS).monospace());
                });
        }
    }
}

/// Instructions panel — shown in the context drawer when CLAUDE.md / AGENTS.md
/// or other instruction files were discovered for the active session.
/// Each origin gets its own subtle color: project=accent, user-global=blue, editor=t1.
fn draw_instructions_panel(ui: &mut Ui, ins: &InstructionsState) {
    ui.add_space(4.0);
    Frame::none()
        .inner_margin(egui::Margin::symmetric(SP4, SP2))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                ui.label(
                    RichText::new("AGENT CONTEXT")
                        .color(TX3)
                        .size(8.5)
                        .monospace(),
                );
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    ui.label(
                        RichText::new(format!("{} files", ins.sources.len()))
                            .color(TX3)
                            .size(9.0)
                            .monospace(),
                    );
                });
            });
            ui.add_space(SP1);

            for s in &ins.sources {
                let (origin_col, origin_short) = match s.origin.as_str() {
                    "project" => (ACCENT, "proj"),
                    "user-global" => (BLUE, "user"),
                    "editor-rule" => (TX1, "edit"),
                    _ => (TX2, "ext "),
                };
                // One row per source: [origin tag] label … size
                Frame::none()
                    .fill(BG3)
                    .rounding(R_SM)
                    .inner_margin(egui::Margin::symmetric(SP2, 3.0))
                    .outer_margin(egui::Margin {
                        bottom: 2.0,
                        ..Default::default()
                    })
                    .show(ui, |ui| {
                        ui.horizontal(|ui| {
                            ui.label(
                                RichText::new(origin_short)
                                    .color(origin_col)
                                    .size(8.5)
                                    .monospace()
                                    .strong(),
                            );
                            ui.label(RichText::new(&s.label).color(TX1).size(SZ_XS));
                            ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                                ui.label(
                                    RichText::new(fmt_size_bytes(s.bytes))
                                        .color(TX3)
                                        .size(9.0)
                                        .monospace(),
                                );
                            });
                        });
                    });
            }

            if !ins.skills.is_empty() {
                ui.add_space(SP1);
                ui.label(
                    RichText::new("PROFILE SKILLS")
                        .color(TX3)
                        .size(8.5)
                        .monospace(),
                );
                ui.add_space(2.0);
                ui.horizontal_wrapped(|ui| {
                    for sk in &ins.skills {
                        Frame::none()
                            .fill(ACCENT_BG)
                            .rounding(R_PILL)
                            .inner_margin(egui::Margin::symmetric(SP2, 1.5))
                            .show(ui, |ui| {
                                ui.label(RichText::new(sk).color(ACCENT).size(9.0).monospace());
                            });
                    }
                });
            }
        });
}

fn fmt_size_bytes(n: i64) -> String {
    if n >= 1024 {
        format!("{:.1}K", n as f64 / 1024.0)
    } else {
        format!("{}B", n)
    }
}

/// Ollama-specific model picker — lists installed vision models with a
/// "selected" indicator, plus curated pullable models with Pull buttons.
/// Falls back to a free-text editor if the daemon can't reach Ollama.
fn ollama_model_picker(ui: &mut Ui, cfg: &VisionConfigDto) {
    ui.label(RichText::new("MODEL").color(TX3).size(8.5).monospace());
    ui.add_space(SP1);

    let cache_id = egui::Id::new("ollama_models_cache");
    let loading_id = egui::Id::new("ollama_models_loading");
    let error_id = egui::Id::new("ollama_models_error");

    let cached: Option<String> = ui.ctx().data_mut(|m| m.get_temp::<String>(cache_id));
    let loading: bool = ui
        .ctx()
        .data_mut(|m| *m.get_temp_mut_or_default::<bool>(loading_id));
    let last_error: Option<String> = ui.ctx().data_mut(|m| m.get_temp::<String>(error_id));

    // Top bar: refresh + current selection
    ui.horizontal(|ui| {
        ui.label(RichText::new("Current:").color(TX3).size(9.5));
        ui.label(
            RichText::new(&cfg.model)
                .color(ACCENT)
                .size(SZ_XS)
                .monospace(),
        );
        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
            let label = if loading {
                "Loading…"
            } else if cached.is_some() {
                "Refresh"
            } else {
                "Load models"
            };
            if btn(ui, label).clicked() && !loading {
                let ctx_clone = ui.ctx().clone();
                ctx_clone.data_mut(|m| m.insert_temp(loading_id, true));
                let (tx, rx) = std::sync::mpsc::channel();
                crate::api::send_list_ollama_models(cfg.base_url.clone(), tx);
                std::thread::spawn(move || {
                    if let Ok(result) = rx.recv() {
                        match result {
                            Ok(resp) => {
                                let s = serde_json::to_string(&resp).unwrap_or_default();
                                ctx_clone.data_mut(|m| m.insert_temp(cache_id, s));
                                ctx_clone.data_mut(|m| m.remove::<String>(error_id));
                            }
                            Err(e) => {
                                ctx_clone.data_mut(|m| m.insert_temp(error_id, e));
                                ctx_clone.data_mut(|m| m.remove::<String>(cache_id));
                            }
                        }
                        ctx_clone.data_mut(|m| m.insert_temp(loading_id, false));
                        ctx_clone.request_repaint();
                    }
                });
            }
        });
    });

    if let Some(err) = last_error {
        ui.add_space(SP1);
        ui.label(
            RichText::new(format!("⚠ {}", err))
                .color(YELLOW)
                .size(SZ_XS)
                .monospace(),
        );
        return;
    }

    let Some(json_str) = cached else {
        ui.add_space(SP1);
        ui.label(
            RichText::new("Click Load models to scan local Ollama for installed models.")
                .color(TX3)
                .size(SZ_XS),
        );
        return;
    };

    let parsed: crate::types::OllamaModelsResponse =
        serde_json::from_str(&json_str).unwrap_or_default();

    // Installed section
    ui.add_space(SP2);
    ui.label(
        RichText::new(format!("INSTALLED ({})", parsed.installed.len()))
            .color(TX3)
            .size(8.5)
            .monospace(),
    );
    ui.add_space(SP1);
    if parsed.installed.is_empty() {
        ui.label(
            RichText::new("No models installed yet. Pull one below.")
                .color(TX3)
                .size(SZ_XS)
                .italics(),
        );
    } else {
        for m in &parsed.installed {
            let selected = m.name == cfg.model;
            let (rect, resp) =
                ui.allocate_exact_size(Vec2::new(ui.available_width(), 32.0), Sense::click());
            let fill = if selected {
                ACCENT_BG
            } else if resp.hovered() {
                BG3
            } else {
                Color32::TRANSPARENT
            };
            ui.painter().rect_filled(rect, R_SM, fill);
            if selected {
                ui.painter()
                    .rect_stroke(rect, R_SM, Stroke::new(1.0, ACCENT));
            }
            let name_col = if selected { TX0 } else { TX1 };
            ui.painter().text(
                egui::pos2(rect.left() + SP3, rect.center().y),
                egui::Align2::LEFT_CENTER,
                &m.name,
                egui::FontId::new(SZ_XS, egui::FontFamily::Monospace),
                name_col,
            );
            // Right side: size + vision badge
            let size_text = format!("{} · {}", fmt_size_gb(m.size), m.param_size);
            ui.painter().text(
                egui::pos2(rect.right() - SP3, rect.center().y),
                egui::Align2::RIGHT_CENTER,
                size_text,
                egui::FontId::new(9.5, egui::FontFamily::Monospace),
                TX3,
            );
            if m.is_vision {
                ui.painter().text(
                    egui::pos2(rect.right() - SP3 - 120.0, rect.center().y),
                    egui::Align2::RIGHT_CENTER,
                    "VISION",
                    egui::FontId::new(9.0, egui::FontFamily::Monospace),
                    GREEN,
                );
            }
            if resp.hovered() {
                ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
            }
            if resp.clicked() && !selected {
                let mut new_cfg = cfg.clone();
                new_cfg.model = m.name.clone();
                crate::api::send_update_vision_config(new_cfg);
            }
            ui.add_space(3.0);
        }
    }

    // Curated section
    ui.add_space(SP3);
    ui.horizontal(|ui| {
        ui.label(
            RichText::new("PULL VISION MODEL")
                .color(TX3)
                .size(8.5)
                .monospace(),
        );
        ui.label(RichText::new("(from ollama.com)").color(TX3).size(9.5));
    });
    ui.add_space(SP1);

    let installed_names: std::collections::HashSet<String> =
        parsed.installed.iter().map(|m| m.name.clone()).collect();

    for cm in &parsed.curated {
        let already = installed_names.contains(&cm.tag);
        Frame::none()
            .fill(BG3)
            .stroke(Stroke::new(1.0, BORDER0))
            .rounding(R_SM)
            .inner_margin(egui::Margin::same(SP2))
            .outer_margin(egui::Margin {
                bottom: 4.0,
                ..Default::default()
            })
            .show(ui, |ui| {
                ui.horizontal(|ui| {
                    ui.vertical(|ui| {
                        ui.horizontal(|ui| {
                            ui.label(
                                RichText::new(&cm.display_name)
                                    .color(TX0)
                                    .size(SZ_XS)
                                    .strong(),
                            );
                            ui.label(
                                RichText::new(format!("· {}", cm.tag))
                                    .color(TX3)
                                    .size(9.5)
                                    .monospace(),
                            );
                        });
                        ui.label(RichText::new(&cm.description).color(TX2).size(SZ_XS));
                    });
                    ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                        if already {
                            ui.label(
                                RichText::new("✓ installed")
                                    .color(GREEN)
                                    .size(SZ_XS)
                                    .monospace(),
                            );
                        } else {
                            if btn_primary(ui, "Pull").clicked() {
                                crate::api::send_pull_ollama_model(
                                    cm.tag.clone(),
                                    cfg.base_url.clone(),
                                );
                            }
                        }
                        ui.add_space(SP2);
                        ui.label(RichText::new(&cm.size).color(TX3).size(9.5).monospace());
                    });
                });
            });
    }
}

fn fmt_size_gb(bytes: i64) -> String {
    if bytes >= 1_000_000_000 {
        format!("{:.1} GB", bytes as f64 / 1_000_000_000.0)
    } else if bytes >= 1_000_000 {
        format!("{:.0} MB", bytes as f64 / 1_000_000.0)
    } else {
        format!("{} B", bytes)
    }
}

// ─── Providers tab — bespoke layout instead of cookie-cutter cards ──────────

fn draw_providers_tab(ui: &mut Ui, state: &DashboardState) {
    if state.provider_details.is_empty() {
        ui.add_space(SP4);
        ui.label(
            RichText::new("Loading provider info…")
                .color(TX2)
                .size(SZ_XS),
        );
        ui.add_space(SP1);
        ui.label(
            RichText::new("Start the daemon to scan your system.")
                .color(TX3)
                .size(SZ_XS),
        );
        return;
    }

    // Summary header
    let total = state.provider_details.len();
    let ready: usize = state
        .provider_details
        .iter()
        .filter(|d| d.probe_status == "available")
        .count();
    let needs_setup: usize = state
        .provider_details
        .iter()
        .filter(|d| {
            matches!(
                d.probe_status.as_str(),
                "not_found" | "unavailable" | "no_key"
            )
        })
        .count();

    ui.horizontal(|ui| {
        ui.label(
            RichText::new(ready.to_string())
                .color(GREEN)
                .size(28.0)
                .strong()
                .monospace(),
        );
        ui.add_space(2.0);
        ui.vertical(|ui| {
            ui.add_space(SP1);
            ui.label(
                RichText::new(format!("of {} ready", total))
                    .color(TX1)
                    .size(SZ_XS),
            );
            if needs_setup > 0 {
                ui.label(
                    RichText::new(format!("{} need setup", needs_setup))
                        .color(TX3)
                        .size(9.5),
                );
            }
        });
        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
            ui.label(
                RichText::new("edit .relay/relay.toml for advanced config")
                    .color(TX3)
                    .size(9.5),
            );
        });
    });

    ui.add_space(SP3);
    h_rule(ui);
    ui.add_space(SP3);

    // Group: ready providers first, then setup-needed
    let mut order: Vec<&ProviderDetail> = state.provider_details.iter().collect();
    order.sort_by_key(|d| match d.probe_status.as_str() {
        "available" => 0,
        "no_key" => 1,
        "unavailable" => 2,
        "installing" | "authenticating" => 3,
        "not_found" => 4,
        _ => 5,
    });

    for (i, d) in order.iter().enumerate() {
        provider_row(ui, d);
        if i < order.len() - 1 {
            ui.add_space(SP2);
        }
    }
}

/// Single-row provider entry. Inline layout, no nested card boxes.
fn provider_row(ui: &mut Ui, d: &ProviderDetail) {
    let (accent, status_label) = match d.probe_status.as_str() {
        "available" => (GREEN, "ready"),
        "no_key" => (YELLOW, "needs sign-in"),
        "not_found" => (TX2, "not installed"),
        "unavailable" => (YELLOW, "not running"),
        "installing" => (ACCENT, "installing"),
        "authenticating" => (ACCENT, "signing in"),
        _ => (TX2, "manual setup"),
    };
    let is_busy = d.probe_status == "installing" || d.probe_status == "authenticating";
    let needs_install = matches!(
        d.probe_status.as_str(),
        "not_found" | "unavailable" | "manual"
    );
    let needs_auth = d.probe_status == "no_key";

    // Background tint matches state — ready=darker(bg3), needs-work=bg3+yellow tint via border
    let fill = BG3;
    let border = if d.probe_status == "available" {
        Stroke::new(1.0, BORDER0)
    } else if matches!(d.probe_status.as_str(), "installing" | "authenticating") {
        Stroke::new(1.0, ACCENT)
    } else {
        Stroke::new(1.0, BORDER1)
    };

    Frame::none()
        .fill(fill)
        .stroke(border)
        .rounding(R_LG)
        .inner_margin(egui::Margin {
            left: SP4,
            right: SP3,
            top: SP3,
            bottom: SP3,
        })
        .show(ui, |ui| {
            // ─── Top row: indicator | name | status pill + toggle ────────
            ui.horizontal(|ui| {
                // Vertical accent bar (not full border-left ban — this is inside the card)
                let (bar_rect, _) = ui.allocate_exact_size(Vec2::new(3.0, 36.0), Sense::hover());
                ui.painter()
                    .rect_filled(bar_rect, Rounding::same(2.0), accent);
                ui.add_space(SP2);

                // Provider info
                ui.vertical(|ui| {
                    ui.horizontal(|ui| {
                        ui.label(
                            RichText::new(&d.display_name)
                                .color(TX0)
                                .size(SZ_MD)
                                .strong(),
                        );
                        ui.add_space(SP1);
                        // Inline status text (no badge box)
                        ui.label(
                            RichText::new(format!("· {}", status_label))
                                .color(accent)
                                .size(SZ_XS)
                                .monospace(),
                        );
                    });
                    ui.label(RichText::new(&d.description).color(TX2).size(SZ_XS));
                });

                // Right side: kind tag + enable toggle
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    let mut enabled = d.enabled;
                    toggle(ui, &mut enabled);
                    if enabled != d.enabled {
                        crate::api::send_update_provider(d.name.clone(), enabled, None, None, None);
                    }
                    ui.add_space(SP3);
                    let kind_text = match d.kind.as_str() {
                        "cli" => "CLI",
                        "local" => "LOCAL",
                        "extension" => "EXT",
                        _ => "API",
                    };
                    ui.label(RichText::new(kind_text).color(TX3).size(9.0).monospace());
                });
            });

            // ─── Actions / hint section (only if not ready) ────────────
            if d.probe_status != "available" {
                ui.add_space(SP3);

                if is_busy {
                    // Animated spinner-ish text
                    let elapsed = ui.input(|i| i.time);
                    let dots = match (elapsed * 2.0) as i64 % 4 {
                        0 => "",
                        1 => ".",
                        2 => "..",
                        _ => "...",
                    };
                    ui.label(
                        RichText::new(format!("running{}", dots))
                            .color(ACCENT)
                            .size(SZ_XS)
                            .monospace(),
                    );
                    ui.label(
                        RichText::new("see new terminal window for output")
                            .color(TX3)
                            .size(9.5),
                    );
                    ui.ctx()
                        .request_repaint_after(std::time::Duration::from_millis(400));
                } else {
                    // Action row
                    ui.horizontal_wrapped(|ui| {
                        if d.can_install && needs_install {
                            if btn_primary(ui, "Install").clicked() {
                                crate::api::send_install_provider(d.name.clone());
                            }
                            ui.add_space(SP1);
                        }
                        if d.can_oauth && (needs_auth || needs_install) {
                            let label = if needs_install {
                                "Then sign in"
                            } else {
                                "Sign in with browser"
                            };
                            if btn(ui, label).clicked() {
                                crate::api::send_oauth_provider(d.name.clone());
                            }
                            ui.add_space(SP1);
                        }
                        if d.can_api_key && (needs_auth || needs_install) {
                            let id = egui::Id::new(("apikey_open", &d.name));
                            let mut open = ui
                                .ctx()
                                .data_mut(|m| *m.get_temp_mut_or_default::<bool>(id));
                            let label = if open { "Hide API key" } else { "Use API key" };
                            if btn(ui, label).clicked() {
                                open = !open;
                                ui.ctx().data_mut(|m| m.insert_temp(id, open));
                            }
                            ui.add_space(SP1);
                        }
                        if !d.setup_url.is_empty() {
                            if btn(ui, "Docs").clicked() {
                                let _ = crate::api::open_url(&d.setup_url);
                            }
                            ui.add_space(SP1);
                        }
                        // Ollama-backed fallback toggle
                        if d.can_launch_ollama {
                            let id = egui::Id::new(("ollama_launch_open", &d.name));
                            let mut open = ui
                                .ctx()
                                .data_mut(|m| *m.get_temp_mut_or_default::<bool>(id));
                            let label = if open {
                                "Hide Ollama"
                            } else {
                                "Run via Ollama"
                            };
                            if btn(ui, label).clicked() {
                                open = !open;
                                ui.ctx().data_mut(|m| m.insert_temp(id, open));
                            }
                        }
                    });

                    // Ollama-launch model picker
                    if d.can_launch_ollama {
                        let id = egui::Id::new(("ollama_launch_open", &d.name));
                        if ui
                            .ctx()
                            .data_mut(|m| *m.get_temp_mut_or_default::<bool>(id))
                        {
                            ui.add_space(SP2);
                            ollama_launch_form(ui, d);
                        }
                    }

                    // API key form
                    if d.can_api_key {
                        let id = egui::Id::new(("apikey_open", &d.name));
                        if ui
                            .ctx()
                            .data_mut(|m| *m.get_temp_mut_or_default::<bool>(id))
                        {
                            ui.add_space(SP2);
                            api_key_form(ui, d);
                        }
                    }
                }
            } else {
                // Model picker (works for any provider with available_models)
                if !d.available_models.is_empty() {
                    ui.add_space(SP2);
                    provider_model_picker(ui, d);
                }
                // Account switcher (account-aware handoff, pillar 3). Always
                // shown so multi-login is discoverable, with a hint when empty.
                ui.add_space(SP2);
                account_switcher(ui, d);
                // Ready state — show subtle re-auth + ollama details inline
                let has_extra =
                    d.name == "ollama" || d.can_oauth || d.can_api_key || d.declared_cap > 0;
                if has_extra {
                    ui.add_space(SP2);
                    ui.horizontal_wrapped(|ui| {
                        // Ollama: model + URL
                        if d.name == "ollama" {
                            ui.label(
                                RichText::new(d.model.as_deref().unwrap_or("qwen2.5-coder:32b"))
                                    .color(TX1)
                                    .size(SZ_XS)
                                    .monospace(),
                            );
                            ui.label(RichText::new("·").color(TX3).size(9.0));
                            ui.label(
                                RichText::new(
                                    d.base_url.as_deref().unwrap_or("http://localhost:11434"),
                                )
                                .color(TX2)
                                .size(SZ_XS)
                                .monospace(),
                            );
                            ui.add_space(SP3);
                        }
                        // Declared cap
                        if d.declared_cap > 0 {
                            ui.label(
                                RichText::new(format!("cap {}", fmt_tokens(d.declared_cap as u64)))
                                    .color(TX3)
                                    .size(9.5)
                                    .monospace(),
                            );
                            ui.add_space(SP3);
                        }
                        // Re-auth in right column
                        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                            if d.can_api_key {
                                let id = egui::Id::new(("apikey_open", &d.name));
                                let mut open = ui
                                    .ctx()
                                    .data_mut(|m| *m.get_temp_mut_or_default::<bool>(id));
                                let label = if open {
                                    "Hide"
                                } else if d.api_key_set {
                                    "Replace key"
                                } else {
                                    "Add key"
                                };
                                if btn(ui, label).clicked() {
                                    open = !open;
                                    ui.ctx().data_mut(|m| m.insert_temp(id, open));
                                }
                                ui.add_space(SP1);
                            }
                        });
                    });
                    if d.can_api_key {
                        let id = egui::Id::new(("apikey_open", &d.name));
                        if ui
                            .ctx()
                            .data_mut(|m| *m.get_temp_mut_or_default::<bool>(id))
                        {
                            ui.add_space(SP2);
                            api_key_form(ui, d);
                        }
                    }
                }
            }
        });
}

/// account_switcher renders one chip per configured login for a provider; clicking
/// an inactive one switches the active account for the next handoff (pillar 3).
fn account_switcher(ui: &mut Ui, d: &ProviderDetail) {
    ui.horizontal_wrapped(|ui| {
        ui.label(RichText::new("Account").color(TX3).size(8.5).monospace());
        ui.add_space(SP1);
        if d.accounts.is_empty() {
            ui.label(
                RichText::new("one login — add more in .relay/relay.toml to switch")
                    .color(TX3)
                    .size(9.0)
                    .italics(),
            );
            return;
        }
        for a in &d.accounts {
            let active = a.active || a.label == d.active_account;
            if chip_select(ui, &a.label, active).clicked() && !active {
                crate::api::send_switch_account(d.name.clone(), a.label.clone());
            }
        }
    });
}

/// Ollama-launch form embedded inside a provider card.
/// Runs `ollama launch <tool> --model <model>` so a provider that's missing
/// cloud auth can still operate against a local Ollama backend.
fn ollama_launch_form(ui: &mut Ui, d: &ProviderDetail) {
    Frame::none()
        .fill(BG2)
        .stroke(Stroke::new(1.0, BORDER1))
        .rounding(R_SM)
        .inner_margin(egui::Margin::same(SP3))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                ui.label(
                    RichText::new("via local Ollama")
                        .color(TX1)
                        .size(SZ_XS)
                        .monospace(),
                );
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if ui
                        .add(
                            egui::Label::new(
                                RichText::new("how it works ↗")
                                    .color(TX3)
                                    .size(9.0)
                                    .underline(),
                            )
                            .sense(Sense::click()),
                        )
                        .clicked()
                    {
                        let _ =
                            crate::api::open_url(&format!(
                            "https://github.com/ollama/ollama/blob/main/docs/integrations/{}.mdx",
                            if d.name == "claude" { "claude-code" } else { d.name.as_str() }
                        ));
                    }
                });
            });
            ui.add_space(SP1);
            ui.label(
                RichText::new(
                    "Skips cloud auth. Runs the provider against a model installed in Ollama.",
                )
                .color(TX2)
                .size(SZ_XS),
            );
            ui.add_space(SP2);

            if d.ollama_model_suggestions.is_empty() {
                ui.label(
                    RichText::new("No recommended models for this provider.")
                        .color(TX3)
                        .size(SZ_XS),
                );
                return;
            }

            ui.label(
                RichText::new("RECOMMENDED MODELS")
                    .color(TX3)
                    .size(8.5)
                    .monospace(),
            );
            ui.add_space(SP1);

            for tag in &d.ollama_model_suggestions {
                Frame::none()
                    .fill(BG3)
                    .rounding(R_SM)
                    .inner_margin(egui::Margin::symmetric(SP2, 3.0))
                    .outer_margin(egui::Margin {
                        bottom: 3.0,
                        ..Default::default()
                    })
                    .show(ui, |ui| {
                        ui.horizontal(|ui| {
                            // Cloud-suffixed tags get a small badge
                            let is_cloud = tag.ends_with(":cloud");
                            if is_cloud {
                                Frame::none()
                                    .fill(BG4)
                                    .rounding(R_PILL)
                                    .inner_margin(egui::Margin::symmetric(SP2, 1.0))
                                    .show(ui, |ui| {
                                        ui.label(
                                            RichText::new("cloud")
                                                .color(BLUE)
                                                .size(8.5)
                                                .monospace(),
                                        );
                                    });
                            } else {
                                Frame::none()
                                    .fill(BG4)
                                    .rounding(R_PILL)
                                    .inner_margin(egui::Margin::symmetric(SP2, 1.0))
                                    .show(ui, |ui| {
                                        ui.label(
                                            RichText::new("local")
                                                .color(GREEN)
                                                .size(8.5)
                                                .monospace(),
                                        );
                                    });
                            }
                            ui.label(RichText::new(tag).color(TX0).size(SZ_XS).monospace());
                            ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                                let label = if is_cloud { "Launch (cloud)" } else { "Launch" };
                                if btn_primary(ui, label).clicked() {
                                    crate::api::send_ollama_launch(d.name.clone(), tag.clone());
                                }
                            });
                        });
                    });
            }
        });
}

/// Model picker for a provider card. Renders a row of pills; click to switch.
/// Persists via POST /api/config/providers writing `model` field.
fn provider_model_picker(ui: &mut Ui, d: &ProviderDetail) {
    let current = d.model.clone().unwrap_or_default();
    ui.horizontal_wrapped(|ui| {
        ui.label(RichText::new("MODEL").color(TX3).size(8.5).monospace());
        ui.add_space(SP1);
        // Default option (clears the override)
        let default_active = current.is_empty();
        if model_pill(ui, "default", default_active).clicked() {
            crate::api::send_update_provider(
                d.name.clone(),
                d.enabled,
                None,
                Some(String::new()),
                None,
            );
        }
        for m in &d.available_models {
            let active = m == &current;
            if model_pill(ui, m, active).clicked() && !active {
                crate::api::send_update_provider(
                    d.name.clone(),
                    d.enabled,
                    None,
                    Some(m.clone()),
                    None,
                );
            }
        }
    });
}

fn model_pill(ui: &mut Ui, label: &str, active: bool) -> egui::Response {
    let font = egui::FontId::new(SZ_XS, egui::FontFamily::Monospace);
    let galley = ui
        .painter()
        .layout_no_wrap(label.to_owned(), font.clone(), TX0);
    let desired = Vec2::new(galley.size().x + SP3 * 2.0, galley.size().y + 6.0);
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
    let (fill, fg, border) = if active {
        (ACCENT_BG, ACCENT, ACCENT)
    } else if resp.hovered() {
        (BTN_HOVER, TX0, BORDER2)
    } else {
        (BTN_BG, TX1, BORDER1)
    };
    ui.painter().rect_filled(rect, R_PILL, fill);
    ui.painter()
        .rect_stroke(rect, R_PILL, Stroke::new(1.0, border));
    ui.painter()
        .text(rect.center(), egui::Align2::CENTER_CENTER, label, font, fg);
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
    resp
}

/// API-key entry form embedded inside a provider card.
/// Persists the in-progress value via egui's data store keyed by provider name.
fn api_key_form(ui: &mut Ui, d: &ProviderDetail) {
    let key_id = egui::Id::new(("apikey_value", &d.name));
    let mut value: String = ui
        .ctx()
        .data_mut(|m| m.get_temp_mut_or_default::<String>(key_id).clone());

    Frame::none()
        .fill(BG2)
        .stroke(Stroke::new(1.0, BORDER1))
        .rounding(R_SM)
        .inner_margin(egui::Margin::same(SP3))
        .show(ui, |ui| {
            // Header line
            ui.horizontal(|ui| {
                let env_var = d.api_key_env_var.as_deref().unwrap_or("API_KEY");
                ui.label(RichText::new(env_var).color(TX1).size(SZ_XS).monospace());
                if d.api_key_set {
                    ui.add_space(SP2);
                    dot(ui, GREEN, 5.0);
                    ui.label(
                        RichText::new("currently set")
                            .color(GREEN)
                            .size(9.5)
                            .monospace(),
                    );
                }
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if let Some(url) = &d.api_key_url {
                        if !url.is_empty() {
                            let r = ui.add(
                                egui::Label::new(
                                    RichText::new("Get a key →")
                                        .color(ACCENT)
                                        .size(SZ_XS)
                                        .underline(),
                                )
                                .sense(Sense::click()),
                            );
                            if r.clicked() {
                                let _ = crate::api::open_url(url);
                            }
                            if r.hovered() {
                                ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                            }
                        }
                    }
                });
            });
            ui.add_space(SP2);

            // Password-style input
            let resp = ui.add(
                egui::TextEdit::singleline(&mut value)
                    .password(true)
                    .desired_width(f32::INFINITY)
                    .hint_text("paste API key, then Save")
                    .font(egui::FontId::new(SZ_XS, egui::FontFamily::Monospace)),
            );

            // Persist intermediate value
            ui.ctx().data_mut(|m| m.insert_temp(key_id, value.clone()));

            ui.add_space(SP2);
            ui.horizontal(|ui| {
                let can_save = !value.trim().is_empty();
                let save_btn = btn_primary(ui, "Save key");
                let enter_pressed =
                    resp.lost_focus() && ui.input(|i| i.key_pressed(egui::Key::Enter));
                if (save_btn.clicked() || enter_pressed) && can_save {
                    crate::api::send_api_key(d.name.clone(), value.trim().to_string());
                    // Clear stored value and close the form
                    ui.ctx()
                        .data_mut(|m| m.insert_temp::<String>(key_id, String::new()));
                    let open_id = egui::Id::new(("apikey_open", &d.name));
                    ui.ctx().data_mut(|m| m.insert_temp(open_id, false));
                }
                ui.add_space(SP1);
                if btn(ui, "Cancel").clicked() {
                    ui.ctx()
                        .data_mut(|m| m.insert_temp::<String>(key_id, String::new()));
                    let open_id = egui::Id::new(("apikey_open", &d.name));
                    ui.ctx().data_mut(|m| m.insert_temp(open_id, false));
                }
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    ui.label(
                        RichText::new("Saved to .relay/.env (gitignored)")
                            .color(TX3)
                            .size(9.5),
                    );
                });
            });
        });
}

fn srow(ui: &mut Ui, label: &str, sub: Option<&str>, content: impl FnOnce(&mut Ui)) {
    Frame::none()
        .inner_margin(egui::Margin {
            top: SP2,
            bottom: SP2,
            ..Default::default()
        })
        .stroke(Stroke::new(0.0, Color32::TRANSPARENT))
        .show(ui, |ui| {
            ui.horizontal(|ui| {
                ui.vertical(|ui| {
                    ui.label(RichText::new(label).color(TX0).size(SZ_SM));
                    if let Some(s) = sub {
                        ui.label(RichText::new(s).color(TX2).size(SZ_XS));
                    }
                });
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    content(ui);
                });
            });
        });
    h_rule(ui);
}

fn toggle(ui: &mut Ui, on: &mut bool) {
    let desired = Vec2::new(32.0, 18.0);
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
    let fill = if *on { ACCENT } else { BG4 };
    ui.painter().rect_filled(rect, R_PILL, fill);
    let knob_x = if *on {
        rect.right() - 10.0
    } else {
        rect.left() + 10.0
    };
    ui.painter()
        .circle_filled(egui::pos2(knob_x, rect.center().y), 7.0, TX0);
    if resp.clicked() {
        *on = !*on;
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
}

// ═══════════════════════════════════════════════════════════════════════════
// HANDOFF OVERLAY — cinematic
// ═══════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════
// Approval bar — top-of-screen toast row for pending approvals
// ═══════════════════════════════════════════════════════════════════════════
fn draw_approval_bar(ctx: &egui::Context, approvals: &[ApprovalRequest]) {
    egui::TopBottomPanel::top("approval_bar")
        .exact_height(46.0)
        .frame(
            Frame::none()
                .fill(YELLOW_BG)
                .stroke(Stroke::new(1.0, YELLOW))
                .inner_margin(egui::Margin::symmetric(SP4, SP1)),
        )
        .show(ctx, |ui| {
            let req = &approvals[0]; // show first; others queued
            ui.horizontal_centered(|ui| {
                let (col, label) = match req.severity.as_str() {
                    "danger" => (RED, "DANGER"),
                    "warn" => (YELLOW, "REVIEW"),
                    _ => (TX1, "INFO"),
                };
                ui.label(
                    RichText::new(label)
                        .color(col)
                        .size(SZ_XS)
                        .strong()
                        .monospace(),
                );
                ui.add_space(SP2);
                ui.vertical(|ui| {
                    ui.label(RichText::new(&req.action).color(TX0).size(SZ_XS).strong());
                    if !req.reason.is_empty() {
                        ui.label(RichText::new(&req.reason).color(TX2).size(9.5));
                    }
                });
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if btn(ui, "Deny").clicked() {
                        crate::api::send_approval(req.id.clone(), false, String::new());
                    }
                    ui.add_space(SP1);
                    if btn_primary(ui, "Approve").clicked() {
                        crate::api::send_approval(req.id.clone(), true, String::new());
                    }
                    if approvals.len() > 1 {
                        ui.add_space(SP2);
                        ui.label(
                            RichText::new(format!("+{} queued", approvals.len() - 1))
                                .color(TX3)
                                .size(9.5),
                        );
                    }
                });
            });
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// Slash command palette — Cmd-K modal
// ═══════════════════════════════════════════════════════════════════════════
#[allow(clippy::too_many_arguments)]
fn draw_slash_palette(
    ctx: &egui::Context,
    show: &mut bool,
    text: &mut String,
    sel: &mut usize,
    show_handoff: &mut bool,
    handoff_start: &mut Option<Instant>,
    new_task_open: &mut bool,
    nav: &mut NavPage,
    main_tab: &mut MainTab,
    paused: &mut bool,
) {
    // All commands. fn returns true if it consumes (closes palette).
    #[allow(dead_code)]
    struct Cmd {
        label: &'static str,
        hint: &'static str,
        action: Action,
    }
    #[allow(dead_code)]
    enum Action {
        Run(fn(&mut bool, &mut Option<Instant>, &mut bool, &mut NavPage, &mut MainTab, &mut bool)),
    }
    let commands: &[(&str, &str, &str)] = &[
        ("/new-task", "Open new task dialog", "new_task"),
        ("/handoff", "Trigger immediate handoff", "handoff"),
        ("/pause", "Pause / resume agent", "pause"),
        ("/dashboard", "Go to Dashboard", "nav_dashboard"),
        ("/detect", "Detect running agents", "nav_detect"),
        ("/projects", "Go to Projects", "nav_projects"),
        ("/graph", "Go to Graph", "nav_graph"),
        ("/profiles", "Go to Profiles", "nav_profiles"),
        ("/audit", "Go to Audit", "nav_audit"),
        ("/settings", "Go to Settings", "nav_settings"),
        ("/diff", "Show diff", "tab_diff"),
        ("/contract", "Show contract", "tab_contract"),
        ("/cost", "Show event stream", "tab_stream"),
    ];
    let _ = (Action::Run(|_, _, _, _, _, _| {}),); // silence unused-warning in enum
    let _ = commands.len();
    let q = text.to_lowercase();
    let filtered: Vec<(&str, &str, &str)> = commands
        .iter()
        .filter(|(cmd, _, _)| q.is_empty() || cmd.contains(&q))
        .cloned()
        .collect();

    if *sel >= filtered.len() && !filtered.is_empty() {
        *sel = 0;
    }

    egui::Window::new("slash_palette")
        .title_bar(false)
        .resizable(false)
        .anchor(egui::Align2::CENTER_TOP, egui::Vec2::new(0.0, 80.0))
        .fixed_size(egui::Vec2::new(440.0, 320.0))
        .frame(
            Frame::none()
                .fill(BG1)
                .stroke(Stroke::new(1.0, ACCENT))
                .rounding(R_LG),
        )
        .show(ctx, |ui| {
            // Input
            Frame::none()
                .inner_margin(egui::Margin::same(SP3))
                .show(ui, |ui| {
                    ui.horizontal(|ui| {
                        ui.label(
                            RichText::new("/")
                                .color(ACCENT)
                                .size(SZ_MD)
                                .strong()
                                .monospace(),
                        );
                        let resp = ui.add(
                            egui::TextEdit::singleline(text)
                                .desired_width(380.0)
                                .hint_text("type a command…")
                                .font(egui::FontId::new(SZ_MD, egui::FontFamily::Proportional)),
                        );
                        resp.request_focus();

                        let input = ui.input(|i| {
                            (
                                i.key_pressed(egui::Key::Escape),
                                i.key_pressed(egui::Key::Enter),
                                i.key_pressed(egui::Key::ArrowDown),
                                i.key_pressed(egui::Key::ArrowUp),
                            )
                        });
                        if input.0 {
                            *show = false;
                        }
                        if input.2 && !filtered.is_empty() {
                            *sel = (*sel + 1) % filtered.len();
                        }
                        if input.3 && !filtered.is_empty() {
                            *sel = if *sel == 0 {
                                filtered.len() - 1
                            } else {
                                *sel - 1
                            };
                        }
                        if input.1 && !filtered.is_empty() {
                            let id = filtered[*sel].2;
                            run_palette_action(
                                id,
                                show_handoff,
                                handoff_start,
                                new_task_open,
                                nav,
                                main_tab,
                                paused,
                            );
                            *show = false;
                        }
                    });
                });
            h_rule(ui);

            // Results
            ScrollArea::vertical()
                .auto_shrink([false, false])
                .max_height(250.0)
                .show(ui, |ui| {
                    for (i, (cmd, hint, id)) in filtered.iter().enumerate() {
                        let active = i == *sel;
                        let desired = Vec2::new(ui.available_width(), 32.0);
                        let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
                        let fill = if active {
                            ACCENT_BG
                        } else if resp.hovered() {
                            BG2
                        } else {
                            Color32::TRANSPARENT
                        };
                        ui.painter().rect_filled(rect, R_SM, fill);
                        let col = if active { TX0 } else { TX1 };
                        ui.painter().text(
                            egui::pos2(rect.left() + SP3, rect.center().y),
                            egui::Align2::LEFT_CENTER,
                            cmd,
                            egui::FontId::new(SZ_XS, egui::FontFamily::Monospace),
                            if active { ACCENT } else { col },
                        );
                        ui.painter().text(
                            egui::pos2(rect.left() + 160.0, rect.center().y),
                            egui::Align2::LEFT_CENTER,
                            hint,
                            egui::FontId::new(SZ_XS, egui::FontFamily::Proportional),
                            col,
                        );
                        if resp.clicked() {
                            run_palette_action(
                                id,
                                show_handoff,
                                handoff_start,
                                new_task_open,
                                nav,
                                main_tab,
                                paused,
                            );
                            *show = false;
                        }
                        if resp.hovered() {
                            ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
                            *sel = i;
                        }
                    }
                });
        });
}

fn run_palette_action(
    id: &str,
    show_handoff: &mut bool,
    handoff_start: &mut Option<Instant>,
    new_task_open: &mut bool,
    nav: &mut NavPage,
    main_tab: &mut MainTab,
    paused: &mut bool,
) {
    match id {
        "new_task" => {
            *new_task_open = true;
        }
        "handoff" => {
            crate::api::send_handoff();
            *show_handoff = true;
            *handoff_start = Some(Instant::now());
        }
        "pause" => {
            *paused = !*paused;
            crate::api::send_pause(*paused);
        }
        "nav_dashboard" => *nav = NavPage::Dashboard,
        "nav_detect" => *nav = NavPage::Detect,
        "nav_projects" => *nav = NavPage::Projects,
        "nav_graph" => *nav = NavPage::Graph,
        "nav_profiles" => *nav = NavPage::Profiles,
        "nav_audit" => *nav = NavPage::Audit,
        "nav_settings" => *nav = NavPage::Settings,
        "tab_diff" => {
            *nav = NavPage::Dashboard;
            *main_tab = MainTab::Diff;
        }
        "tab_contract" => {
            *nav = NavPage::Dashboard;
            *main_tab = MainTab::Contract;
        }
        "tab_stream" => {
            *nav = NavPage::Dashboard;
            *main_tab = MainTab::EventStream;
        }
        _ => {}
    }
}

fn draw_handoff_overlay(
    ctx: &egui::Context,
    state: &DashboardState,
    start: Instant,
    show: &mut bool,
) {
    let elapsed = start.elapsed().as_secs_f32();
    let phase: u32 = if elapsed < 0.8 {
        0
    } else if elapsed < 2.0 {
        1
    } else if elapsed < 3.2 {
        2
    } else if elapsed < 4.6 {
        3
    } else {
        4
    };

    // Dark backdrop
    let screen = ctx.screen_rect();
    let backdrop_layer = egui::LayerId::new(egui::Order::Background, egui::Id::new("handoff_bg"));
    ctx.layer_painter(backdrop_layer).rect_filled(
        screen,
        Rounding::ZERO,
        Color32::from_rgba_premultiplied(4, 4, 4, 230),
    );

    let labels = [
        "Pausing agent…",
        "Sealing contract…",
        "Dispatching…",
        "✓ Live",
    ];
    let progress = [3u32, 32, 68, 100];
    let phase_clamped = phase.min(3) as usize;

    // Get "next" provider name
    let next_provider = state
        .providers
        .iter()
        .find(|p| p.is_next)
        .map(|p| p.name.as_str())
        .unwrap_or("next");
    let active_provider = state
        .providers
        .iter()
        .find(|p| p.state == ProviderState::Active)
        .map(|p| p.name.as_str())
        .unwrap_or("claude");

    egui::Window::new("handoff_modal")
        .title_bar(false)
        .resizable(false)
        .collapsible(false)
        .anchor(egui::Align2::CENTER_CENTER, Vec2::ZERO)
        .fixed_size(Vec2::new(480.0, 280.0))
        .frame(
            Frame::none()
                .fill(BG1)
                .stroke(Stroke::new(1.0, BORDER1))
                .rounding(R_LG),
        )
        .show(ctx, |ui| {
            // Hero section
            Frame::none()
                .fill(BG2)
                .inner_margin(egui::Margin::symmetric(SP4 + SP2, SP4))
                .show(ui, |ui| {
                    // Relay logomark >—<
                    ui.vertical_centered(|ui| {
                        ui.add_space(SP2);
                        let desired = Vec2::new(96.0, 28.0);
                        let (rect, _) = ui.allocate_exact_size(desired, Sense::hover());
                        let painter = ui.painter();
                        let cx = rect.center().x;
                        let cy = rect.center().y;
                        // Left chevron >
                        painter.line_segment(
                            [egui::pos2(cx - 36.0, cy - 11.0), egui::pos2(cx - 18.0, cy)],
                            Stroke::new(3.0, TX1),
                        );
                        painter.line_segment(
                            [egui::pos2(cx - 18.0, cy), egui::pos2(cx - 36.0, cy + 11.0)],
                            Stroke::new(3.0, TX1),
                        );
                        // Right chevron <
                        painter.line_segment(
                            [egui::pos2(cx + 36.0, cy - 11.0), egui::pos2(cx + 18.0, cy)],
                            Stroke::new(2.5, TX2),
                        );
                        painter.line_segment(
                            [egui::pos2(cx + 18.0, cy), egui::pos2(cx + 36.0, cy + 11.0)],
                            Stroke::new(2.5, TX2),
                        );
                        // Bridge (draws in based on phase)
                        if phase >= 1 {
                            let bridge_x = cx - 18.0 + (phase as f32 - 1.0).min(1.0) * 36.0;
                            painter.line_segment(
                                [egui::pos2(cx - 18.0, cy), egui::pos2(bridge_x, cy)],
                                Stroke::new(3.0, ACCENT),
                            );
                        }

                        ui.add_space(SP3);

                        // Provider transfer row
                        ui.horizontal(|ui| {
                            ui.add_space(60.0);
                            ui.vertical(|ui| {
                                ui.label(
                                    RichText::new(active_provider)
                                        .color(TX0)
                                        .size(16.0)
                                        .monospace()
                                        .strong(),
                                );
                                ui.label(
                                    RichText::new("pausing").color(YELLOW).size(9.0).monospace(),
                                );
                            });
                            ui.add_space(SP4 + SP2);
                            ui.label(RichText::new("→").color(ACCENT).size(16.0));
                            ui.add_space(SP4 + SP2);
                            ui.vertical(|ui| {
                                let nc = if phase >= 4 { TX0 } else { TX2 };
                                ui.label(
                                    RichText::new(next_provider)
                                        .color(nc)
                                        .size(16.0)
                                        .monospace()
                                        .strong(),
                                );
                                let sc = if phase >= 4 { "● active" } else { "standby" };
                                let sc_col = if phase >= 4 { GREEN } else { TX3 };
                                ui.label(RichText::new(sc).color(sc_col).size(9.0).monospace());
                            });
                        });
                    });
                });

            h_rule(ui);

            // Contract details (phase 1+)
            Frame::none()
                .inner_margin(egui::Margin::symmetric(SP4 + SP2, SP3))
                .show(ui, |ui| {
                    if phase >= 1 {
                        ui.horizontal(|ui| {
                            ui.label(
                                RichText::new("Continuation contract")
                                    .color(TX2)
                                    .size(9.0)
                                    .monospace(),
                            );
                            if phase >= 2 {
                                ui.add_space(SP2);
                                dot(ui, GREEN, 4.0);
                                ui.label(
                                    RichText::new("signed · v1.0.0")
                                        .color(GREEN)
                                        .size(9.0)
                                        .monospace(),
                                );
                            }
                        });
                        ui.add_space(SP2);
                    }

                    // Progress bar
                    let prog_val = progress[phase_clamped] as f32 / 100.0;
                    let bar_col = if phase >= 4 { GREEN } else { ACCENT };
                    let (rect, _) = ui.allocate_exact_size(
                        Vec2::new(ui.available_width() - 180.0, 2.0),
                        Sense::hover(),
                    );
                    ui.painter().rect_filled(rect, R_SM, BG4);
                    let fill_w = rect.width() * prog_val;
                    ui.painter().rect_filled(
                        Rect::from_min_size(rect.min, Vec2::new(fill_w, rect.height())),
                        R_SM,
                        bar_col,
                    );

                    ui.add_space(SP3);
                    ui.horizontal(|ui| {
                        ui.label(
                            RichText::new(labels[phase_clamped])
                                .color(if phase >= 4 { GREEN } else { TX1 })
                                .size(SZ_XS)
                                .monospace(),
                        );
                        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                            if phase >= 4 && btn_primary(ui, "Continue →").clicked() {
                                *show = false;
                            }
                        });
                    });
                });
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// NEW TASK MODAL
// ═══════════════════════════════════════════════════════════════════════════

// (used by titlebar "New task" button and setup step 3)
// Rendered in update() after all panels via egui::Window

// ═══════════════════════════════════════════════════════════════════════════
// PAGE CHROME HELPERS
// ═══════════════════════════════════════════════════════════════════════════

fn page_header(ui: &mut Ui, title: &str, subtitle: &str) {
    egui::TopBottomPanel::top("page_header")
        .exact_height(46.0)
        .frame(
            Frame::none()
                .fill(BG2)
                .inner_margin(egui::Margin::symmetric(SP5, 0.0)),
        )
        .show_separator_line(true)
        .show_inside(ui, |ui| {
            ui.horizontal_centered(|ui| {
                ui.label(RichText::new(title).color(TX0).strong().size(SZ_MD));
                ui.add_space(SP2);
                ui.label(RichText::new(subtitle).color(TX2).size(SZ_XS));
            });
        });
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════
// ICON PAINTER — exact SVG paths from Relay Dashboard.html IC object
// ═══════════════════════════════════════════════════════════════════════════

/// Paint a named icon centered at `center`, drawn in a `size`×`size` bounding box.
/// Coordinates map from SVG 16×16 viewBox.
fn paint_icon(painter: &egui::Painter, center: egui::Pos2, size: f32, icon: &str, color: Color32) {
    let s = size / 16.0; // scale factor: 1 SVG unit → s pixels
    let ox = center.x - size * 0.5;
    let oy = center.y - size * 0.5;
    let p = |x: f32, y: f32| egui::pos2(ox + x * s, oy + y * s);
    let rr = |v: f32| v * s;

    let sw = Stroke::new(1.35 * s, color);
    let sw2 = Stroke::new(1.1 * s, color);
    let sw3 = Stroke::new(1.25 * s, color);
    let faint = {
        let [cr, cg, cb, _] = color.to_array();
        Stroke::new(
            1.0 * s,
            Color32::from_rgba_premultiplied(
                (cr as u16 * 40 / 100) as u8,
                (cg as u16 * 40 / 100) as u8,
                (cb as u16 * 40 / 100) as u8,
                100,
            ),
        )
    };

    match icon {
        // ── 2×2 grid ────────────────────────────────────────────────────────
        "projects" => {
            let rn = Rounding::same(rr(1.2));
            painter.rect_stroke(egui::Rect::from_min_max(p(1.5, 1.5), p(7.0, 7.0)), rn, sw);
            painter.rect_stroke(egui::Rect::from_min_max(p(9.0, 1.5), p(14.5, 7.0)), rn, sw);
            painter.rect_stroke(egui::Rect::from_min_max(p(1.5, 9.0), p(7.0, 14.5)), rn, sw);
            painter.rect_stroke(egui::Rect::from_min_max(p(9.0, 9.0), p(14.5, 14.5)), rn, sw);
        }

        // ── asymmetric panel layout ─────────────────────────────────────────
        "dashboard" => {
            let rn = Rounding::same(rr(1.2));
            painter.rect_stroke(egui::Rect::from_min_max(p(2.0, 2.0), p(7.5, 7.5)), rn, sw);
            painter.rect_stroke(egui::Rect::from_min_max(p(2.0, 9.5), p(7.5, 14.0)), rn, sw);
            painter.rect_stroke(egui::Rect::from_min_max(p(9.5, 2.0), p(14.0, 11.0)), rn, sw);
            painter.line_segment([p(9.5, 13.0), p(14.0, 13.0)], faint);
        }

        // ── 3-node connected graph ──────────────────────────────────────────
        "graph" => {
            painter.circle_stroke(p(8.0, 4.0), rr(2.0), sw);
            painter.circle_stroke(p(3.0, 12.5), rr(2.0), sw);
            painter.circle_stroke(p(13.0, 12.5), rr(2.0), sw);
            painter.line_segment([p(6.3, 5.4), p(4.2, 10.8)], sw2);
            painter.line_segment([p(9.7, 5.4), p(11.8, 10.8)], sw2);
            painter.line_segment([p(5.0, 12.5), p(11.0, 12.5)], sw2);
        }

        // ── two horizontal rows (profiles/list) ────────────────────────────
        "profiles" => {
            let rn = Rounding::same(rr(1.2));
            painter.rect_stroke(egui::Rect::from_min_max(p(1.5, 2.0), p(14.5, 6.0)), rn, sw);
            painter.rect_stroke(egui::Rect::from_min_max(p(1.5, 8.0), p(14.5, 12.0)), rn, sw);
            painter.line_segment([p(4.0, 4.0), p(12.0, 4.0)], faint);
            painter.line_segment([p(4.0, 10.0), p(12.0, 10.0)], faint);
        }

        // ── shield with checkmark ───────────────────────────────────────────
        "audit" => {
            // Shield outline: M8 1.5 L14 4.5 V8 … V4.5 L8 1.5
            painter.line_segment([p(8.0, 1.5), p(14.0, 4.5)], sw);
            painter.line_segment([p(8.0, 1.5), p(2.0, 4.5)], sw);
            painter.line_segment([p(14.0, 4.5), p(14.0, 8.0)], sw);
            painter.line_segment([p(2.0, 4.5), p(2.0, 8.0)], sw);
            // Curved bottom — approximate with two segments meeting at tip
            painter.line_segment([p(14.0, 8.0), p(8.0, 14.5)], sw);
            painter.line_segment([p(2.0, 8.0), p(8.0, 14.5)], sw);
            // Checkmark: M5.5 8l2 2 3-3.5
            painter.line_segment([p(5.5, 8.0), p(7.5, 10.0)], sw3);
            painter.line_segment([p(7.5, 10.0), p(10.5, 6.5)], sw3);
        }

        // ── gear/cog ────────────────────────────────────────────────────────
        "settings" => {
            painter.circle_stroke(p(8.0, 8.0), rr(2.3), sw);
            // 8 spokes from path "M8 2v1.5M8 12.5V14M2 8h1.5M12.5 8H14…"
            for (a, b) in [
                (p(8.0, 2.0), p(8.0, 3.5)),
                (p(8.0, 12.5), p(8.0, 14.0)),
                (p(2.0, 8.0), p(3.5, 8.0)),
                (p(12.5, 8.0), p(14.0, 8.0)),
                (p(3.5, 3.5), p(4.5, 4.5)),
                (p(11.5, 11.5), p(12.5, 12.5)),
                (p(3.5, 12.5), p(4.5, 11.5)),
                (p(11.5, 4.5), p(12.5, 3.5)),
            ] {
                painter.line_segment([a, b], sw3);
            }
        }

        // ── folder (14×14 viewBox) ──────────────────────────────────────────
        // path "M1.5 4.5C…3H5.8l1.8 2H11.5c…v5.1c…H3c…V4.5z"
        "folder" => {
            let fs = size / 14.0;
            let fp = |x: f32, y: f32| egui::pos2(ox + x * fs, oy + y * fs);
            let fsw = Stroke::new(1.3 * fs, color);
            // Body
            painter.line_segment([fp(1.5, 5.0), fp(1.5, 11.5)], fsw);
            painter.line_segment([fp(1.5, 11.5), fp(12.5, 11.5)], fsw);
            painter.line_segment([fp(12.5, 11.5), fp(12.5, 5.5)], fsw);
            // Tab top
            painter.line_segment([fp(12.5, 5.5), fp(7.0, 5.5)], fsw);
            painter.line_segment([fp(7.0, 5.5), fp(5.5, 3.5)], fsw);
            painter.line_segment([fp(5.5, 3.5), fp(2.5, 3.5)], fsw);
            painter.line_segment([fp(2.5, 3.5), fp(1.5, 4.5)], fsw);
            painter.line_segment([fp(1.5, 4.5), fp(1.5, 5.0)], fsw);
        }

        // ── arrow right (9×8 viewBox) ───────────────────────────────────────
        "arrowRight" => {
            let aw = size / 9.0;
            let ah = size / 8.0;
            let ap = |x: f32, y: f32| egui::pos2(ox + x * aw, oy + y * ah);
            let asw = Stroke::new(1.2 * aw, color);
            painter.line_segment([ap(0.0, 4.0), ap(6.5, 4.0)], asw);
            painter.line_segment([ap(5.0, 1.0), ap(8.5, 4.0)], asw);
            painter.line_segment([ap(5.0, 7.0), ap(8.5, 4.0)], asw);
        }

        // ── chevron right (7×10 viewBox) ───────────────────────────────────
        "chevRight" => {
            let cw = size / 7.0;
            let ch = size / 10.0;
            let cp = |x: f32, y: f32| egui::pos2(ox + x * cw, oy + y * ch);
            let csw = Stroke::new(1.2 * cw, color);
            painter.line_segment([cp(1.5, 1.5), cp(5.5, 5.0)], csw);
            painter.line_segment([cp(5.5, 5.0), cp(1.5, 8.5)], csw);
        }

        // ── layout icons (14×11 viewBox) ───────────────────────────────────
        "layout1" => {
            // narrow left + wide right (icon rail layout)
            let lw = size / 14.0;
            let lh = size / 11.0;
            let lp = |x: f32, y: f32| egui::pos2(ox + x * lw, oy + y * lh);
            let lsw = Stroke::new(1.1 * lw, color);
            let rn = Rounding::same(lw * 1.0);
            painter.rect_stroke(
                egui::Rect::from_min_max(lp(0.5, 0.5), lp(3.5, 10.5)),
                rn,
                lsw,
            );
            painter.rect_stroke(
                egui::Rect::from_min_max(lp(5.0, 0.5), lp(13.5, 10.5)),
                rn,
                lsw,
            );
        }
        "layout2" => {
            // medium left + medium right (full sidebar layout)
            let lw = size / 14.0;
            let lh = size / 11.0;
            let lp = |x: f32, y: f32| egui::pos2(ox + x * lw, oy + y * lh);
            let lsw = Stroke::new(1.1 * lw, color);
            let rn = Rounding::same(lw * 1.0);
            painter.rect_stroke(
                egui::Rect::from_min_max(lp(0.5, 0.5), lp(5.5, 10.5)),
                rn,
                lsw,
            );
            painter.rect_stroke(
                egui::Rect::from_min_max(lp(7.0, 0.5), lp(13.5, 10.5)),
                rn,
                lsw,
            );
        }

        // ── Triangle pointing up (filled — easy to read at 14px) ───────────
        "up" => {
            painter.add(egui::Shape::convex_polygon(
                vec![p(8.0, 4.0), p(12.0, 11.0), p(4.0, 11.0)],
                color,
                Stroke::NONE,
            ));
        }
        "down" => {
            painter.add(egui::Shape::convex_polygon(
                vec![p(8.0, 12.0), p(4.0, 5.0), p(12.0, 5.0)],
                color,
                Stroke::NONE,
            ));
        }

        // ── × close / remove ───────────────────────────────────────────────
        "x" => {
            let sw = Stroke::new(1.5 * s, color);
            painter.line_segment([p(4.5, 4.5), p(11.5, 11.5)], sw);
            painter.line_segment([p(11.5, 4.5), p(4.5, 11.5)], sw);
        }

        // ── + add ──────────────────────────────────────────────────────────
        "plus" => {
            let sw = Stroke::new(1.5 * s, color);
            painter.line_segment([p(8.0, 3.0), p(8.0, 13.0)], sw);
            painter.line_segment([p(3.0, 8.0), p(13.0, 8.0)], sw);
        }

        // ── − minus ────────────────────────────────────────────────────────
        "minus" => {
            let sw = Stroke::new(1.5 * s, color);
            painter.line_segment([p(3.0, 8.0), p(13.0, 8.0)], sw);
        }

        // ── ⊢ ⊣ drawer toggles ─────────────────────────────────────────────
        "drawer_close" => {
            let sw = Stroke::new(1.1 * s, color);
            painter.rect_stroke(
                egui::Rect::from_min_max(p(2.0, 3.0), p(14.0, 13.0)),
                Rounding::same(rr(1.0)),
                sw,
            );
            painter.line_segment([p(11.0, 3.0), p(11.0, 13.0)], sw);
        }
        "drawer_open" => {
            let sw = Stroke::new(1.1 * s, color);
            painter.rect_stroke(
                egui::Rect::from_min_max(p(2.0, 3.0), p(14.0, 13.0)),
                Rounding::same(rr(1.0)),
                sw,
            );
            painter.line_segment([p(5.0, 3.0), p(5.0, 13.0)], sw);
        }

        // ── radar / detect (concentric rings + sweep) ──────────────────────
        "detect" => {
            painter.circle_stroke(p(8.0, 8.0), rr(6.2), sw2);
            painter.circle_stroke(p(8.0, 8.0), rr(3.4), sw2);
            painter.circle_filled(p(8.0, 8.0), rr(1.3), color);
            painter.line_segment([p(8.0, 8.0), p(12.6, 3.4)], sw);
        }

        _ => {}
    }
}

/// Square icon-button (26×26). Replaces text-glyph buttons.
fn icon_btn(ui: &mut Ui, icon: &str, tooltip: &str) -> egui::Response {
    let (rect, resp) = ui.allocate_exact_size(Vec2::splat(26.0), Sense::click());
    if ui.is_rect_visible(rect) {
        let (fill, stroke_col, fg) = if resp.hovered() {
            (BTN_HOVER, BORDER2, TX0)
        } else {
            (BTN_BG, BORDER1, TX1)
        };
        ui.painter().rect_filled(rect, R, fill);
        ui.painter()
            .rect_stroke(rect, R, Stroke::new(1.0, stroke_col));
        paint_icon(ui.painter(), rect.center(), 14.0, icon, fg);
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
        if !tooltip.is_empty() {
            egui::show_tooltip_at_pointer(
                ui.ctx(),
                ui.layer_id(),
                egui::Id::new(("icon_tip", tooltip)),
                |ui| {
                    ui.label(RichText::new(tooltip).color(TX0).size(SZ_XS));
                },
            );
        }
    }
    resp
}

/// Text-label button with leading painted icon.
fn btn_with_icon(ui: &mut Ui, icon: &str, label: &str) -> egui::Response {
    let font = egui::FontId::new(SZ_XS, egui::FontFamily::Proportional);
    let galley = ui
        .painter()
        .layout_no_wrap(label.to_owned(), font.clone(), TX0);
    let icon_w = 12.0;
    let gap = 5.0;
    let pad_h = SP2 * 2.0;
    let pad_v = 6.0;
    let desired = Vec2::new(
        icon_w + gap + galley.size().x + pad_h,
        galley.size().y + pad_v,
    );
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
    if ui.is_rect_visible(rect) {
        let (fill, stroke_col, fg) = if resp.hovered() {
            (BTN_HOVER, BORDER2, TX0)
        } else {
            (BTN_BG, BORDER1, TX1)
        };
        ui.painter().rect_filled(rect, R, fill);
        ui.painter()
            .rect_stroke(rect, R, Stroke::new(1.0, stroke_col));
        let icon_cx = rect.left() + pad_h / 2.0 + icon_w / 2.0;
        paint_icon(
            ui.painter(),
            egui::pos2(icon_cx, rect.center().y),
            icon_w,
            icon,
            fg,
        );
        let text_x = icon_cx + icon_w / 2.0 + gap;
        ui.painter().text(
            egui::pos2(text_x, rect.center().y),
            egui::Align2::LEFT_CENTER,
            label,
            font,
            fg,
        );
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
    resp
}

/// Primary version of `btn_with_icon`.
fn btn_primary_with_icon(ui: &mut Ui, icon: &str, label: &str) -> egui::Response {
    let font = egui::FontId::new(SZ_XS, egui::FontFamily::Proportional);
    let galley = ui
        .painter()
        .layout_no_wrap(label.to_owned(), font.clone(), Color32::BLACK);
    let icon_w = 12.0;
    let gap = 5.0;
    let pad_h = SP2 * 2.0;
    let pad_v = 6.0;
    let desired = Vec2::new(
        icon_w + gap + galley.size().x + pad_h,
        galley.size().y + pad_v,
    );
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
    if ui.is_rect_visible(rect) {
        let fill = if resp.hovered() {
            Color32::from_rgb(0xea, 0x73, 0x44)
        } else {
            ACCENT
        };
        ui.painter().rect_filled(rect, R, fill);
        let dark = Color32::from_rgb(0x0a, 0x0a, 0x0a);
        let icon_cx = rect.left() + pad_h / 2.0 + icon_w / 2.0;
        paint_icon(
            ui.painter(),
            egui::pos2(icon_cx, rect.center().y),
            icon_w,
            icon,
            dark,
        );
        let text_x = icon_cx + icon_w / 2.0 + gap;
        ui.painter().text(
            egui::pos2(text_x, rect.center().y),
            egui::Align2::LEFT_CENTER,
            label,
            font,
            dark,
        );
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
    resp
}

// .stab from design: padding 8px 14px, border-bottom 1.5px accent when active.
// Paint everything via painter — no child widget to eat click events.
fn tab_btn(ui: &mut Ui, current: &mut MainTab, tab: MainTab, label: &str) {
    let active = *current == tab;
    let font = egui::FontId::new(12.0, egui::FontFamily::Proportional);

    let galley = ui
        .painter()
        .layout_no_wrap(label.to_owned(), font.clone(), Color32::WHITE);
    let desired = Vec2::new(galley.size().x + 28.0, 32.0); // 14px × 2 horiz pad
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());

    if ui.is_rect_visible(rect) {
        // Active: 1.5px orange bottom border (design .stab.active border-bottom-color)
        if active {
            ui.painter().rect_filled(
                Rect::from_min_size(
                    egui::pos2(rect.left(), rect.bottom() - 1.5),
                    Vec2::new(rect.width(), 1.5),
                ),
                Rounding::ZERO,
                ACCENT,
            );
        }

        // rgba(.3) inactive → rgba(.6) hover → #ececec active
        let col = if active {
            TX0
        } else if resp.hovered() {
            Color32::from_rgba_premultiplied(153, 153, 153, 153)
        } else {
            Color32::from_rgba_premultiplied(77, 77, 77, 77)
        };

        ui.painter()
            .text(rect.center(), egui::Align2::CENTER_CENTER, label, font, col);
    }

    if resp.clicked() {
        *current = tab;
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
}

/// Secondary button — rgba(.06) bg, rgba(.1) border, hover: rgba(.1) bg
fn btn(ui: &mut Ui, label: &str) -> egui::Response {
    let text = egui::RichText::new(label).size(SZ_XS);
    let galley = ui.painter().layout_no_wrap(
        label.to_owned(),
        egui::FontId::new(SZ_XS, egui::FontFamily::Proportional),
        TX0,
    );
    let pad = Vec2::new(SP2 * 2.0, 6.0);
    let desired = galley.size() + pad;
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
    if ui.is_rect_visible(rect) {
        let (fill, border_col, text_col) = if resp.hovered() {
            (BTN_HOVER, BORDER2, TX0)
        } else {
            (BTN_BG, BORDER1, TX1)
        };
        ui.painter().rect_filled(rect, R, fill);
        ui.painter()
            .rect_stroke(rect, R, Stroke::new(1.0, border_col));
        let tpos = rect.center() - galley.size() / 2.0;
        ui.painter().galley(tpos, galley, text_col);
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
    let _ = text;
    resp
}

/// Primary button — accent fill, dark text, hover: lighter accent
fn btn_primary(ui: &mut Ui, label: &str) -> egui::Response {
    let galley = ui.painter().layout_no_wrap(
        label.to_owned(),
        egui::FontId::proportional(SZ_XS),
        Color32::BLACK,
    );
    let pad = Vec2::new(SP2 * 2.0, 6.0);
    let desired = galley.size() + pad;
    let (rect, resp) = ui.allocate_exact_size(desired, Sense::click());
    if ui.is_rect_visible(rect) {
        let fill = if resp.hovered() {
            Color32::from_rgb(0xea, 0x73, 0x44) // #ea7344 hover
        } else {
            ACCENT
        };
        ui.painter().rect_filled(rect, R, fill);
        let tpos = rect.center() - galley.size() / 2.0;
        ui.painter()
            .galley(tpos, galley, Color32::from_rgb(0x0a, 0x0a, 0x0a));
    }
    if resp.hovered() {
        ui.ctx().set_cursor_icon(egui::CursorIcon::PointingHand);
    }
    resp
}

fn dot(ui: &mut Ui, color: Color32, size: f32) {
    let (rect, _) = ui.allocate_exact_size(Vec2::splat(size), Sense::hover());
    ui.painter().circle_filled(rect.center(), size / 2.0, color);
}

fn sparkline(ui: &mut Ui, history: &[f32], h: f32) {
    let desired = Vec2::new(ui.available_width(), h);
    let (rect, _) = ui.allocate_exact_size(desired, Sense::hover());
    if history.is_empty() {
        return;
    }
    let max = history.iter().cloned().fold(0.0_f32, f32::max).max(0.001);
    let bar_w = (rect.width() / history.len() as f32 - 2.0).max(2.0);
    for (i, &v) in history.iter().enumerate() {
        let x = rect.left() + i as f32 * (bar_w + 2.0);
        let bar_h = rect.height() * (v / max).min(1.0);
        let top_y = rect.bottom() - bar_h;
        let bar_rect = Rect::from_min_size(egui::pos2(x, top_y), Vec2::new(bar_w, bar_h));
        let col = if i == history.len() - 1 {
            TX0
        } else if v >= 0.8 {
            GREEN
        } else if v >= 0.6 {
            TX2
        } else {
            BG4
        };
        ui.painter().rect_filled(bar_rect, Rounding::same(1.5), col);
    }
}

fn h_rule(ui: &mut Ui) {
    ui.add(egui::Separator::default().shrink(0.0).spacing(0.0));
}

fn tag_style(tag: &EventTag) -> (Color32, Color32, &'static str) {
    match tag {
        EventTag::ToolUse => (BG4, TX1, "tool"),
        EventTag::Result => (GREEN_BG, GREEN, "result"),
        EventTag::Quota => (YELLOW_BG, YELLOW, "quota"),
        EventTag::Handoff => (ACCENT_BG, ACCENT, "handoff"),
        EventTag::System => (BG3, TX2, "system"),
        EventTag::Text => (BLUE_BG, BLUE, "text"),
        EventTag::Waiting => (Color32::TRANSPARENT, TX3, "wait"),
    }
}

fn fmt_tokens(n: u64) -> String {
    if n >= 1_000_000 {
        format!("{:.1}M", n as f64 / 1_000_000.0)
    } else if n >= 1_000 {
        format!("{:.0}K", n as f64 / 1_000.0)
    } else {
        n.to_string()
    }
}

fn short_sha(sha: &str) -> String {
    if sha.len() > 12 {
        format!("{}…", &sha[..12])
    } else if sha.is_empty() {
        "—".into()
    } else {
        sha.into()
    }
}
