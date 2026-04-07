//! GeekClaw 工具系统。
//!
//! 定义 Tool trait 和 ToolRegistry，提供工具注册、查找和并发执行。

pub mod builtin;
mod registry;

pub use registry::ToolRegistry;

use async_trait::async_trait;
use geekclaw_providers::{Message, ToolDefinition, ToolFunctionDefinition};
use serde_json::Value;
use std::collections::HashMap;

/// 工具执行上下文，包含会话信息和工作目录等。
#[derive(Debug, Clone)]
pub struct ToolContext {
    /// 会话标识符。
    pub session_key: String,
    /// 渠道名称。
    pub channel: String,
    /// 聊天 ID。
    pub chat_id: String,
    /// 当前工作目录。
    pub working_dir: String,
    /// 额外元数据。
    pub metadata: HashMap<String, String>,
}

/// 工具执行结果。
#[derive(Debug)]
pub struct ToolResult {
    /// 工具输出内容。
    pub content: String,
    /// 是否执行成功。
    pub success: bool,
}

impl ToolResult {
    pub fn ok(content: impl Into<String>) -> Self {
        Self {
            content: content.into(),
            success: true,
        }
    }

    pub fn err(content: impl Into<String>) -> Self {
        Self {
            content: content.into(),
            success: false,
        }
    }
}

/// 工具接口。所有工具（内置和外部）必须实现此 trait。
#[async_trait]
pub trait Tool: Send + Sync {
    /// 工具名称（唯一标识符）。
    fn name(&self) -> &str;

    /// 工具描述（供 LLM 理解用途）。
    fn description(&self) -> &str;

    /// 工具参数的 JSON Schema。
    fn parameters(&self) -> Value;

    /// 执行工具。
    async fn execute(&self, args: Value, ctx: &ToolContext) -> ToolResult;

    /// 转换为 LLM API 的工具定义格式。
    fn to_definition(&self) -> ToolDefinition {
        ToolDefinition {
            r#type: "function".into(),
            function: ToolFunctionDefinition {
                name: self.name().into(),
                description: self.description().into(),
                parameters: self.parameters(),
            },
        }
    }
}

/// 工具调用请求（从 LLM 响应中解析）。
#[derive(Debug, Clone)]
pub struct ToolCallRequest {
    /// 工具调用 ID。
    pub id: String,
    /// 工具名称。
    pub name: String,
    /// 参数。
    pub arguments: Value,
}

/// 从 LLM 响应中的 tool_calls 解析出工具调用请求。
pub fn parse_tool_calls(msg: &Message) -> Vec<ToolCallRequest> {
    msg.tool_calls
        .iter()
        .filter_map(|tc| {
            let (name, args) = if let Some(ref func) = tc.function {
                let args: Value = serde_json::from_str(&func.arguments).unwrap_or(Value::Null);
                (func.name.clone(), args)
            } else if !tc.name.is_empty() {
                (
                    tc.name.clone(),
                    Value::Object(
                        tc.arguments
                            .iter()
                            .map(|(k, v)| (k.clone(), v.clone()))
                            .collect(),
                    ),
                )
            } else {
                return None;
            };

            Some(ToolCallRequest {
                id: tc.id.clone(),
                name,
                arguments: args,
            })
        })
        .collect()
}
