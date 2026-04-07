//! 外部插件 LLM Provider。
//!
//! 通过 JSON-RPC over stdio 调用外部插件进程（如 Python 脚本）实现 LLM 调用，
//! 保持与 Go 版本的插件兼容性。

use std::collections::HashMap;

use geekclaw_plugin::{PluginConfig, PluginProcess, SpawnOpts};
use serde_json::Value;

use crate::{
    FunctionCall, LlmProvider, LlmResponse, Message, ProviderError, ToolCall, ToolDefinition,
    UsageInfo,
};

/// 外部插件 LLM Provider。
///
/// 通过 JSON-RPC 2.0 over stdio 与外部进程通信。
pub struct ExternalProvider {
    process: PluginProcess,
    default_model: String,
}

impl ExternalProvider {
    /// 启动外部插件进程并初始化。
    pub async fn start(
        name: &str,
        config: PluginConfig,
        default_model: &str,
    ) -> Result<Self, ProviderError> {
        let process = PluginProcess::new(name, config);

        let opts = SpawnOpts {
            log_category: "provider".into(),
            init_method: "initialize".into(),
            init_params: Some(serde_json::json!({})),
            stop_method: "shutdown".into(),
            log_method: "log".into(),
            on_notification: None,
            services: HashMap::new(),
        };

        process
            .spawn(opts)
            .await
            .map_err(|e| ProviderError::Other(format!("启动插件进程失败: {e}")))?;

        Ok(Self {
            process,
            default_model: default_model.to_string(),
        })
    }

    /// 停止插件进程。
    pub async fn stop(&self) {
        self.process.stop().await;
    }
}

#[async_trait::async_trait]
impl LlmProvider for ExternalProvider {
    async fn chat(
        &self,
        messages: &[Message],
        tools: &[ToolDefinition],
        model: &str,
        options: &HashMap<String, Value>,
    ) -> Result<LlmResponse, ProviderError> {
        let model = if model.is_empty() {
            &self.default_model
        } else {
            model
        };

        let params = serde_json::json!({
            "messages": messages,
            "tools": tools,
            "model": model,
            "options": options,
        });

        let result = self
            .process
            .call("chat", Some(params))
            .await
            .map_err(|e| ProviderError::Other(format!("插件 chat 调用失败: {e}")))?;

        parse_plugin_response(result)
    }

    fn default_model(&self) -> &str {
        &self.default_model
    }
}

/// 解析插件 JSON-RPC 响应为 LlmResponse。
fn parse_plugin_response(value: Value) -> Result<LlmResponse, ProviderError> {
    let content = value["content"].as_str().unwrap_or_default().to_string();
    let finish_reason = value["finish_reason"]
        .as_str()
        .unwrap_or_default()
        .to_string();

    let tool_calls = if let Some(tcs) = value["tool_calls"].as_array() {
        tcs.iter()
            .filter_map(|tc| {
                let id = tc["id"].as_str()?.to_string();
                let func_name = tc["function"]["name"].as_str()?.to_string();
                let func_args = tc["function"]["arguments"]
                    .as_str()
                    .unwrap_or("{}")
                    .to_string();

                Some(ToolCall {
                    id,
                    r#type: Some("function".into()),
                    function: Some(FunctionCall {
                        name: func_name.clone(),
                        arguments: func_args.clone(),
                    }),
                    name: func_name,
                    arguments: serde_json::from_str(&func_args).unwrap_or_default(),
                })
            })
            .collect()
    } else {
        vec![]
    };

    let usage = if value["usage"].is_object() {
        Some(UsageInfo {
            prompt_tokens: value["usage"]["prompt_tokens"].as_u64().unwrap_or(0) as u32,
            completion_tokens: value["usage"]["completion_tokens"].as_u64().unwrap_or(0) as u32,
            total_tokens: value["usage"]["total_tokens"].as_u64().unwrap_or(0) as u32,
        })
    } else {
        None
    };

    Ok(LlmResponse {
        content,
        reasoning_content: value["reasoning_content"]
            .as_str()
            .map(|s| s.to_string()),
        tool_calls,
        finish_reason,
        usage,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_simple_response() {
        let json = serde_json::json!({
            "content": "Hello!",
            "finish_reason": "stop",
            "usage": {
                "prompt_tokens": 10,
                "completion_tokens": 5,
                "total_tokens": 15
            }
        });

        let resp = parse_plugin_response(json).unwrap();
        assert_eq!(resp.content, "Hello!");
        assert_eq!(resp.finish_reason, "stop");
        assert!(resp.tool_calls.is_empty());
        assert_eq!(resp.usage.unwrap().total_tokens, 15);
    }

    #[test]
    fn test_parse_tool_call_response() {
        let json = serde_json::json!({
            "content": "",
            "finish_reason": "tool_calls",
            "tool_calls": [{
                "id": "call_123",
                "type": "function",
                "function": {
                    "name": "read_file",
                    "arguments": "{\"path\": \"/tmp/test.txt\"}"
                }
            }]
        });

        let resp = parse_plugin_response(json).unwrap();
        assert_eq!(resp.tool_calls.len(), 1);
        assert_eq!(resp.tool_calls[0].name, "read_file");
    }
}
