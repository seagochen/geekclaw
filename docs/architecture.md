# GeekClaw 架构总览

## 项目定位

GeekClaw 是一个**最小核心 AI Agent**，用 Rust 编写。它保留了一个 Agent 能独立运行所需的最小模块集，设计目标是帮助理解"大型 Agent 如何构造"。

## 技术栈

- **语言**: Rust (edition 2021)
- **异步运行时**: Tokio
- **通信**: JSON-RPC 2.0 over stdio（与 Python 插件）
- **存储**: JSONL 文件（会话历史）、JSON 文件（定时任务）
- **CLI**: clap

## Crate 架构

```
geekclaw (bin)          ← CLI 入口 + 交互式对话循环
    │
    ├── geekclaw-agent  ← 核心循环：消息 → LLM → 工具 → 响应
    │   ├── geekclaw-bus        ← 消息总线 (tokio mpsc)
    │   ├── geekclaw-memory     ← JSONL 会话持久化
    │   ├── geekclaw-providers  ← LLM 调用 + 故障转移
    │   │   └── geekclaw-plugin ← JSON-RPC 2.0 传输层
    │   ├── geekclaw-tools      ← 工具系统 + 内置工具
    │   │   └── geekclaw-cron   ← 定时任务调度
    │   └── geekclaw-config     ← YAML 配置加载
    │
    └── geekclaw-logger ← 结构化日志
```

## 消息流

```
用户输入
    │
    ▼
InboundMessage ──→ MessageBus (mpsc) ──→ AgentLoop
                                            │
                                    ┌───────┴───────┐
                                    ▼               ▼
                              ContextBuilder   SessionStore
                              (系统提示词 +      (加载历史)
                               token 裁剪)
                                    │
                                    ▼
                              LlmProvider.chat()
                              (FallbackChain 故障转移)
                                    │
                            ┌───────┴───────┐
                            │ tool_calls?   │
                            ▼               ▼
                      ToolRegistry      直接返回
                      .execute_batch()  content
                            │
                            ▼
                      工具结果 → 再次调用 LLM
                            │
                            ▼
                      最终 content
                            │
                            ▼
OutboundMessage ──→ MessageBus (mpsc) ──→ 输出到用户
```

## 并发模型

| 组件 | 机制 | 说明 |
|------|------|------|
| 消息总线 | `tokio::sync::mpsc` | 缓冲区 64，异步无阻塞 |
| 工具执行 | `tokio::spawn` + `Semaphore(10)` | 最多 10 个工具并发，每个有 panic recovery |
| 会话存储 | 64 分片 `tokio::sync::Mutex` | FNV 哈希分片，不同会话并行 |
| 故障转移 | `FallbackChain` + `CooldownTracker` | 指数退避冷却，最近成功缓存 |

## 关闭顺序

```
CancellationToken → AgentLoop 退出 → Bus 关闭
```

## 扩展点

所有核心抽象都是 trait，方便添加新实现：

- `LlmProvider` — 添加新的 LLM 后端
- `Tool` — 添加新的内置工具
- `SessionStore` — 替换存储后端（如 SQLite）

被移除的非核心模块（channels, commands, skills, routing 等）记录在 [DEFERRED.md](../DEFERRED.md)，可按需重新实现。
