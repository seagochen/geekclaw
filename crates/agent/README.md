# geekclaw-agent — Agent 核心循环

## 模块概述

整个系统的大脑。编排消息处理的完整流程：接收消息 → 加载历史 → 构建上下文 → LLM 调用（含工具执行循环）→ 保存历史 → 发送响应。是唯一依赖几乎所有其他 crate 的顶层模块。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/lib.rs` | `AgentInstance`（agent 配置：模型、迭代上限、token 预算等）、`ProcessOptions`（单次消息处理选项）、`from_config()` 从 Config 创建实例 |
| `src/loop_core.rs` | `AgentLoop`：主循环实现。`run()` 消费入站消息，`process_message()` 处理单条消息，`run_llm_loop()` LLM 迭代（含工具调用），`call_direct()` / `call_with_fallback()` 两种 LLM 调用路径 |
| `src/context.rs` | `ContextBuilder`：组装系统提示词 + 历史 + 用户消息；`estimate_tokens()` 粗略 token 估算；`trim_to_budget()` 裁剪超出预算的历史。含 3 个测试 |
| `tests/integration.rs` | 集成测试：MockProvider + 6 个端到端场景（简单文本、工具调用、会话持久化、无历史模式、空消息、未知工具） |

## 核心流程

### process_message 管线

```
process_message(opts):
    ①  history = memory.get_history(session_key)    // 加载会话历史
    ②  messages = ContextBuilder.build_messages(     // 组装上下文
            history, user_message)
    ③  ContextBuilder.trim_to_budget(&mut messages)  // token 预算裁剪
    ④  memory.append(session_key, user_msg)          // 保存用户消息
    ⑤  content = run_llm_loop(&mut messages, opts)   // LLM + 工具循环
    ⑥  memory.append(session_key, assistant_msg)     // 保存助手响应
    ⑦  outbound_tx.send(OutboundMessage)             // 发送响应
```

### LLM 迭代循环

```
run_llm_loop(messages, opts):
    for iteration in 0..max_iterations:
        response = call LLM (direct 或 fallback)

        if no tool_calls:
            return response.content    // 完成

        // 有工具调用
        messages.push(assistant msg with tool_calls)
        results = tools.execute_batch(tool_calls, ctx, max=10)
        for (id, result) in results:
            messages.push(tool_result msg)

        // 继续循环，让 LLM 处理工具结果

    return "达到最大迭代次数"
```

## 算法说明

### Token 预算裁剪

```rust
fn estimate_tokens(messages) -> usize:
    sum of (msg.content.len() / 4 + 1) for each message

fn trim_to_budget(messages):
    while estimate_tokens(messages) > max_context_tokens
          && messages.len() > 2:
        messages.remove(1)   // 删除最早的历史（保留第一条系统提示词和最后一条用户消息）
```

**注意**：这是一个非常粗略的估算（4 字符 ≈ 1 token），实际的 tokenizer 对中文和特殊字符的处理完全不同。

### 故障转移路径选择

```
if candidates.len() > 1:
    call_with_fallback()  → FallbackChain.execute()
else:
    call_direct()         → provider.chat() 直接调用
```

## 设计决策

- `AgentLoop` 持有 `CancellationToken`，通过 `tokio::select!` 实现优雅关闭
- `process_message()` 是公开的，既用于主循环也用于外部直接调用（如测试、cron 触发）
- 工具执行的 `ToolContext` 从 `ProcessOptions` 构建，包含 session/channel/chatID 信息
- 出站消息通过 `mpsc::Sender` 发送，`AgentLoop` 不直接依赖 `MessageBus` 实例

## 不完善之处

- **Token 估算极其粗糙**：`len()/4` 对英文还行，中文严重低估（一个中文字符 3 字节 / 4 ≈ 0.75 token，实际约 1-2 token）。应集成 tiktoken 或类似库
- **无上下文压缩**：Go 版本在超出 token 限制时调用 LLM 生成摘要，Rust 版只是删除最早的消息
- **无会话超时**：`AgentInstance.session_timeout` 字段存在但未使用
- **无消息并行处理**：Go 版本按 session key 分发到独立 worker（不同会话并行，同一会话串行），Rust 版是完全串行的单循环
- **无重试机制**：LLM 调用失败直接返回错误，Go 版本对 timeout 有 2 次重试
- **无推理内容转发**：Go 版本将 `reasoning_content` 发送到专门的推理频道，Rust 版忽略
- **无斜杠命令**：Go 版本在消息进入 LLM 前先检查 `/help`、`/clear` 等命令，Rust 版所有消息都发给 LLM
- **无多 Agent 支持**：Go 版本有 `AgentRegistry` 管理多个 agent 实例和路由，Rust 版只有单 agent
