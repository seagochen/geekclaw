//! LLM Provider 核心类型定义。

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// LLM Provider 通用接口。
#[async_trait::async_trait]
pub trait LlmProvider: Send + Sync {
    /// 发送对话请求。
    async fn chat(
        &self,
        messages: &[Message],
        tools: &[ToolDefinition],
        model: &str,
        options: &HashMap<String, serde_json::Value>,
    ) -> Result<LlmResponse, ProviderError>;

    /// 返回默认模型标识。
    fn default_model(&self) -> &str;
}

/// Provider 错误类型。
#[derive(Debug, thiserror::Error)]
pub enum ProviderError {
    #[error("HTTP 请求失败: {0}")]
    Http(#[from] reqwest::Error),

    #[error("API 错误 (status={status}): {message}")]
    Api { status: u16, message: String },

    #[error("{0}")]
    Other(String),
}

/// 对话中的一条消息。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub role: String,
    #[serde(default)]
    pub content: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub media: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reasoning_content: Option<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub system_parts: Vec<ContentBlock>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tool_calls: Vec<ToolCall>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_call_id: Option<String>,
}

impl Message {
    /// 创建用户消息。
    pub fn user(content: impl Into<String>) -> Self {
        Self {
            role: "user".into(),
            content: content.into(),
            ..Default::default()
        }
    }

    /// 创建助手消息。
    pub fn assistant(content: impl Into<String>) -> Self {
        Self {
            role: "assistant".into(),
            content: content.into(),
            ..Default::default()
        }
    }

    /// 创建系统消息。
    pub fn system(content: impl Into<String>) -> Self {
        Self {
            role: "system".into(),
            content: content.into(),
            ..Default::default()
        }
    }

    /// 创建工具结果消息。
    pub fn tool_result(tool_call_id: impl Into<String>, content: impl Into<String>) -> Self {
        Self {
            role: "tool".into(),
            content: content.into(),
            tool_call_id: Some(tool_call_id.into()),
            ..Default::default()
        }
    }
}

impl Default for Message {
    fn default() -> Self {
        Self {
            role: String::new(),
            content: String::new(),
            media: Vec::new(),
            reasoning_content: None,
            system_parts: Vec::new(),
            tool_calls: Vec::new(),
            tool_call_id: None,
        }
    }
}

/// 工具调用。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub function: Option<FunctionCall>,
    /// 解析后的工具名称（内部使用）。
    #[serde(skip)]
    pub name: String,
    /// 解析后的参数（内部使用）。
    #[serde(skip)]
    pub arguments: HashMap<String, serde_json::Value>,
}

/// 函数调用详情。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionCall {
    pub name: String,
    pub arguments: String,
}

/// LLM 响应。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LlmResponse {
    #[serde(default)]
    pub content: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reasoning_content: Option<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tool_calls: Vec<ToolCall>,
    #[serde(default)]
    pub finish_reason: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub usage: Option<UsageInfo>,
}

/// Token 使用统计。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct UsageInfo {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

/// 系统消息的结构化内容块。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContentBlock {
    pub r#type: String,
    pub text: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<CacheControl>,
}

/// 缓存控制标记。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CacheControl {
    pub r#type: String,
}

/// 工具定义（用于 LLM API）。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolDefinition {
    pub r#type: String,
    pub function: ToolFunctionDefinition,
}

/// 工具函数定义。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolFunctionDefinition {
    pub name: String,
    pub description: String,
    pub parameters: serde_json::Value,
}

/// 故障转移原因分类。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum FailoverReason {
    Auth,
    RateLimit,
    Billing,
    Timeout,
    Format,
    Overloaded,
    Unknown,
}

impl std::fmt::Display for FailoverReason {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Auth => write!(f, "auth"),
            Self::RateLimit => write!(f, "rate_limit"),
            Self::Billing => write!(f, "billing"),
            Self::Timeout => write!(f, "timeout"),
            Self::Format => write!(f, "format"),
            Self::Overloaded => write!(f, "overloaded"),
            Self::Unknown => write!(f, "unknown"),
        }
    }
}

/// 故障转移错误，封装原始错误并附加分类元数据。
#[derive(Debug, thiserror::Error)]
#[error("failover({reason}): provider={provider} model={model} status={status}: {source}")]
pub struct FailoverError {
    pub reason: FailoverReason,
    pub provider: String,
    pub model: String,
    pub status: u16,
    #[source]
    pub source: Box<dyn std::error::Error + Send + Sync>,
}

impl FailoverError {
    /// 是否应触发故障转移到下一个候选者。
    /// 不可重试的：格式错误（请求结构错误）。
    pub fn is_retriable(&self) -> bool {
        self.reason != FailoverReason::Format
    }
}

/// 模型配置：主模型 + 备选模型列表。
#[derive(Debug, Clone)]
pub struct ModelConfig {
    pub primary: String,
    pub fallbacks: Vec<String>,
}
