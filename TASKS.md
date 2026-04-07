# GeekClaw: Go → Rust 迁移计划

## 目标

将 GeekClaw 从 Go 重写为 Rust，同时精简为**最小核心 Agent**。
保留能让一个 Agent "思考、记忆、行动、调度" 的最小模块集，其余功能移除并记录到 [DEFERRED.md](DEFERRED.md)。

## 设计原则

- **最小核心**：只保留 Agent 能独立运行的必要模块
- **Trait 驱动**：核心抽象用 trait 定义，方便未来扩展
- **Tokio 异步**：IO 密集型操作全部 async
- **零 unsafe**：除非 FFI 绝对必要

## 最小核心模块（7 个）

| 模块 | Rust crate | 职责 | 对应 Go 源码 |
|------|-----------|------|-------------|
| bus | `geekclaw-bus` | 进程内消息路由（inbound/outbound channel） | `bus/` 451 行 |
| memory | `geekclaw-memory` | JSONL 会话持久化 + 摘要截断 | `memory/` 1938 行 |
| cron | `geekclaw-cron` | 定时任务调度（cron 表达式 + 一次性 + 间隔） | `cron/` 645 行 |
| providers | `geekclaw-providers` | LLM Provider trait + 故障转移链 + 冷却追踪 | `providers/` 3646 行 |
| plugin | `geekclaw-plugin` | JSON-RPC 2.0 over stdio 传输层 | `plugin/` 826 行 |
| tools | `geekclaw-tools` | Tool trait + 注册表（不含具体工具实现） | `tools/{types,base,registry}.go` ~500 行 |
| agent | `geekclaw-agent` | 核心循环 + token/context 管理 + 工具执行编排 | `agent/{loop,context,llm}.go` ~2000 行 |

辅助模块（必须但轻量）：

| 模块 | 职责 |
|------|------|
| `geekclaw-config` | YAML 配置加载 + 环境变量覆盖 |
| `geekclaw-logger` | 结构化日志 |

## Rust 项目结构

```
geekclaw/
├── Cargo.toml              # workspace root
├── crates/
│   ├── bus/                # geekclaw-bus
│   ├── memory/             # geekclaw-memory
│   ├── cron/               # geekclaw-cron
│   ├── providers/          # geekclaw-providers
│   ├── plugin/             # geekclaw-plugin
│   ├── tools/              # geekclaw-tools
│   ├── agent/              # geekclaw-agent
│   ├── config/             # geekclaw-config
│   └── logger/             # geekclaw-logger
└── src/
    └── main.rs             # CLI 入口
```

## 关键 Rust 依赖

| 用途 | crate |
|------|-------|
| 异步运行时 | `tokio` |
| 序列化 | `serde`, `serde_json` |
| HTTP 客户端 | `reqwest` |
| YAML 配置 | `serde_yaml` |
| Cron 表达式 | `cron` |
| UUID | `uuid` |
| 日志 | `tracing`, `tracing-subscriber` |
| CLI | `clap` |
| 错误处理 | `thiserror`, `anyhow` |

## 迁移步骤

### Phase 0: 项目搭建 ✅
- [x] 初始化 Cargo workspace
- [x] 创建所有 crate 骨架（Cargo.toml + lib.rs）
- [x] 配置 workspace 依赖共享

### Phase 1: 基础层（无依赖的底层模块） ✅
- [x] **logger** — `tracing` 封装，结构化日志输出
- [x] **config** — YAML 加载 + `GEEKCLAW_` 环境变量覆盖 + 配置结构体
- [x] **bus** — `tokio::sync::mpsc` 实现 InboundMessage/OutboundMessage 路由
- [x] 为以上模块写单元测试

### Phase 2: 存储与通信层 ✅
- [x] **memory** — JSONL 读写 + 分片锁 + 元数据缓存 + 摘要截断
- [x] **plugin** — JSON-RPC 2.0 wire types + stdio Transport + 进程生命周期管理
- [x] **cron** — CronJob/CronStore 结构 + cron 表达式/间隔/一次性三种调度
- [x] 为以上模块写单元测试

### Phase 3: LLM 与工具层 ✅
- [x] **providers** — `LlmProvider` trait + `FallbackChain` + `CooldownTracker` + 错误分类
- [x] **tools** — `Tool` trait + `ToolRegistry` + 工具执行并发控制
- [x] 为以上模块写单元测试

### Phase 4: 核心循环 ✅
- [x] **agent** — AgentLoop 主循环：消息消费 → 上下文构建 → LLM 调用 → 工具执行 → 响应发送
- [x] Token/Context 管理：token 计数、上下文窗口裁剪、系统提示词构建
- [ ] 集成测试：bus → agent → providers → tools 端到端流程

### Phase 5: 入口与集成 ✅
- [x] **main.rs** — clap CLI（agent/version 子命令）
- [ ] 端到端冒烟测试
- [x] 更新 CLAUDE.md 为 Rust 项目
- [x] 删除 Go 代码（2026-04-07）

### Phase 6: Provider 实现与端到端串通
- [x] **OpenAI 兼容 HTTP Provider** — 原生 Rust reqwest 实现，支持 OpenAI/Anthropic/DeepSeek 等兼容 API
- [x] **ExternalProvider** — 通过 JSON-RPC 调用 Python 插件（保持与 Go 版本的插件兼容性）
- [x] **main.rs 完整串联** — 配置 → Provider 创建 → Agent 启动 → ���互式 stdin 输入
- [ ] 端到端冒烟测试（需要实际 API key）

### Phase 7: 内置工具
- [x] **shell** — Shell 命令执行（超时 + 危险命令拦截 12 种模式）
- [x] **filesystem** — 文件读写 + 目录列表（64KB 大小限制）
- [x] **main.rs 注册** — 内置工具自动注册到 ToolRegistry
- [ ] **cron_tool** — 定时任务管理工具（按需实现）
- [ ] 更多工具按需迁移（见 DEFERRED.md）

### 后续工作
- [ ] 集成测试 + 端到端冒烟测试
- [x] 删除 Go 代码（2026-04-07）
- [ ] 性能基准测试

## 核心 Trait 设计草案

```rust
// providers
#[async_trait]
pub trait LlmProvider: Send + Sync {
    async fn chat(
        &self,
        messages: &[Message],
        tools: &[ToolDefinition],
        model: &str,
        options: &Options,
    ) -> Result<LlmResponse>;
    fn default_model(&self) -> &str;
}

// tools
#[async_trait]
pub trait Tool: Send + Sync {
    fn name(&self) -> &str;
    fn description(&self) -> &str;
    fn parameters(&self) -> serde_json::Value;
    async fn execute(&self, args: serde_json::Value, ctx: &ToolContext) -> Result<String>;
}

// memory
#[async_trait]
pub trait SessionStore: Send + Sync {
    async fn append(&self, key: &str, msg: &Message) -> Result<()>;
    async fn get_history(&self, key: &str, limit: usize) -> Result<Vec<Message>>;
    async fn truncate(&self, key: &str, keep: usize) -> Result<()>;
    async fn set_summary(&self, key: &str, summary: &str) -> Result<()>;
}
```

## 非核心功能

所有被移除的模块及其功能记录在 → [DEFERRED.md](DEFERRED.md)
