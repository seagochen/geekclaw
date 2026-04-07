//! 集成测试：验证 bus → agent → providers → tools 完整流程。
//!
//! 使用 MockProvider 模拟 LLM 响应，不需要真实 API key。

use std::collections::HashMap;
use std::sync::Arc;

use async_trait::async_trait;
use geekclaw_agent::{AgentInstance, AgentLoop, ProcessOptions};
use geekclaw_bus::MessageBus;
use geekclaw_memory::JSONLStore;
use geekclaw_providers::{
    LlmProvider, LlmResponse, Message, ProviderError, ToolCall, ToolDefinition, FunctionCall,
    UsageInfo,
};
use geekclaw_tools::{Tool, ToolContext, ToolRegistry, ToolResult};
use serde_json::{json, Value};
use tokio::sync::Mutex;

// ─── Mock Provider ──────────────────────────────────────────────────────────

/// 可编程的 Mock LLM Provider，用于集成测试。
struct MockProvider {
    responses: Mutex<Vec<LlmResponse>>,
}

impl MockProvider {
    fn new(responses: Vec<LlmResponse>) -> Self {
        Self {
            responses: Mutex::new(responses),
        }
    }

    /// 创建一个返回简单文本的 mock。
    fn with_text(text: &str) -> Self {
        Self::new(vec![LlmResponse {
            content: text.into(),
            reasoning_content: None,
            tool_calls: vec![],
            finish_reason: "stop".into(),
            usage: Some(UsageInfo {
                prompt_tokens: 10,
                completion_tokens: 5,
                total_tokens: 15,
            }),
        }])
    }

    /// 创建一个先调用工具、再返回文本的 mock（两轮对话）。
    fn with_tool_call(tool_name: &str, args: &str, final_text: &str) -> Self {
        Self::new(vec![
            // 第一轮：LLM 请求调用工具
            LlmResponse {
                content: String::new(),
                reasoning_content: None,
                tool_calls: vec![ToolCall {
                    id: "call_test_001".into(),
                    r#type: Some("function".into()),
                    function: Some(FunctionCall {
                        name: tool_name.into(),
                        arguments: args.into(),
                    }),
                    name: tool_name.into(),
                    arguments: serde_json::from_str(args).unwrap_or_default(),
                }],
                finish_reason: "tool_calls".into(),
                usage: Some(UsageInfo {
                    prompt_tokens: 20,
                    completion_tokens: 10,
                    total_tokens: 30,
                }),
            },
            // 第二轮：工具结果后，LLM 返回最终文本
            LlmResponse {
                content: final_text.into(),
                reasoning_content: None,
                tool_calls: vec![],
                finish_reason: "stop".into(),
                usage: Some(UsageInfo {
                    prompt_tokens: 30,
                    completion_tokens: 15,
                    total_tokens: 45,
                }),
            },
        ])
    }
}

#[async_trait]
impl LlmProvider for MockProvider {
    async fn chat(
        &self,
        _messages: &[Message],
        _tools: &[ToolDefinition],
        _model: &str,
        _options: &HashMap<String, Value>,
    ) -> Result<LlmResponse, ProviderError> {
        let mut responses = self.responses.lock().await;
        if responses.is_empty() {
            Ok(LlmResponse {
                content: "(no more mock responses)".into(),
                reasoning_content: None,
                tool_calls: vec![],
                finish_reason: "stop".into(),
                usage: None,
            })
        } else {
            Ok(responses.remove(0))
        }
    }

    fn default_model(&self) -> &str {
        "mock-model"
    }
}

// ─── Mock Tool ──────────────────────────────────────────────────────────────

struct EchoTool;

#[async_trait]
impl Tool for EchoTool {
    fn name(&self) -> &str {
        "echo"
    }
    fn description(&self) -> &str {
        "回显输入文本"
    }
    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "text": { "type": "string" }
            },
            "required": ["text"]
        })
    }
    async fn execute(&self, args: Value, _ctx: &ToolContext) -> ToolResult {
        let text = args["text"].as_str().unwrap_or("(empty)");
        ToolResult::ok(format!("echo: {text}"))
    }
}

// ─── Helper ─────────────────────────────────────────────────────────────────

async fn setup(
    provider: Arc<dyn LlmProvider>,
    tools: Vec<Arc<dyn Tool>>,
) -> (AgentLoop, Arc<dyn geekclaw_memory::SessionStore>) {
    let tmp = tempfile::tempdir().unwrap();
    let data_dir = tmp.path().join("data");
    let meta_dir = tmp.path().join("meta");

    let memory: Arc<dyn geekclaw_memory::SessionStore> =
        Arc::new(JSONLStore::new(&data_dir, &meta_dir).await.unwrap());

    let bus = MessageBus::new();
    let outbound_tx = bus.outbound_sender();

    let registry = Arc::new(ToolRegistry::new());
    for tool in tools {
        registry.register(tool);
    }

    let agent_instance = AgentInstance {
        id: "test".into(),
        model: "mock-model".into(),
        fallback_models: vec![],
        default_provider: "mock".into(),
        max_iterations: 5,
        session_timeout: 60,
        max_tokens: None,
        temperature: None,
        system_prompt: "You are a helpful assistant.".into(),
        max_context_tokens: 128_000,
    };

    let agent = AgentLoop::new(agent_instance, outbound_tx, memory.clone(), provider, registry);

    // 让 tempdir 不被 drop — leak 是测试中可接受的
    std::mem::forget(tmp);

    (agent, memory)
}

// ─── Tests ──────────────────────────────────────────────────────────────────

#[tokio::test]
async fn test_simple_text_response() {
    let provider = Arc::new(MockProvider::with_text("Hello from mock!"));
    let (agent, _memory) = setup(provider, vec![]).await;

    let opts = ProcessOptions {
        session_key: "test:simple".into(),
        channel: "test".into(),
        chat_id: "1".into(),
        user_message: "Say hello".into(),
        send_response: false,
        ..Default::default()
    };

    let result = agent.process_message(opts).await.unwrap();
    assert_eq!(result, "Hello from mock!");
}

#[tokio::test]
async fn test_tool_call_flow() {
    let provider = Arc::new(MockProvider::with_tool_call(
        "echo",
        r#"{"text": "hello world"}"#,
        "The echo tool returned: hello world",
    ));
    let tools: Vec<Arc<dyn Tool>> = vec![Arc::new(EchoTool)];
    let (agent, _memory) = setup(provider, tools).await;

    let opts = ProcessOptions {
        session_key: "test:tool".into(),
        channel: "test".into(),
        chat_id: "1".into(),
        user_message: "Echo hello world".into(),
        send_response: false,
        ..Default::default()
    };

    let result = agent.process_message(opts).await.unwrap();
    assert_eq!(result, "The echo tool returned: hello world");
}

#[tokio::test]
async fn test_session_persistence() {
    let provider = Arc::new(MockProvider::new(vec![
        LlmResponse {
            content: "First response".into(),
            reasoning_content: None,
            tool_calls: vec![],
            finish_reason: "stop".into(),
            usage: None,
        },
        LlmResponse {
            content: "Second response".into(),
            reasoning_content: None,
            tool_calls: vec![],
            finish_reason: "stop".into(),
            usage: None,
        },
    ]));
    let (agent, memory) = setup(provider, vec![]).await;

    // 第一轮对话。
    let opts1 = ProcessOptions {
        session_key: "test:persist".into(),
        channel: "test".into(),
        chat_id: "1".into(),
        user_message: "First message".into(),
        send_response: false,
        ..Default::default()
    };
    agent.process_message(opts1).await.unwrap();

    // 第二轮对话。
    let opts2 = ProcessOptions {
        session_key: "test:persist".into(),
        channel: "test".into(),
        chat_id: "1".into(),
        user_message: "Second message".into(),
        send_response: false,
        ..Default::default()
    };
    agent.process_message(opts2).await.unwrap();

    // 验证会话历史持久化了 4 条消息（2 user + 2 assistant）。
    let history = memory.get_history("test:persist", 0).await.unwrap();
    assert_eq!(history.len(), 4);
    assert_eq!(history[0].role, "user");
    assert_eq!(history[0].content, "First message");
    assert_eq!(history[1].role, "assistant");
    assert_eq!(history[1].content, "First response");
    assert_eq!(history[2].role, "user");
    assert_eq!(history[2].content, "Second message");
    assert_eq!(history[3].role, "assistant");
    assert_eq!(history[3].content, "Second response");
}

#[tokio::test]
async fn test_no_history_mode() {
    let provider = Arc::new(MockProvider::with_text("Stateless response"));
    let (agent, memory) = setup(provider, vec![]).await;

    // 先写入一些历史。
    memory
        .append("test:nohistory", &Message::user("old message"))
        .await
        .unwrap();

    let opts = ProcessOptions {
        session_key: "test:nohistory".into(),
        channel: "test".into(),
        chat_id: "1".into(),
        user_message: "Ping".into(),
        send_response: false,
        no_history: true,
        ..Default::default()
    };

    let result = agent.process_message(opts).await.unwrap();
    assert_eq!(result, "Stateless response");
}

#[tokio::test]
async fn test_empty_message_no_crash() {
    let provider = Arc::new(MockProvider::with_text("Response to empty"));
    let (agent, _) = setup(provider, vec![]).await;

    let opts = ProcessOptions {
        session_key: "test:empty".into(),
        channel: "test".into(),
        chat_id: "1".into(),
        user_message: String::new(),
        send_response: false,
        ..Default::default()
    };

    let result = agent.process_message(opts).await.unwrap();
    assert_eq!(result, "Response to empty");
}

#[tokio::test]
async fn test_unknown_tool_returns_error() {
    // LLM 请求调用一个不存在的工具。
    let provider = Arc::new(MockProvider::with_tool_call(
        "nonexistent_tool",
        r#"{}"#,
        "Tool failed but I recovered",
    ));
    let (agent, _) = setup(provider, vec![]).await;

    let opts = ProcessOptions {
        session_key: "test:unknown_tool".into(),
        channel: "test".into(),
        chat_id: "1".into(),
        user_message: "Call something".into(),
        send_response: false,
        ..Default::default()
    };

    // 应该不 panic，agent 能优雅处理未知工具。
    let result = agent.process_message(opts).await.unwrap();
    assert_eq!(result, "Tool failed but I recovered");
}
