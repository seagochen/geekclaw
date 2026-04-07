# geekclaw-providers — LLM Provider 抽象层

## 模块概述

最大的 crate（1727 行）。提供统一的 LLM 调用接口、两个内置 Provider 实现、故障转移链、冷却追踪器和错误分类器。这是 Agent 与外部 LLM 服务之间的抽象层。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/types.rs` | 核心类型：`LlmProvider` trait、`Message`、`LlmResponse`、`ToolCall`、`ToolDefinition`、`FailoverError`、`FailoverReason`、`ProviderError`。所有其他文件的基础 |
| `src/openai_compat.rs` | **OpenAI 兼容 HTTP Provider**：原生 reqwest 实现，构建请求体、解析响应、错误映射。含 3 个测试 |
| `src/external.rs` | **外部插件 Provider**：通过 `geekclaw-plugin` 的 JSON-RPC 调用 Python 插件。含 2 个测试 |
| `src/fallback.rs` | `FallbackChain`：故障转移编排、候选者解析、最近成功缓存。含 3 个测试 |
| `src/cooldown.rs` | `CooldownTracker`：指数退避冷却管理。含 3 个测试 |
| `src/error_classifier.rs` | 错误分类器：40+ 模式匹配，HTTP 状态码 + 消息文本。含 4 个测试 |
| `src/model_ref.rs` | `ModelRef` 解析：`provider/model` 格式、provider 名称规范化。含 5 个测试 |
| `src/lib.rs` | 模块导出 |

## 核心 Trait

```rust
#[async_trait]
pub trait LlmProvider: Send + Sync {
    async fn chat(
        &self,
        messages: &[Message],
        tools: &[ToolDefinition],
        model: &str,
        options: &HashMap<String, Value>,
    ) -> Result<LlmResponse, ProviderError>;

    fn default_model(&self) -> &str;
}
```

## 协议说明

### OpenAI Chat Completions API

`OpenAICompatProvider` 与 OpenAI 兼容端点通信：

```
POST {base_url}/chat/completions
Authorization: Bearer {api_key}
Content-Type: application/json

{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Hello"}],
  "tools": [...],
  "max_tokens": 8192,
  "temperature": 0.7
}
```

响应解析映射：
- `choices[0].message.content` → `LlmResponse.content`
- `choices[0].message.tool_calls` → `LlmResponse.tool_calls`
- `choices[0].finish_reason` → `LlmResponse.finish_reason`
- `usage.*` → `LlmResponse.usage`

### 外部插件协议

`ExternalProvider` 通过 JSON-RPC 调用 `chat` 方法，参数和返回值格式与 OpenAI 一致。详见 `crates/plugin/README.md`。

## 算法说明

### 错误分类器（40+ 模式）

`classify_error()` 按优先级顺序匹配：

```
1. HTTP 状态码匹配：
   401/403 → Auth | 402 → Billing | 408 → Timeout
   429 → RateLimit | 400 → Format | 500/502/503 → Timeout

2. 消息文本匹配（优先级从高到低）：
   RateLimit: "rate limit", "429", "too many requests", "quota exceeded", ...
   Overloaded: "overloaded" → 视为 RateLimit
   Billing: "402", "payment required", "insufficient credits", ...
   Timeout: "timeout", "timed out", "deadline exceeded", ...
   Auth: "invalid api key", "unauthorized", "401", "403", ...
   Format: "tool_use.id", "invalid request format", ...

3. 图像错误（特殊处理）：
   "image dimensions exceed max" → Format（不可重试）
   "image exceeds.*mb" → Format（不可重试）
```

### 冷却退避算法

**标准冷却**（非计费错误）：

```
cooldown = min(1h, 1min × 5^min(n-1, 3))

n=1 → 1 分钟
n=2 → 5 分钟
n=3 → 25 分钟
n=4+ → 1 小时（上限）
```

**计费冷却**（402 等）：

```
cooldown = min(24h, 5h × 2^min(n-1, 10))

n=1 → 5 小时
n=2 → 10 小时
n=3 → 20 小时
n=4+ → 24 小时（上限）
```

**故障窗口**：24 小时内无故障则重置错误计数。

### 故障转移链

```
FallbackChain::execute(candidates, run_fn):
    1. 检查最近成功缓存（TTL 5 分钟）
       命中 → 直接调用 → 成功则返回

    2. 遍历 candidates:
       a. 检查 cooldown.is_available(provider)
          否 → skip（记录 skipped attempt）
       b. 调用 run_fn(provider, model)
          成功 → 缓存成功候选 → 返回
          失败 → classify_error:
            - 不可重试 (Format) → 立即返回错误
            - 可重试 → mark_failure → continue
            - 无法分类 → 立即返回错误

    3. 全部失败/跳过 → FallbackExhaustedError
```

### Provider 名称规范化

```
"claude" → "anthropic"
"gpt" → "openai"
"google" → "gemini"
"glm" → "zhipu"
"z.ai" | "z-ai" → "zai"
"qwen" → "qwen-portal"
```

## 设计决策

- `FailoverError.is_retriable()` 只有 Format 不可重试，其他都触发故障转移
- 最近成功缓存的 key 是候选者列表的指纹，不同候选组合互不影响
- `CooldownTracker` 用 `std::sync::RwLock`（非 tokio），因为操作都是内存计算，不需要 async

## 不完善之处

- **无流式响应**：`OpenAICompatProvider` 只支持非流式调用，不支持 SSE streaming
- **无请求重试**：Go 版本对 timeout 错误有 2 次重试 + 指数退避，Rust 版没有
- **无上下文压缩**：Go 版本在 context_length_exceeded 时会自动摘要压缩，Rust 版直接返回错误
- **无 Anthropic 原生支持**：Anthropic Messages API 格式与 OpenAI 不同（system 单独字段、tool_use block），当前只能通过兼容代理或外部插件调用
- **无图像/多模态支持**：Message 有 `media` 字段但 OpenAICompatProvider 不处理它
- **无 token 计数**：依赖 LLM API 返回的 usage，不能在本地预估 token 数
- **无请求/响应日志**：生产环境需要记录请求响应用于调试，当前只有 debug 级日志
