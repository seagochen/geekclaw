//! GeekClaw 结构化日志模块。
//!
//! 基于 `tracing` + `tracing-subscriber`，提供统一的日志初始化。

use tracing_subscriber::{EnvFilter, fmt, layer::SubscriberExt, util::SubscriberInitExt};

/// 初始化全局日志。
///
/// 日志级别通过 `GEEKCLAW_LOG` 环境变量控制，默认 `info`。
/// JSON 格式通过 `GEEKCLAW_LOG_JSON=1` 启用。
pub fn init() {
    let filter = EnvFilter::try_from_env("GEEKCLAW_LOG")
        .unwrap_or_else(|_| EnvFilter::new("info"));

    let use_json = std::env::var("GEEKCLAW_LOG_JSON")
        .map(|v| v == "1" || v == "true")
        .unwrap_or(false);

    let registry = tracing_subscriber::registry().with(filter);

    if use_json {
        registry.with(fmt::layer().json()).init();
    } else {
        registry
            .with(fmt::layer().with_target(true).with_thread_ids(false))
            .init();
    }
}
