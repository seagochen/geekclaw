//! Agent 主循环：消息消费 → 上下文构建 → LLM 调用 → 工具执行 → 响应发送。

use crate::{AgentInstance, ContextBuilder, ProcessOptions};
use geekclaw_bus::{InboundMessage, OutboundMessage};
use geekclaw_memory::SessionStore;
use geekclaw_providers::{
    FallbackChain, CooldownTracker, LlmProvider, LlmResponse, Message,
};
use geekclaw_tools::{ToolCallRequest, ToolContext, ToolRegistry};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tracing::{debug, error, info, warn};

/// Agent 主循环。
pub struct AgentLoop {
    /// Agent 实例配置。
    agent: AgentInstance,
    /// 消息总线出站发送端。
    outbound_tx: mpsc::Sender<OutboundMessage>,
    /// 会话存储。
    memory: Arc<dyn SessionStore>,
    /// LLM Provider。
    provider: Arc<dyn LlmProvider>,
    /// 故障转移链。
    fallback: FallbackChain,
    /// 工具注册表。
    tools: Arc<ToolRegistry>,
    /// 取消令牌。
    cancel: CancellationToken,
}

impl AgentLoop {
    /// 创建新的 Agent 循环。
    pub fn new(
        agent: AgentInstance,
        outbound_tx: mpsc::Sender<OutboundMessage>,
        memory: Arc<dyn SessionStore>,
        provider: Arc<dyn LlmProvider>,
        tools: Arc<ToolRegistry>,
    ) -> Self {
        let cooldown = CooldownTracker::new();
        let fallback = FallbackChain::new(cooldown);
        Self {
            agent,
            outbound_tx,
            memory,
            provider,
            fallback,
            tools,
            cancel: CancellationToken::new(),
        }
    }

    /// 运行主循环：持续消费入站消息并处理。
    pub async fn run(&self, mut inbound_rx: mpsc::Receiver<InboundMessage>) {
        info!(agent_id = %self.agent.id, "Agent 循环启动");

        loop {
            tokio::select! {
                Some(msg) = inbound_rx.recv() => {
                    let opts = ProcessOptions {
                        session_key: msg.session_key.clone(),
                        channel: msg.channel.clone(),
                        chat_id: msg.chat_id.clone(),
                        user_message: msg.content.clone(),
                        send_response: true,
                        ..Default::default()
                    };

                    if let Err(e) = self.process_message(opts).await {
                        error!(error = %e, "处理消息失败");
                    }
                }
                _ = self.cancel.cancelled() => {
                    info!(agent_id = %self.agent.id, "Agent 循环收到取消信号");
                    break;
                }
            }
        }

        info!(agent_id = %self.agent.id, "Agent 循环已停止");
    }

    /// 处理单条消息：加载历史 → 构建上下文 → LLM 调用循环 → 发送响应。
    pub async fn process_message(&self, opts: ProcessOptions) -> anyhow::Result<String> {
        debug!(
            session = %opts.session_key,
            channel = %opts.channel,
            "开始处理消息"
        );

        // 1. 加载会话历史。
        let history = if opts.no_history {
            Vec::new()
        } else {
            self.memory
                .get_history(&opts.session_key, 0)
                .await
                .unwrap_or_default()
        };

        // 2. 构建上下文。
        let ctx_builder = ContextBuilder::new(
            self.agent.system_prompt.clone(),
            self.agent.max_context_tokens,
        );
        let mut messages = ctx_builder.build_messages(history, &opts.user_message);

        // Token 预算裁剪。
        ctx_builder.trim_to_budget(&mut messages);

        // 3. 保存用户消息到历史。
        if !opts.user_message.is_empty() {
            let user_msg = Message::user(&opts.user_message);
            let _ = self.memory.append(&opts.session_key, &user_msg).await;
        }

        // 4. LLM 调用循环（带工具执行）。
        let final_content = self.run_llm_loop(&mut messages, &opts).await?;

        // 5. 保存助手响应到历史。
        if !final_content.is_empty() {
            let assistant_msg = Message::assistant(&final_content);
            let _ = self.memory.append(&opts.session_key, &assistant_msg).await;
        }

        // 6. 发送响应。
        if opts.send_response && !final_content.is_empty() {
            let _ = self
                .outbound_tx
                .send(OutboundMessage {
                    channel: opts.channel.clone(),
                    chat_id: opts.chat_id.clone(),
                    content: final_content.clone(),
                    ..Default::default()
                })
                .await;
        }

        Ok(final_content)
    }

    /// LLM 调用循环：反复调用 LLM，处理工具调用，直到无更多工具调用或达到迭代上限。
    async fn run_llm_loop(
        &self,
        messages: &mut Vec<Message>,
        opts: &ProcessOptions,
    ) -> anyhow::Result<String> {
        let candidates = self.agent.candidates();
        let mut final_content = String::new();

        for iteration in 0..self.agent.max_iterations {
            debug!(iteration, "LLM 迭代");

            // 调用 LLM。
            let response = if candidates.len() > 1 {
                self.call_with_fallback(messages, &candidates).await?
            } else {
                self.call_direct(messages).await?
            };

            // 记录 token 使用。
            if let Some(ref usage) = response.usage {
                debug!(
                    prompt_tokens = usage.prompt_tokens,
                    completion_tokens = usage.completion_tokens,
                    total_tokens = usage.total_tokens,
                    "Token 使用"
                );
            }

            // 检查是否有工具调用。
            if response.tool_calls.is_empty() {
                final_content = response.content;
                break;
            }

            // 将助手消息（含工具调用）加入上下文。
            messages.push(Message {
                role: "assistant".into(),
                content: response.content.clone(),
                tool_calls: response.tool_calls.clone(),
                ..Default::default()
            });

            // 解析并执行工具调用。
            let tool_calls: Vec<ToolCallRequest> = response
                .tool_calls
                .iter()
                .filter_map(|tc| {
                    let (name, args) = if let Some(ref func) = tc.function {
                        let args = serde_json::from_str(&func.arguments)
                            .unwrap_or(serde_json::Value::Null);
                        (func.name.clone(), args)
                    } else {
                        return None;
                    };
                    Some(ToolCallRequest {
                        id: tc.id.clone(),
                        name,
                        arguments: args,
                    })
                })
                .collect();

            let tool_ctx = ToolContext {
                session_key: opts.session_key.clone(),
                channel: opts.channel.clone(),
                chat_id: opts.chat_id.clone(),
                working_dir: opts
                    .working_dir
                    .clone()
                    .unwrap_or_else(|| ".".into()),
                metadata: HashMap::new(),
            };

            info!(count = tool_calls.len(), "执行工具调用");

            // 并发执行（上限 10）。
            let results = self
                .tools
                .execute_batch(tool_calls, &tool_ctx, 10)
                .await;

            // 将工具结果加入上下文。
            for (call_id, result) in results {
                let tool_msg = Message::tool_result(&call_id, &result.content);
                messages.push(tool_msg);
            }

            // 如果是最后一次迭代，记录警告。
            if iteration == self.agent.max_iterations - 1 {
                warn!("达到最大工具迭代次数 {}", self.agent.max_iterations);
                final_content =
                    "I've reached the maximum number of tool iterations. Please try again with a simpler request.".into();
            }
        }

        Ok(final_content)
    }

    /// 直接调用 LLM（不使用故障转移链）。
    async fn call_direct(&self, messages: &[Message]) -> anyhow::Result<LlmResponse> {
        let tool_defs = self.tools.definitions();
        let opts = HashMap::new();
        let response = self
            .provider
            .chat(messages, &tool_defs, &self.agent.model, &opts)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))?;
        Ok(response)
    }

    /// 通过故障转移链调用 LLM。
    async fn call_with_fallback(
        &self,
        messages: &[Message],
        candidates: &[geekclaw_providers::FallbackCandidate],
    ) -> anyhow::Result<LlmResponse> {
        let tool_defs = self.tools.definitions();
        let opts = HashMap::new();
        let provider = Arc::clone(&self.provider);
        let messages = messages.to_vec();
        let tool_defs_clone = tool_defs.clone();
        let opts_clone = opts.clone();

        let result = self
            .fallback
            .execute(candidates, |_provider_name, model| {
                let provider = Arc::clone(&provider);
                let messages = messages.clone();
                let tool_defs = tool_defs_clone.clone();
                let opts = opts_clone.clone();
                let model = model.to_string();
                async move {
                    provider
                        .chat(&messages, &tool_defs, &model, &opts)
                        .await
                        .map_err(|e| -> Box<dyn std::error::Error + Send + Sync> {
                            Box::new(e)
                        })
                }
            })
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))?;

        Ok(result.response)
    }

    /// 停止 Agent 循环。
    pub fn stop(&self) {
        self.cancel.cancel();
    }
}
