//! main.rs — Relay desktop UI entry point (eframe).
//!
//! Run: cargo run -p relay-ui
//! Build release: cargo build -p relay-ui --release

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")] // hide console on Windows release
#![allow(unknown_lints)] // tolerate lints not present in older toolchains
#![allow(float_literal_f32_fallback)] // egui APIs are f32; float literals coerce intentionally

mod api;
mod app;
mod theme;
mod types;

use eframe::NativeOptions;
use egui::Vec2;

fn main() -> eframe::Result<()> {
    let options = NativeOptions {
        viewport: egui::ViewportBuilder::default()
            .with_title("Relay")
            .with_inner_size(Vec2::new(1280.0, 780.0))
            .with_min_inner_size(Vec2::new(800.0, 540.0))
            .with_resizable(true)
            .with_icon(load_icon()),
        // Disable window-size persistence — prevents startup as a tiny box
        // if eframe previously saved a small size.
        centered: true,
        ..Default::default()
    };

    eframe::run_native(
        "Relay",
        options,
        Box::new(|cc| Ok(Box::new(app::RelayApp::new(cc)))),
    )
}

fn load_icon() -> egui::IconData {
    let bytes = include_bytes!("../assets/Relay.png");
    match image::load_from_memory(bytes) {
        Ok(img) => {
            let rgba = img.into_rgba8();
            let (w, h) = rgba.dimensions();
            egui::IconData {
                rgba: rgba.into_raw(),
                width: w,
                height: h,
            }
        }
        Err(_) => egui::IconData {
            rgba: vec![0xe0, 0x6a, 0x38, 0xff],
            width: 1,
            height: 1,
        },
    }
}
