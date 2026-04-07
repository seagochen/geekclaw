//! # geekclaw-plugin
//!
//! 外部插件进程的 JSON-RPC 2.0 over stdio 通信基础设施。
//!
//! 本模块为所有插件桥接层（渠道、命令、工具、语音、LLM 提供者）
//! 提供统一的线路类型、传输层和进程生命周期管理，避免重复实现。
//!
//! ## 架构
//!
//! ```text
//! PluginProcess::spawn()
//!   → 启动子进程
//!   → 创建 Transport（stdin 写入 / stdout 读取）
//!   → 后台 read_loop 三路分发（响应 / 反向调用 / 通知）
//!   → 初始化握手
//! ```

pub mod config;
pub mod error;
pub mod process;
pub mod transport;
pub mod wire;

// 重新导出常用类型
pub use config::PluginConfig;
pub use error::PluginError;
pub use process::{PluginProcess, SpawnOpts};
pub use transport::{ServiceHandler, Transport};
pub use wire::{JsonRpcNotification, JsonRpcRequest, JsonRpcResponse, RpcError};
