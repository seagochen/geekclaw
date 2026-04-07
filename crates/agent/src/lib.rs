//! GeekClaw Agent 核心模块。
//!
//! 包含 Agent 主循环、LLM 调用编排和工具执行。

mod context;
mod loop_core;

pub use context::ContextBuilder;
pub use loop_core::AgentLoop;

use geekclaw_config::Config;
use geekclaw_providers::FallbackCandidate;

/// Agent 实例配置（从 Config 派生）。
#[derive(Debug, Clone)]
pub struct AgentInstance {
    /// Agent 标识符。
    pub id: String,
    /// 使用的模型。
    pub model: String,
    /// 备选模型列表。
    pub fallback_models: Vec<String>,
    /// 默认 LLM Provider 名称。
    pub default_provider: String,
    /// 最大工具迭代次数。
    pub max_iterations: usize,
    /// 会话超时（秒）。
    pub session_timeout: u64,
    /// 最大输出 token 数。
    pub max_tokens: Option<u32>,
    /// 温度参数。
    pub temperature: Option<f64>,
    /// 系统提示词。
    pub system_prompt: String,
    /// 最大上下文 token 数。
    pub max_context_tokens: usize,
}

impl AgentInstance {
    /// 从配置创建默认 Agent 实例。
    pub fn from_config(cfg: &Config) -> Self {
        let defaults = &cfg.agents.defaults;
        Self {
            id: "default".into(),
            model: defaults.model.clone(),
            fallback_models: Vec::new(),
            default_provider: String::new(),
            max_iterations: defaults.max_tool_iterations,
            session_timeout: defaults.session_timeout,
            max_tokens: None,
            temperature: None,
            system_prompt: defaults.system_prompt.clone(),
            max_context_tokens: defaults.max_context_tokens,
        }
    }

    /// 解析故障转移候选者列表。
    pub fn candidates(&self) -> Vec<FallbackCandidate> {
        let cfg = geekclaw_providers::ModelConfig {
            primary: self.model.clone(),
            fallbacks: self.fallback_models.clone(),
        };
        geekclaw_providers::FallbackChain::resolve_candidates(&cfg, &self.default_provider)
    }
}

/// 消息处理选项。
#[derive(Debug, Clone, Default)]
pub struct ProcessOptions {
    /// 会话标识符。
    pub session_key: String,
    /// 目标渠道。
    pub channel: String,
    /// 目标聊天 ID。
    pub chat_id: String,
    /// 用户消息内容。
    pub user_message: String,
    /// 是否发送响应到总线。
    pub send_response: bool,
    /// 是否跳过加载历史。
    pub no_history: bool,
    /// 工作目录覆盖。
    pub working_dir: Option<String>,
}
