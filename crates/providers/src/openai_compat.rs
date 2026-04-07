//! OpenAI 兼容 HTTP Provider。
//!
//! 支持所有兼容 OpenAI Chat Completions API 的端点（OpenAI、DeepSeek、Groq、Ollama 等）。

use std::collections::HashMap;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tracing::debug;

use crate::{
    FunctionCall, LlmProvider, LlmResponse, Message, ProviderError, ToolCall, ToolDefinition,
    UsageInfo,
};

/// OpenAI 兼容 Provider。
pub struct OpenAICompatProvider {
    client: reqwest::Client,
    base_url: String,
    api_key: String,
    default_model: String,
}

impl OpenAICompatProvider {
    /// 创建新的 OpenAI 兼容 Provider。
    ///
    /// - `base_url`: API 基础 URL，如 `https://api.openai.com/v1`
    /// - `api_key`: API 密钥
    /// - `default_model`: 默认模型标识，如 `gpt-4o`
    pub fn new(base_url: &str, api_key: &str, default_model: &str) -> Self {
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(120))
            .build()
            .expect("创建 HTTP 客户端失败");

        Self {
            client,
            base_url: base_url.trim_end_matches('/').to_string(),
            api_key: api_key.to_string(),
            default_model: default_model.to_string(),
        }
    }
}

#[async_trait::async_trait]
impl LlmProvider for OpenAICompatProvider {
    async fn chat(
        &self,
        messages: &[Message],
        tools: &[ToolDefinition],
        model: &str,
        options: &HashMap<String, serde_json::Value>,
    ) -> Result<LlmResponse, ProviderError> {
        let model = if model.is_empty() {
            &self.default_model
        } else {
            model
        };

        // 构建请求体。
        let req_messages: Vec<ApiMessage> = messages.iter().map(ApiMessage::from_message).collect();

        let mut body = serde_json::json!({
            "model": model,
            "messages": req_messages,
        });

        // 工具定义。
        if !tools.is_empty() {
            body["tools"] = serde_json::to_value(tools).unwrap_or_default();
        }

        // 可选参数。
        if let Some(max_tokens) = options.get("max_tokens") {
            body["max_tokens"] = max_tokens.clone();
        }
        if let Some(temperature) = options.get("temperature") {
            body["temperature"] = temperature.clone();
        }

        let url = format!("{}/chat/completions", self.base_url);
        debug!(url = %url, model = %model, messages = messages.len(), "发送 LLM 请求");

        let response = self
            .client
            .post(&url)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await?;

        let status = response.status().as_u16();
        if status >= 400 {
            let body_text = response.text().await.unwrap_or_default();
            return Err(ProviderError::Api {
                status,
                message: body_text,
            });
        }

        let api_resp: ApiResponse = response
            .json()
            .await
            .map_err(|e| ProviderError::Other(format!("解析响应失败: {e}")))?;

        // 转换为 LlmResponse。
        let choice = api_resp
            .choices
            .into_iter()
            .next()
            .ok_or_else(|| ProviderError::Other("响应中无 choices".into()))?;

        let tool_calls = choice
            .message
            .tool_calls
            .unwrap_or_default()
            .into_iter()
            .map(|tc| ToolCall {
                id: tc.id,
                r#type: Some(tc.r#type),
                function: Some(FunctionCall {
                    name: tc.function.name.clone(),
                    arguments: tc.function.arguments.clone(),
                }),
                name: tc.function.name,
                arguments: serde_json::from_str(&tc.function.arguments).unwrap_or_default(),
            })
            .collect();

        let usage = api_resp.usage.map(|u| UsageInfo {
            prompt_tokens: u.prompt_tokens,
            completion_tokens: u.completion_tokens,
            total_tokens: u.total_tokens,
        });

        Ok(LlmResponse {
            content: choice.message.content.unwrap_or_default(),
            reasoning_content: None,
            tool_calls,
            finish_reason: choice.finish_reason.unwrap_or_default(),
            usage,
        })
    }

    fn default_model(&self) -> &str {
        &self.default_model
    }
}

// --- OpenAI API 请求/响应类型 ---

#[derive(Serialize)]
struct ApiMessage {
    role: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    content: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    tool_calls: Option<Vec<ApiToolCall>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    tool_call_id: Option<String>,
}

impl ApiMessage {
    fn from_message(msg: &Message) -> Self {
        let tool_calls = if msg.tool_calls.is_empty() {
            None
        } else {
            Some(
                msg.tool_calls
                    .iter()
                    .map(|tc| ApiToolCall {
                        id: tc.id.clone(),
                        r#type: "function".into(),
                        function: ApiFunction {
                            name: tc
                                .function
                                .as_ref()
                                .map(|f| f.name.clone())
                                .unwrap_or_default(),
                            arguments: tc
                                .function
                                .as_ref()
                                .map(|f| f.arguments.clone())
                                .unwrap_or_default(),
                        },
                    })
                    .collect(),
            )
        };

        let content = if msg.content.is_empty() && msg.role == "assistant" && tool_calls.is_some() {
            None
        } else {
            Some(msg.content.clone())
        };

        Self {
            role: msg.role.clone(),
            content,
            tool_calls,
            tool_call_id: msg.tool_call_id.clone(),
        }
    }
}

#[derive(Serialize, Deserialize)]
struct ApiToolCall {
    id: String,
    r#type: String,
    function: ApiFunction,
}

#[derive(Serialize, Deserialize)]
struct ApiFunction {
    name: String,
    arguments: String,
}

#[derive(Deserialize)]
struct ApiResponse {
    choices: Vec<ApiChoice>,
    usage: Option<ApiUsage>,
}

#[derive(Deserialize)]
struct ApiChoice {
    message: ApiChoiceMessage,
    finish_reason: Option<String>,
}

#[derive(Deserialize)]
struct ApiChoiceMessage {
    content: Option<String>,
    tool_calls: Option<Vec<ApiToolCall>>,
}

#[derive(Deserialize)]
struct ApiUsage {
    prompt_tokens: u32,
    completion_tokens: u32,
    total_tokens: u32,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_api_message_from_user_message() {
        let msg = Message::user("hello");
        let api_msg = ApiMessage::from_message(&msg);
        assert_eq!(api_msg.role, "user");
        assert_eq!(api_msg.content.unwrap(), "hello");
        assert!(api_msg.tool_calls.is_none());
    }

    #[test]
    fn test_parse_api_response() {
        let json = r#"{
            "choices": [{
                "message": {
                    "content": "Hello! How can I help?",
                    "role": "assistant"
                },
                "finish_reason": "stop"
            }],
            "usage": {
                "prompt_tokens": 10,
                "completion_tokens": 8,
                "total_tokens": 18
            }
        }"#;

        let resp: ApiResponse = serde_json::from_str(json).unwrap();
        assert_eq!(resp.choices.len(), 1);
        assert_eq!(
            resp.choices[0].message.content.as_deref().unwrap(),
            "Hello! How can I help?"
        );
        assert_eq!(resp.usage.as_ref().unwrap().total_tokens, 18);
    }

    #[test]
    fn test_parse_tool_call_response() {
        let json = r#"{
            "choices": [{
                "message": {
                    "content": null,
                    "tool_calls": [{
                        "id": "call_abc123",
                        "type": "function",
                        "function": {
                            "name": "get_weather",
                            "arguments": "{\"city\": \"Tokyo\"}"
                        }
                    }]
                },
                "finish_reason": "tool_calls"
            }],
            "usage": {
                "prompt_tokens": 20,
                "completion_tokens": 15,
                "total_tokens": 35
            }
        }"#;

        let resp: ApiResponse = serde_json::from_str(json).unwrap();
        let tc = resp.choices[0].message.tool_calls.as_ref().unwrap();
        assert_eq!(tc.len(), 1);
        assert_eq!(tc[0].function.name, "get_weather");
    }
}
