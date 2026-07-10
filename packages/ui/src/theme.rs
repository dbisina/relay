//! theme.rs — Relay design tokens.
//!
//! Palette from design/Relay Dashboard.html:
//!   base:#080808  t0:#ececec  accent:#e06a38
//!   s1:  #101010  t1: 50% wh  green: #3aaa70
//!   s2:  #151515  t2: 25% wh  yellow:#cc9420
//!   s3:  #1c1c1c  t3: 12% wh  red:   #d04040
//!   s4:  #232323              blue:  #4a92d8
//!
//! Rounding: 4 px (softened, refined).

use egui::{Color32, Rounding, Stroke, Style, Visuals};

// ── Surface layers (4 depths) ────────────────────────────────────────────────
pub const BG0: Color32 = Color32::from_rgb(0x08, 0x08, 0x08); // base — darkest
pub const BG1: Color32 = Color32::from_rgb(0x10, 0x10, 0x10); // titlebar, sidebar
pub const BG2: Color32 = Color32::from_rgb(0x15, 0x15, 0x15); // provider bar, sub-headers
pub const BG3: Color32 = Color32::from_rgb(0x1c, 0x1c, 0x1c); // cards, inputs
pub const BG4: Color32 = Color32::from_rgb(0x23, 0x23, 0x23); // hover

// ── Borders — premultiplied RGBA whites ────────────────────────────────────
// b0 = rgba(255,255,255,.05), b1 = .08, b2 = .13
pub const BORDER0: Color32 = Color32::from_rgba_premultiplied(13, 13, 13, 13);
pub const BORDER1: Color32 = Color32::from_rgba_premultiplied(20, 20, 20, 20);
pub const BORDER2: Color32 = Color32::from_rgba_premultiplied(33, 33, 33, 33);

// ── Text ─────────────────────────────────────────────────────────────────────
pub const TX0: Color32 = Color32::from_rgb(0xec, 0xec, 0xec);
// TX1 = 50% white, TX2 = 25%, TX3 = 12%
pub const TX1: Color32 = Color32::from_rgba_premultiplied(127, 127, 127, 127);
pub const TX2: Color32 = Color32::from_rgba_premultiplied(64, 64, 64, 64);
pub const TX3: Color32 = Color32::from_rgba_premultiplied(31, 31, 31, 31);

// ── Accent ────────────────────────────────────────────────────────────────────
pub const ACCENT: Color32 = Color32::from_rgb(0xe0, 0x6a, 0x38);
// 12% orange on dark — premultiplied: r=27 g=13 b=7 a=31
pub const ACCENT_BG: Color32 = Color32::from_rgba_premultiplied(27, 13, 7, 31);

// ── Semantic ─────────────────────────────────────────────────────────────────
pub const GREEN: Color32 = Color32::from_rgb(0x3a, 0xaa, 0x70);
// 10% green — premultiplied: 6,17,11 a=26
pub const GREEN_BG: Color32 = Color32::from_rgba_premultiplied(6, 17, 11, 26);
pub const YELLOW: Color32 = Color32::from_rgb(0xcc, 0x94, 0x20);
pub const YELLOW_BG: Color32 = Color32::from_rgba_premultiplied(20, 15, 3, 26);
pub const RED: Color32 = Color32::from_rgb(0xd0, 0x40, 0x40);
pub const BLUE: Color32 = Color32::from_rgb(0x4a, 0x92, 0xd8);
pub const BLUE_BG: Color32 = Color32::from_rgba_premultiplied(7, 15, 22, 26);

// ── Interaction backgrounds — premultiplied RGBA whites ───────────────────────
// nav-item hover:  rgba(255,255,255,.04) = 10
pub const NAV_HOVER: Color32 = Color32::from_rgba_premultiplied(10, 10, 10, 10);
// nav-item active: rgba(255,255,255,.05) = 13  (= BORDER0)
pub const NAV_ACTIVE: Color32 = Color32::from_rgba_premultiplied(13, 13, 13, 13);
// rail-btn hover:  rgba(255,255,255,.05)
pub const RAIL_HOVER: Color32 = Color32::from_rgba_premultiplied(13, 13, 13, 13);
// rail-btn active: rgba(255,255,255,.07) = 18
pub const RAIL_ACTIVE: Color32 = Color32::from_rgba_premultiplied(18, 18, 18, 18);
// btn bg:          rgba(255,255,255,.06) = 15
pub const BTN_BG: Color32 = Color32::from_rgba_premultiplied(15, 15, 15, 15);
// btn hover bg:    rgba(255,255,255,.10) = 26
pub const BTN_HOVER: Color32 = Color32::from_rgba_premultiplied(26, 26, 26, 26);
// nav inactive text: rgba(255,255,255,.38) = 97
pub const NAV_TX: Color32 = Color32::from_rgba_premultiplied(97, 97, 97, 97);
// ev-row hover: rgba(255,255,255,.025) = 6
pub const ROW_HOVER: Color32 = Color32::from_rgba_premultiplied(6, 6, 6, 6);

// ── Rounding ─────────────────────────────────────────────────────────────────
pub const R: Rounding = Rounding::same(4.0);
pub const R_SM: Rounding = Rounding::same(3.0);
pub const R_LG: Rounding = Rounding::same(7.0);
pub const R_PILL: Rounding = Rounding::same(100.0);

// ── Font sizes ───────────────────────────────────────────────────────────────
pub const SZ_XS: f32 = 11.0;
pub const SZ_SM: f32 = 12.0;
pub const SZ_MD: f32 = 13.0;
#[allow(dead_code)]
pub const SZ_LG: f32 = 15.0;

// ── Spacing ───────────────────────────────────────────────────────────────────
pub const SP1: f32 = 4.0;
pub const SP2: f32 = 8.0;
pub const SP3: f32 = 12.0;
pub const SP4: f32 = 16.0;
pub const SP5: f32 = 22.0;

/// Apply Relay theme to an egui Context.
pub fn apply(ctx: &egui::Context) {
    let mut style = Style::default();
    let mut visuals = Visuals::dark();

    // Surfaces
    visuals.panel_fill = BG0;
    visuals.window_fill = BG1;
    visuals.faint_bg_color = BG1;
    visuals.extreme_bg_color = BG0;
    visuals.code_bg_color = BG3;
    visuals.hyperlink_color = ACCENT;
    visuals.warn_fg_color = YELLOW;
    visuals.error_fg_color = RED;
    visuals.selection.bg_fill = Color32::from_rgba_premultiplied(57, 27, 14, 64);

    // Rounding
    visuals.window_rounding = R_LG;
    visuals.menu_rounding = R;
    visuals.popup_shadow = egui::epaint::Shadow::NONE;
    visuals.window_shadow = egui::epaint::Shadow::NONE;

    let b0 = Stroke::new(1.0, BORDER0);
    let b1 = Stroke::new(1.0, BORDER1);
    let b2 = Stroke::new(1.0, BORDER2);

    visuals.widgets.noninteractive.bg_fill = BG1;
    visuals.widgets.noninteractive.bg_stroke = b0;
    visuals.widgets.noninteractive.fg_stroke = Stroke::new(1.0, TX2);
    visuals.widgets.noninteractive.rounding = R;

    visuals.widgets.inactive.bg_fill = BG3;
    visuals.widgets.inactive.bg_stroke = b1;
    visuals.widgets.inactive.fg_stroke = Stroke::new(1.0, TX1);
    visuals.widgets.inactive.rounding = R;

    visuals.widgets.hovered.bg_fill = BG4;
    visuals.widgets.hovered.bg_stroke = b2;
    visuals.widgets.hovered.fg_stroke = Stroke::new(1.0, TX0);
    visuals.widgets.hovered.rounding = R;
    visuals.widgets.hovered.expansion = 0.0;

    visuals.widgets.active.bg_fill = BG4;
    visuals.widgets.active.bg_stroke = b2;
    visuals.widgets.active.fg_stroke = Stroke::new(1.0, TX0);
    visuals.widgets.active.rounding = R;
    visuals.widgets.active.expansion = 0.0;

    visuals.widgets.open.bg_fill = BG3;
    visuals.widgets.open.bg_stroke = b2;
    visuals.widgets.open.fg_stroke = Stroke::new(1.0, TX0);
    visuals.widgets.open.rounding = R;

    visuals.override_text_color = Some(TX0);

    style.visuals = visuals;
    style.spacing.item_spacing = egui::vec2(SP2, SP1);
    style.spacing.window_margin = egui::Margin::same(0.0);
    style.spacing.button_padding = egui::vec2(SP2, SP1);
    style.spacing.scroll = egui::style::ScrollStyle {
        bar_width: 4.0,
        ..Default::default()
    };

    ctx.set_style(style);
}
