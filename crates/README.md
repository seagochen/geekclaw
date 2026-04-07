# 项目文件结构说明

## 根目录

```
geekclaw/
├── Cargo.toml              # Workspace 根配置，定义所有 crate 和共享依赖
├── Cargo.lock              # 依赖锁定文件
├── config.example.yaml     # 配置文件模板，build 时自动复制为 config.yaml
├── manage.py               # 构建脚本：build / clean / test / bench
├── geekclaw.py             # CLI 入口封装（自动查找编译后的二进制并转发参数）
├── CLAUDE.md               # AI 助手项目指南（编码规范、架构概览）
├── LICENSE                 # GNUv3 许可证
├── docs/                   # 项目文档
└── crates/                 # Rust 源码（Cargo workspace 成员）
```

## crates/ — 模块详解

项目采用 Cargo Workspace 结构，每个模块是一个独立的 crate。
依赖方向自底向上：底层 crate 不依赖上层，上层组合底层。

```
crates/
├── geekclaw/       # 可执行二进制入口
├── agent/          # Agent 核心循环（顶层编排）
├── bus/            # 消息总线（最底层，无内部依赖）
├── config/         # 配置加载（最底层，无内部依赖）
├── logger/         # 日志初始化（最底层，无内部依赖）
├── memory/         # 会话持久化
├── cron/           # 定时任务调度
├── plugin/         # JSON-RPC 插件通信
├── providers/      # LLM 调用抽象
└── tools/          # 工具系统 + 内置工具
```

### `crates/geekclaw/` — CLI 入口 (1 file, 235 lines)

可执行二进制的入口点。负责：

- **CLI 解析**：使用 clap 定义 `agent` 和 `version` 子命令
- **Provider 创建**：根据 config.yaml 或环境变量创建 LLM Provider
- **组件串联**：创建 bus → memory → tools → agent，启动交互式对话循环
- **交互式 stdin**：读取用户输入，调用 agent.process_message()，打印响应

关键文件：
- `src/main.rs` — 程序入口、配置加载、组件初始化

---

### `crates/agent/` — Agent 核心循环 (3 files, 489 lines)

整个系统的大脑。编排消息处理的完整流程：

```
接收消息 → 加载历史 → 构建上下文 → 调用 LLM → 执行工具 → 保存历史 → 发送响应
```

负责：
- **AgentLoop**：消息消费主循环，通过 CancellationToken 支持优雅关闭
- **ContextBuilder**：组装系统提示词 + 历史消息 + 用户消息，管理 token 预算裁剪
- **LLM 调用循环**：反复调用 LLM 直到无 tool_calls 或达到 max_iterations
- **故障转移集成**：多候选模型时通过 FallbackChain 自动切换

关键文件：
- `src/lib.rs` — AgentInstance 配置、ProcessOptions
- `src/loop_core.rs` — AgentLoop 主循环、LLM 调用、工具执行编排
- `src/context.rs` — ContextBuilder、token 估算、历史裁剪
- `tests/integration.rs` — 集成测试（MockProvider 端到端验证）

---

### `crates/bus/` — 消息总线 (2 files, 218 lines)

进程内的消息路由层，是所有模块间通信的桥梁。

基于 `tokio::sync::mpsc`，提供入站（外部 → Agent）和出站（Agent → 外部）两个异步通道。
缓冲区大小 64，支持多生产者。

关键类型：
- `MessageBus` — 总线实例，持有 inbound/outbound 通道
- `InboundMessage` — 入站消息（channel、sender、content、session_key 等）
- `OutboundMessage` — 出站消息（channel、chat_id、content）

关键文件：
- `src/lib.rs` — MessageBus 实现（publish/consume/close）
- `src/types.rs` — 消息类型定义（InboundMessage、OutboundMessage、SenderInfo）

---

### `crates/config/` — 配置加载 (2 files, 209 lines)

从 YAML 文件加载配置，支持 `GEEKCLAW_` 前缀的环境变量覆盖。

支持的环境变量：
| 变量 | 作用 |
|------|------|
| `GEEKCLAW_MODEL` | 默认模型 |
| `GEEKCLAW_MAX_TOOL_ITERATIONS` | 最大工具迭代次数 |
| `GEEKCLAW_SESSION_TIMEOUT` | 会话超时（秒） |
| `GEEKCLAW_MAX_CONTEXT_TOKENS` | 最大上下文 token 数 |

Provider 的 `api_key` 字段支持 `$ENV_VAR` 语法引用环境变量。

关键文件：
- `src/lib.rs` — Config 结构体、load()、环境变量覆盖
- `src/error.rs` — ConfigError

---

### `crates/logger/` — 结构化日志 (1 file, 28 lines)

最轻量的 crate。基于 `tracing` + `tracing-subscriber` 封装日志初始化。

- 日志级别通过 `GEEKCLAW_LOG` 环境变量控制（默认 `info`）
- JSON 格式通过 `GEEKCLAW_LOG_JSON=1` 启用

关键文件：
- `src/lib.rs` — `init()` 函数

---

### `crates/memory/` — 会话持久化 (4 files, 537 lines)

基于 JSONL 文件的会话历史存储引擎。每个会话两个文件：

- `{key}.jsonl` — 追加式消息日志（一行一条 JSON 消息）
- `{key}.meta.json` — 元数据（摘要、逻辑截断偏移、消息计数）

设计特点：
- **追加式写入**：只 append，不修改已有内容，崩溃安全
- **逻辑截断**：truncate 不删除行，而是在 meta 中记录 skip 偏移量
- **分片锁**：64 个 `tokio::sync::Mutex` 分片，FNV 哈希映射，不同会话并行
- **元数据缓存**：内存中缓存 meta，避免每次操作读磁盘

核心 trait：`SessionStore`（append / get_history / truncate / set_summary / count）

关键文件：
- `src/lib.rs` — SessionStore trait 定义
- `src/jsonl.rs` — JSONLStore 实现（分片锁、原子写入、行解析）
- `src/types.rs` — 复用 providers 的 Message 类型
- `src/error.rs` — MemoryError
- `benches/jsonl_bench.rs` — 性能基准测试

---

### `crates/cron/` — 定时任务调度 (5 files, 1033 lines)

支持三种调度模式的定时任务系统：

| 类型 | 说明 | 示例 |
|------|------|------|
| `cron` | cron 表达式 | `0 9 * * * * *`（每天 9 点） |
| `every` | 固定间隔（毫秒） | `60000`（每分钟） |
| `at` | 一次性时间戳（毫秒） | `1712534400000` |

CronStore 使用 JSON 文件持久化任务列表，支持原子写入（写临时文件 + rename）。
CronService 运行 tick 循环（每 30 秒检查一次到期任务）。

关键文件：
- `src/types.rs` — CronJob、CronSchedule、CronPayload、CronJobState
- `src/store.rs` — CronStore（CRUD + 持久化 + 下次执行时间计算）
- `src/service.rs` — CronService（tick 循环 + 回调触发）
- `src/error.rs` — CronError
- `src/lib.rs` — 模块导出

---

### `crates/plugin/` — JSON-RPC 插件通信 (6 files, 948 lines)

与外部插件进程（如 Python 脚本）的通信基础设施。

通过 JSON-RPC 2.0 over stdin/stdout 协议通信：
- **GeekClaw → Plugin**：通过 stdin 发送 JSON-RPC request
- **Plugin → GeekClaw**：通过 stdout 返回 response 或 notification

PluginProcess 管理完整的子进程生命周期：
1. 启动子进程（过滤 LD_PRELOAD 等危险环境变量）
2. 建立 Transport（BufReader/BufWriter 读写 stdin/stdout）
3. 初始化握手（调用 initialize 方法）
4. 正常 RPC 调用（pending requests map + oneshot channel 分发）
5. 优雅关闭（调用 shutdown + 超时 SIGKILL）

关键文件：
- `src/wire.rs` — JSON-RPC 2.0 wire types（Request、Response、Notification、RpcError）
- `src/transport.rs` — stdio 传输层（读写、请求 ID、响应分发）
- `src/process.rs` — PluginProcess（生命周期管理、spawn、call、stop）
- `src/config.rs` — PluginConfig + 危险环境变量过滤
- `src/error.rs` — PluginError

---

### `crates/providers/` — LLM Provider 抽象 (8 files, 1727 lines)

最大的 crate。统一的 LLM 调用接口 + 故障转移 + 错误分类。

核心 trait：`LlmProvider`（chat / default_model）

两个内置实现：
- **OpenAICompatProvider** — 原生 Rust HTTP 调用（reqwest），支持所有 OpenAI Chat Completions API 兼容端点
- **ExternalProvider** — 通过 JSON-RPC 调用外部 Python 插件

故障转移系统：
- **FallbackChain** — 按序尝试候选 provider/model，遵循冷却期
- **CooldownTracker** — 指数退避（1min → 5min → 25min → 1h），计费错误单独退避（5h → 10h → 24h）
- **ErrorClassifier** — 40+ 模式匹配（HTTP 状态码 + 错误消息），分类为 Auth/RateLimit/Billing/Timeout/Format
- **ModelRef** — `provider/model` 格式解析 + provider 名称规范化

关键文件：
- `src/types.rs` — LlmProvider trait、Message、LlmResponse、ToolCall、ToolDefinition、FailoverError
- `src/openai_compat.rs` — OpenAI 兼容 HTTP Provider
- `src/external.rs` — 外部插件 Provider
- `src/fallback.rs` — FallbackChain（故障转移编排 + 最近成功缓存）
- `src/cooldown.rs` — CooldownTracker（指数退避冷却）
- `src/error_classifier.rs` — 错误分类器（40+ 模式匹配）
- `src/model_ref.rs` — 模型引用解析 + provider 规范化

---

### `crates/tools/` — 工具系统 (6 files, 1138 lines)

定义工具接口和注册表，以及三个内置工具。

核心 trait：`Tool`（name / description / parameters / execute）

**ToolRegistry**：
- 注册/查找工具
- 生成 LLM API 的 ToolDefinition 列表
- `execute_batch()` 并发执行多个工具调用（Semaphore 控制上限 10，每个 task 有 panic recovery）

**内置工具**：

| 工具 | 功能 | 安全措施 |
|------|------|----------|
| `shell` | 执行 shell 命令 | 12 种危险命令拦截模式（rm -rf, sudo, eval 等）+ 超时控制 |
| `filesystem` | 文件读写 + 目录列表 | 64KB 读取大小限制 |
| `cron` | 定时任务管理 | list/add/remove/enable/disable |

关键文件：
- `src/lib.rs` — Tool trait、ToolContext、ToolResult、parse_tool_calls()
- `src/registry.rs` — ToolRegistry（注册、查找、并发执行）
- `src/builtin/shell.rs` — Shell 工具（命令执行 + 危险拦截）
- `src/builtin/filesystem.rs` — 文件系统工具（read/write/list）
- `src/builtin/cron_tool.rs` — 定时任务管理工具

---

## docs/ — 文档

| 文件 | 内容 |
|------|------|
| `architecture.md` | 架构总览：crate 依赖图、消息流、并发模型、扩展点 |
| `data-flow.md` | 数据流细节：agent 循环、故障转移链、JSONL 持久化机制 |
| `json-rpc-protocol.md` | 插件通信协议：wire types、LLM Provider 调用示例 |
| `project-structure.md` | 本文档 |

## 依赖关系图

```
geekclaw (bin)
├── geekclaw-agent
│   ├── geekclaw-bus          (无内部依赖)
│   ├── geekclaw-config       (无内部依赖)
│   ├── geekclaw-memory
│   │   └── geekclaw-providers
│   │       └── geekclaw-plugin (无内部依赖)
│   ├── geekclaw-providers     (同上)
│   └── geekclaw-tools
│       ├── geekclaw-providers (同上)
│       └── geekclaw-cron     (无内部依赖)
├── geekclaw-bus
├── geekclaw-config
├── geekclaw-logger           (无内部依赖)
├── geekclaw-memory
├── geekclaw-providers
└── geekclaw-tools
```
