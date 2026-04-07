//! 工具注册表：管理工具的注册、查找和并发执行。

use crate::{Tool, ToolCallRequest, ToolContext, ToolResult};
use geekclaw_providers::Message;
use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use tracing::{info, warn};

/// 工具条目。
struct ToolEntry {
    tool: Arc<dyn Tool>,
    #[allow(dead_code)]
    is_core: bool,
}

/// 工具注册表。
pub struct ToolRegistry {
    tools: RwLock<HashMap<String, ToolEntry>>,
}

impl ToolRegistry {
    /// 创建空的注册表。
    pub fn new() -> Self {
        Self {
            tools: RwLock::new(HashMap::new()),
        }
    }

    /// 注册核心工具。
    pub fn register(&self, tool: Arc<dyn Tool>) {
        let name = tool.name().to_string();
        let mut tools = self.tools.write().unwrap();
        if tools.contains_key(&name) {
            warn!(name = %name, "工具注册覆盖已有工具");
        }
        tools.insert(
            name.clone(),
            ToolEntry {
                tool,
                is_core: true,
            },
        );
        info!(name = %name, "注册核心工具");
    }

    /// 注册外部工具（非核心）。
    pub fn register_external(&self, tool: Arc<dyn Tool>) {
        let name = tool.name().to_string();
        let mut tools = self.tools.write().unwrap();
        tools.insert(
            name,
            ToolEntry {
                tool,
                is_core: false,
            },
        );
    }

    /// 查找工具。
    pub fn get(&self, name: &str) -> Option<Arc<dyn Tool>> {
        let tools = self.tools.read().unwrap();
        tools.get(name).map(|e| Arc::clone(&e.tool))
    }

    /// 获取所有工具定义（用于 LLM API）。
    pub fn definitions(&self) -> Vec<geekclaw_providers::ToolDefinition> {
        let tools = self.tools.read().unwrap();
        let mut defs: Vec<_> = tools.values().map(|e| e.tool.to_definition()).collect();
        defs.sort_by(|a, b| a.function.name.cmp(&b.function.name));
        defs
    }

    /// 获取已注册工具名称列表。
    pub fn names(&self) -> Vec<String> {
        let tools = self.tools.read().unwrap();
        let mut names: Vec<_> = tools.keys().cloned().collect();
        names.sort();
        names
    }

    /// 工具数量。
    pub fn len(&self) -> usize {
        self.tools.read().unwrap().len()
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    /// 并发执行多个工具调用（上限 max_concurrent）。
    pub async fn execute_batch(
        &self,
        calls: Vec<ToolCallRequest>,
        ctx: &ToolContext,
        max_concurrent: usize,
    ) -> Vec<(String, ToolResult)> {
        use tokio::sync::Semaphore;

        let semaphore = Arc::new(Semaphore::new(max_concurrent));
        let mut handles = Vec::with_capacity(calls.len());

        for call in calls {
            let tool = self.get(&call.name);
            let ctx = ctx.clone();
            let sem = Arc::clone(&semaphore);

            handles.push(tokio::spawn(async move {
                let _permit = sem.acquire().await.unwrap();

                let result = match tool {
                    Some(t) => {
                        // panic recovery
                        match tokio::spawn({
                            let t = Arc::clone(&t);
                            let args = call.arguments.clone();
                            let ctx = ctx.clone();
                            async move { t.execute(args, &ctx).await }
                        })
                        .await
                        {
                            Ok(r) => r,
                            Err(e) => ToolResult::err(format!("工具执行 panic: {e}")),
                        }
                    }
                    None => ToolResult::err(format!("未找到工具: {}", call.name)),
                };

                (call.id, result)
            }));
        }

        let mut results = Vec::with_capacity(handles.len());
        for handle in handles {
            match handle.await {
                Ok(r) => results.push(r),
                Err(e) => results.push(("unknown".into(), ToolResult::err(format!("任务失败: {e}")))),
            }
        }
        results
    }

    /// 执行单个工具调用并返回 Message（工具结果消息）。
    pub async fn execute_one(&self, call: &ToolCallRequest, ctx: &ToolContext) -> Message {
        let result = match self.get(&call.name) {
            Some(tool) => tool.execute(call.arguments.clone(), ctx).await,
            None => ToolResult::err(format!("未找到工具: {}", call.name)),
        };

        Message::tool_result(&call.id, result.content)
    }
}

impl Default for ToolRegistry {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use async_trait::async_trait;
    use serde_json::json;

    struct EchoTool;

    #[async_trait]
    impl Tool for EchoTool {
        fn name(&self) -> &str {
            "echo"
        }
        fn description(&self) -> &str {
            "回显输入"
        }
        fn parameters(&self) -> serde_json::Value {
            json!({
                "type": "object",
                "properties": {
                    "text": { "type": "string" }
                },
                "required": ["text"]
            })
        }
        async fn execute(&self, args: serde_json::Value, _ctx: &ToolContext) -> ToolResult {
            let text = args["text"].as_str().unwrap_or("empty");
            ToolResult::ok(text)
        }
    }

    fn test_ctx() -> ToolContext {
        ToolContext {
            session_key: "test".into(),
            channel: "test".into(),
            chat_id: "1".into(),
            working_dir: "/tmp".into(),
            metadata: HashMap::new(),
        }
    }

    #[test]
    fn test_register_and_lookup() {
        let registry = ToolRegistry::new();
        registry.register(Arc::new(EchoTool));

        assert_eq!(registry.len(), 1);
        assert!(registry.get("echo").is_some());
        assert!(registry.get("nonexistent").is_none());
    }

    #[test]
    fn test_definitions() {
        let registry = ToolRegistry::new();
        registry.register(Arc::new(EchoTool));

        let defs = registry.definitions();
        assert_eq!(defs.len(), 1);
        assert_eq!(defs[0].function.name, "echo");
    }

    #[tokio::test]
    async fn test_execute_batch() {
        let registry = ToolRegistry::new();
        registry.register(Arc::new(EchoTool));

        let calls = vec![
            ToolCallRequest {
                id: "call_1".into(),
                name: "echo".into(),
                arguments: json!({"text": "hello"}),
            },
            ToolCallRequest {
                id: "call_2".into(),
                name: "echo".into(),
                arguments: json!({"text": "world"}),
            },
        ];

        let results = registry.execute_batch(calls, &test_ctx(), 10).await;
        assert_eq!(results.len(), 2);
        assert!(results.iter().all(|(_, r)| r.success));
    }

    #[tokio::test]
    async fn test_execute_unknown_tool() {
        let registry = ToolRegistry::new();
        let call = ToolCallRequest {
            id: "call_1".into(),
            name: "nonexistent".into(),
            arguments: json!({}),
        };

        let results = registry.execute_batch(vec![call], &test_ctx(), 10).await;
        assert!(!results[0].1.success);
    }
}
