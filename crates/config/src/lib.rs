//! GeekClaw 配置加载模块。
//!
//! 从 YAML 文件加载配置，支持 `GEEKCLAW_` 前缀环境变量覆盖。

use serde::Deserialize;
use std::path::Path;

mod error;
pub use error::ConfigError;

/// 顶层配置结构。
#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct Config {
    pub agents: AgentsConfig,
    pub tools: ToolsConfig,
    pub providers: Vec<ProviderConfig>,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            agents: AgentsConfig::default(),
            tools: ToolsConfig::default(),
            providers: Vec::new(),
        }
    }
}

/// Agent 默认配置。
#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct AgentsConfig {
    pub defaults: AgentDefaults,
}

impl Default for AgentsConfig {
    fn default() -> Self {
        Self {
            defaults: AgentDefaults::default(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct AgentDefaults {
    /// 默认 LLM 模型标识。
    pub model: String,
    /// 最大工具迭代次数。
    pub max_tool_iterations: usize,
    /// 单次会话超时（秒）。
    pub session_timeout: u64,
    /// 系统提示词。
    pub system_prompt: String,
    /// 最大上下文 token 数。
    pub max_context_tokens: usize,
}

impl Default for AgentDefaults {
    fn default() -> Self {
        Self {
            model: "gpt-4o".into(),
            max_tool_iterations: 10,
            session_timeout: 300,
            system_prompt: String::new(),
            max_context_tokens: 128_000,
        }
    }
}

/// 工具相关配置。
#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct ToolsConfig {
    pub cron: CronToolConfig,
}

impl Default for ToolsConfig {
    fn default() -> Self {
        Self {
            cron: CronToolConfig::default(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct CronToolConfig {
    /// 定时任务执行超时（分钟）。
    pub exec_timeout_minutes: u64,
}

impl Default for CronToolConfig {
    fn default() -> Self {
        Self {
            exec_timeout_minutes: 5,
        }
    }
}

/// LLM Provider 配置。
#[derive(Debug, Clone, Deserialize)]
pub struct ProviderConfig {
    /// Provider 名称（如 "openai"、"anthropic"）。
    pub name: String,
    /// API base URL。
    pub base_url: Option<String>,
    /// API key（支持环境变量引用，如 `$OPENAI_API_KEY`）。
    pub api_key: Option<String>,
    /// 默认模型。
    pub model: Option<String>,
    /// 备选模型列表。
    #[serde(default)]
    pub fallback_models: Vec<String>,
}

/// 从 YAML 文件加载配置。
pub fn load<P: AsRef<Path>>(path: P) -> Result<Config, ConfigError> {
    let content = std::fs::read_to_string(path.as_ref())
        .map_err(|e| ConfigError::Io(path.as_ref().to_path_buf(), e))?;
    let mut config: Config =
        serde_yaml::from_str(&content).map_err(ConfigError::Parse)?;
    apply_env_overrides(&mut config);
    resolve_env_refs(&mut config);
    Ok(config)
}

/// 加载默认配置（不读取文件）。
pub fn load_default() -> Config {
    let mut config = Config::default();
    apply_env_overrides(&mut config);
    config
}

/// 应用 `GEEKCLAW_` 前缀的环境变量覆盖。
fn apply_env_overrides(config: &mut Config) {
    if let Ok(v) = std::env::var("GEEKCLAW_MODEL") {
        config.agents.defaults.model = v;
    }
    if let Ok(v) = std::env::var("GEEKCLAW_MAX_TOOL_ITERATIONS") {
        if let Ok(n) = v.parse() {
            config.agents.defaults.max_tool_iterations = n;
        }
    }
    if let Ok(v) = std::env::var("GEEKCLAW_SESSION_TIMEOUT") {
        if let Ok(n) = v.parse() {
            config.agents.defaults.session_timeout = n;
        }
    }
    if let Ok(v) = std::env::var("GEEKCLAW_MAX_CONTEXT_TOKENS") {
        if let Ok(n) = v.parse() {
            config.agents.defaults.max_context_tokens = n;
        }
    }
}

/// 解析 provider 配置中 `$ENV_VAR` 格式的环境变量引用。
fn resolve_env_refs(config: &mut Config) {
    for provider in &mut config.providers {
        if let Some(ref key) = provider.api_key {
            if let Some(env_name) = key.strip_prefix('$') {
                provider.api_key = std::env::var(env_name).ok();
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = load_default();
        assert_eq!(config.agents.defaults.max_tool_iterations, 10);
        assert_eq!(config.agents.defaults.session_timeout, 300);
    }

    #[test]
    fn test_parse_yaml() {
        let yaml = r#"
agents:
  defaults:
    model: claude-3-opus
    max_tool_iterations: 20
providers:
  - name: openai
    api_key: $OPENAI_API_KEY
    model: gpt-4o
"#;
        let config: Config = serde_yaml::from_str(yaml).unwrap();
        assert_eq!(config.agents.defaults.model, "claude-3-opus");
        assert_eq!(config.agents.defaults.max_tool_iterations, 20);
        assert_eq!(config.providers.len(), 1);
        assert_eq!(config.providers[0].name, "openai");
    }
}
