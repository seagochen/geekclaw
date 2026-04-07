# GeekClaw 项目指南

## 项目概述

GeekClaw 是一个超轻量级个人 AI Agent，用 Rust 编写（从 Go 迁移），通过 JSON-RPC over stdio 与 Python 插件通信。最小核心设计，仅保留 Agent 能独立运行的必要模块。

## 技术栈

- **语言**: Rust（核心）、Python 3.12（插件）
- **异步运行时**: Tokio
- **通信**: JSON-RPC 2.0 over stdin/stdout
- **存储**: JSONL 文件（会话历史）、JSON 文件（定时任务、状态）
- **依赖**: 见 `Cargo.toml` workspace dependencies

## 目录结构

```
geekclaw/
├── Cargo.toml          # Workspace root
├── TASKS.md            # 迁移计划与进度
├── DEFERRED.md         # 非核心模块文档（已移除功能的记录）
├── crates/
│   ├── agent/          # Agent 核心层（消息循环、LLM 调用编排、上下文/token 管理）
│   ├── bus/            # 进程内消息总线（tokio mpsc）
│   ├── config/         # YAML 配置加载 + GEEKCLAW_ 环境变量覆盖
│   ├── cron/           # 定时任务调度（cron 表达式 + 一次性 + 间隔）
│   ├── geekclaw/       # CLI 入口（clap）
│   ├── logger/         # 结构化日志（tracing + tracing-subscriber）
│   ├── memory/         # JSONL 会话持久化引擎（分片锁 + 元数据缓存）
│   ├── plugin/         # 外部插件进程管理（JSON-RPC 2.0 传输层）
│   ├── providers/      # LLM Provider trait + 故障转移链 + 冷却追踪 + 错误分类
│   └── tools/          # Tool trait + 注册表 + 并发执行
├── geekclaw/           # [遗留] Go 源码（迁移完成后删除）
└── docs/               # 文档
```

## 构建与测试

```bash
# 构建
cargo build

# 运行所有测试
cargo test

# 运行特定 crate 测试
cargo test -p geekclaw-agent
cargo test -p geekclaw-memory
cargo test -p geekclaw-providers

# 检查编译（不生成二进制）
cargo check

# 运行二进制
cargo run -- version
cargo run -- agent
```

## 核心架构模式

### 消息流

```
Inbound → MessageBus (mpsc) → AgentLoop → LLM + Tools → MessageBus (mpsc) → Outbound
```

### 并发模型

- **消息总线**: `tokio::sync::mpsc` 异步通道（缓冲区 64）
- **工具执行**: `tokio::spawn` + `Semaphore` 并发控制（上限 10），每个任务有 panic recovery
- **会话存储**: 64 分片 `tokio::sync::Mutex`，不同会话并行，同一分片串行
- **故障转移**: `FallbackChain` 按序尝试候选 Provider，冷却期指数退避

### 关闭顺序

```
1. CancellationToken → 2. AgentLoop 退出 → 3. Bus 关闭
```

## 编码规范

- 中文注释和日志
- 核心抽象用 trait 定义（`LlmProvider`, `Tool`, `SessionStore`）
- 错误处理：`thiserror` 定义错误类型，`anyhow` 用于应用层
- 错误分类：`providers::classify_error` 结构化分类，支持 40+ 模式
- 插件环境变量：自动过滤 `LD_PRELOAD` 等危险变量
- 零 `unsafe` 代码

## 配置

主配置文件 `config.yaml`，支持环境变量覆盖（`GEEKCLAW_` 前缀）。关键配置项：

| 环境变量 | 说明 |
|----------|------|
| `GEEKCLAW_MODEL` | 默认 LLM 模型 |
| `GEEKCLAW_MAX_TOOL_ITERATIONS` | 最大工具迭代次数 |
| `GEEKCLAW_SESSION_TIMEOUT` | 单次会话超时（秒） |
| `GEEKCLAW_MAX_CONTEXT_TOKENS` | 最大上下文 token 数 |
| `GEEKCLAW_LOG` | 日志级别（默认 info） |
| `GEEKCLAW_LOG_JSON` | 启用 JSON 日志格式（1/true） |

## 核心 Trait

```rust
// LLM Provider
#[async_trait]
pub trait LlmProvider: Send + Sync {
    async fn chat(&self, messages: &[Message], tools: &[ToolDefinition],
                  model: &str, options: &HashMap<String, Value>) -> Result<LlmResponse>;
    fn default_model(&self) -> &str;
}

// 工具
#[async_trait]
pub trait Tool: Send + Sync {
    fn name(&self) -> &str;
    fn description(&self) -> &str;
    fn parameters(&self) -> Value;
    async fn execute(&self, args: Value, ctx: &ToolContext) -> ToolResult;
}

// 会话存储
#[async_trait]
pub trait SessionStore: Send + Sync {
    async fn append(&self, key: &str, msg: &Message) -> Result<()>;
    async fn get_history(&self, key: &str, limit: usize) -> Result<Vec<Message>>;
    async fn truncate(&self, key: &str, keep: usize) -> Result<()>;
    async fn set_summary(&self, key: &str, summary: &str) -> Result<()>;
    async fn get_summary(&self, key: &str) -> Result<Option<String>>;
    async fn count(&self, key: &str) -> Result<usize>;
}
```

## 文档

- [架构总览](docs/architecture.md)
- [数据流](docs/data-flow.md)
- [JSON-RPC 插件协议](docs/json-rpc-protocol.md)
- [非核心模块记录](DEFERRED.md)
