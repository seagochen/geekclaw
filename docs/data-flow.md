# 数据流

## Agent 循环（单次消息处理）

```
process_message(opts)
│
├─ 1. 加载会话历史
│     SessionStore::get_history(session_key)
│     → Vec<Message>
│
├─ 2. 构建上下文
│     ContextBuilder::build_messages(history, user_message)
│     → [system_prompt, ...history, user_message]
│     → trim_to_budget() 裁剪超出 token 预算的历史
│
├─ 3. 保存用户消息
│     SessionStore::append(session_key, user_msg)
│
├─ 4. LLM 调用循环 (最多 max_iterations 次)
│     │
│     ├─ FallbackChain::execute(candidates, run)
│     │   ├─ 检查缓存的最近成功候选
│     │   ├─ 遍历候选者：检查冷却期 → 调用 → 分类错误
│     │   └─ 成功/失败/耗尽
│     │
│     ├─ LlmResponse 有 tool_calls?
│     │   ├─ 是 → ToolRegistry::execute_batch(calls, ctx, 10)
│     │   │       → 将工具结果作为 tool 消息加入上下文
│     │   │       → 继续循环
│     │   └─ 否 → final_content = response.content
│     │           → 退出循环
│     │
│     └─ 达到 max_iterations → 警告并退出
│
├─ 5. 保存助手响应
│     SessionStore::append(session_key, assistant_msg)
│
└─ 6. 发送响应
      outbound_tx.send(OutboundMessage)
```

## 故障转移链

```
FallbackChain::execute(candidates, run_fn)
│
├─ 检查缓存 (lastSuccess, TTL 5min)
│   └─ 命中且未冷却 → 直接调用 → 成功则返回
│
├─ 遍历 candidates:
│   ├─ cooldown.is_available(provider)?
│   │   └─ 否 → 记录 skipped, continue
│   │
│   ├─ run_fn(provider, model)
│   │   ├─ Ok(response) → 缓存成功, mark_success, 返回
│   │   └─ Err(err) → classify_error:
│   │       ├─ 不可重试 (Format) → 立即返回错误
│   │       ├─ 可重试 → mark_failure(冷却), continue
│   │       └─ 无法分类 → 立即返回错误
│   │
│   └─ 最后一个候选 → FallbackExhaustedError
│
└─ 全部跳过 → FallbackExhaustedError
```

## 会话持久化

```
JSONL 文件结构:

{session_key}.jsonl:          # 追加式消息日志
  {"role":"user","content":"你好"}
  {"role":"assistant","content":"你好！有什么..."}
  {"role":"user","content":"帮我查天气"}
  ...

{session_key}.meta.json:      # 元数据侧车文件
  {
    "key": "interactive:default",
    "summary": "",
    "skip": 0,                # 逻辑截断偏移量
    "count": 42,              # 总消息数
    "created_at": "...",
    "updated_at": "..."
  }

截断机制:
  truncate(keep=10) → 设置 skip = count - 10
  get_history() → 跳过前 skip 行，返回剩余
  (物理文件不删除，确保追加式写入的崩溃安全性)
```
