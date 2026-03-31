# 项目概览

## 基本信息

| 项目 | 内容 |
|------|------|
| 项目名称 | GeekClaw |
| Go Module | `github.com/seagosoft/geekclaw` |
| 编程语言 | Go 1.25.7（核心）、Python 3.12（插件 SDK）|
| 构建工具 | `go build -tags stdjson ./...` |
| 构建约束 | CGO 禁用，标准库 JSON |
| 入口文件 | `cmd/geekclaw/main.go` |
| 核心命令 | `geekclaw gateway`（启动 AI 网关服务）|
| 定位 | 超轻量级个人 AI Agent 网关，支持多渠道、多模型、多工具的对话管理 |

---

## 统计信息

| 类别 | 数量 |
|------|------|
| 接口 (interface) | 22 |
| 结构体 (struct) | 85+ |
| Go 包 (package) | 30+ |
| 外部插件类型 | 7 |
| 内置工具 (Tool) | 18 |
| 内置渠道类型 | 14（均为 Python 插件）|
| Python 插件文件 | 30+ |

---

## 核心类型清单与文件定位

### 消息总线 (`pkg/bus`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `MessageBus` | struct | `pkg/bus/bus.go` | 进程内 pub/sub 总线，3 个带缓冲 channel |
| `InboundMessage` | struct | `pkg/bus/types.go` | 入站消息（渠道→Agent）|
| `OutboundMessage` | struct | `pkg/bus/types.go` | 出站文本消息（Agent→渠道）|
| `OutboundMediaMessage` | struct | `pkg/bus/types.go` | 出站媒体消息 |
| `MediaPart` | struct | `pkg/bus/types.go` | 单个媒体分片 |
| `Peer` | struct | `pkg/bus/types.go` | 路由用对端标识（频道、群组等）|
| `SenderInfo` | struct | `pkg/bus/types.go` | 发送者身份信息 |

### 配置系统 (`pkg/config`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `Config` | struct | `pkg/config/config.go` | 根配置，包含所有子配置 |
| `AgentsConfig` | struct | `pkg/config/config.go` | Agent 列表 + 默认值 |
| `AgentDefaults` | struct | `pkg/config/config.go` | Agent 全局默认配置 |
| `AgentConfig` | struct | `pkg/config/config.go` | 单个 Agent 配置 |
| `AgentBinding` | struct | `pkg/config/config.go` | Agent 路由绑定规则 |
| `ModelConfig` | struct | `pkg/config/config.go` | 模型列表条目（含 protocol/model-id）|
| `RoutingConfig` | struct | `pkg/config/config.go` | 复杂度路由配置 |
| `MCPConfig` | struct | `pkg/config/config.go` | MCP 全局配置 |
| `MCPServerConfig` | struct | `pkg/config/config.go` | 单个 MCP 服务器配置 |
| `ToolsConfig` | struct | `pkg/config/config.go` | 工具开关与参数配置 |
| `ExecConfig` | struct | `pkg/config/config.go` | exec 工具配置（超时、白/黑名单）|
| `GatewayConfig` | struct | `pkg/config/config.go` | HTTP 网关配置 |
| `HeartbeatConfig` | struct | `pkg/config/config.go` | 心跳检查配置 |
| `GroupTriggerConfig` | struct | `pkg/config/config.go` | 群组触发器配置 |
| `rrCounter` | struct | `pkg/config/model_resolution.go` | 轮询计数器（模型负载均衡）|

### Provider 系统 (`pkg/providers`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `LLMProvider` | interface | `pkg/providers/provider.go` | LLM 调用核心接口 |
| `StatefulProvider` | interface | `pkg/providers/provider.go` | 有状态 Provider（带 Close）|
| `ThinkingCapable` | interface | `pkg/providers/provider.go` | 支持扩展思考的 Provider |
| `FallbackChain` | struct | `pkg/providers/fallback.go` | 多候选模型故障转移链 |
| `FallbackCandidate` | struct | `pkg/providers/fallback.go` | 候选项（Provider + Model）|
| `FallbackResult` | struct | `pkg/providers/fallback.go` | 转移执行结果 |
| `FallbackExhaustedError` | struct | `pkg/providers/fallback.go` | 所有候选均失败的错误 |
| `FailoverError` | struct | `pkg/providers/failover_error.go` | 带原因的故障转移错误 |
| `CooldownTracker` | struct | `pkg/providers/cooldown.go` | 滑动窗口冷却跟踪器 |

### Provider 协议类型 (`pkg/providers/protocoltypes`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `Message` | struct | `protocoltypes/types.go` | LLM 消息（含 ToolCalls、Media）|
| `LLMResponse` | struct | `protocoltypes/types.go` | LLM 响应（含 ToolCalls、Usage）|
| `ToolCall` | struct | `protocoltypes/types.go` | 工具调用描述 |
| `FunctionCall` | struct | `protocoltypes/types.go` | 函数调用详情 |
| `ContentBlock` | struct | `protocoltypes/types.go` | 内容分块（含 KV Cache 控制）|
| `CacheControl` | struct | `protocoltypes/types.go` | Anthropic KV Cache 控制 |
| `UsageInfo` | struct | `protocoltypes/types.go` | Token 用量统计 |
| `ReasoningDetail` | struct | `protocoltypes/types.go` | 扩展思考详情 |

### Agent 系统 (`pkg/agent`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `AgentInstance` | struct | `pkg/agent/instance.go` | 单个 Agent 实例（模型、工具、会话）|
| `AgentLoop` | struct | `pkg/agent/loop.go` | 主消息处理循环 |
| `AgentRegistry` | struct | `pkg/agent/registry.go` | 多 Agent 注册表 + 路由解析 |
| `ContextBuilder` | struct | `pkg/agent/context.go` | 系统提示词构建器（带缓存）|
| `processOptions` | struct | `pkg/agent/loop.go` | 单次消息处理配置 |
| `MemoryStore` | struct | `pkg/agent/memory.go` | Agent 记忆存储 |

### 渠道系统 (`pkg/channels`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `Channel` | interface | `pkg/channels/base.go` | 渠道核心接口 |
| `TypingCapable` | interface | `pkg/channels/interfaces.go` | 可发送"正在输入"的渠道 |
| `MessageEditor` | interface | `pkg/channels/interfaces.go` | 可编辑消息的渠道 |
| `ReactionCapable` | interface | `pkg/channels/interfaces.go` | 可添加表情反应的渠道 |
| `PlaceholderCapable` | interface | `pkg/channels/interfaces.go` | 可发送占位消息的渠道 |
| `PlaceholderRecorder` | interface | `pkg/channels/interfaces.go` | 记录占位/输入状态 |
| `WebhookHandler` | interface | `pkg/channels/interfaces.go` | 提供 HTTP Webhook 的渠道 |
| `CommandRegistrarCapable` | interface | `pkg/channels/interfaces.go` | 可向平台注册斜杠命令 |
| `MediaStoreAware` | interface | `pkg/channels/interfaces.go` | 感知 MediaStore 的渠道 |
| `BaseChannel` | struct | `pkg/channels/base.go` | 渠道基类（嵌入至所有渠道）|
| `Manager` | struct | `pkg/channels/manager.go` | 渠道生命周期 + 分发管理器 |
| `IsInternalChannel` | func | `pkg/channels/constants.go` | 判断渠道是否为内部渠道 |

### 工具系统 (`pkg/tools`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `Tool` | interface | `pkg/tools/base.go` | 工具核心接口 |
| `AsyncExecutor` | interface | `pkg/tools/base.go` | 异步工具接口 |
| `AsyncCallback` | type | `pkg/tools/base.go` | 异步回调函数类型 |
| `ToolResult` | struct | `pkg/tools/result.go` | 工具执行结果（含 ForLLM/ForUser）|
| `ToolRegistry` | struct | `pkg/tools/registry.go` | 工具注册表（含 TTL 机制）|
| `ToolEntry` | struct | `pkg/tools/registry.go` | 注册条目（工具 + TTL）|
| `ToolLoopConfig` | struct | `pkg/tools/toolloop.go` | 工具循环配置 |
| `ToolLoopResult` | struct | `pkg/tools/toolloop.go` | 工具循环结果 |
| `ExecTool` | struct | `pkg/tools/shell.go` | Shell 命令执行工具 |
| `MCPManager` | interface | `pkg/tools/mcp_tool.go` | MCP 管理器接口 |
| `MCPTool` | struct | `pkg/tools/mcp_tool.go` | MCP 工具包装器 |
| `WebSearchTool` | struct | `pkg/tools/web.go` | Web 搜索工具 |
| `WebFetchTool` | struct | `pkg/tools/web_fetch.go` | 网页抓取工具 |
| `SubagentManager` | struct | `pkg/tools/subagent.go` | 子 Agent 管理器 |
| `SubagentTask` | struct | `pkg/tools/subagent.go` | 子 Agent 任务 |
| `SubagentTool` | struct | `pkg/tools/subagent.go` | 同步子 Agent 工具 |
| `SearchProvider` | interface | `pkg/tools/web.go` | 搜索供应商接口 |

### 会话与路由 (`pkg/session`, `pkg/routing`)

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `SessionStore` | interface | `pkg/session/session_store.go` | 会话存储接口 |
| `Session` | struct | `pkg/session/manager.go` | 会话数据（消息历史 + 摘要）|
| `JSONLBackend` | struct | `pkg/session/jsonl_backend.go` | JSONL 文件存储后端 |
| `Router` | struct | `pkg/routing/router.go` | 复杂度路由器 |
| `RouteResolver` | struct | `pkg/routing/resolver.go` | Agent 路由解析器 |
| `ResolvedRoute` | struct | `pkg/routing/resolver.go` | 路由解析结果 |

### 基础设施

| 类型名 | 类型 | 文件 | 简要说明 |
|--------|------|------|----------|
| `HeartbeatService` | struct | `pkg/heartbeat/service.go` | 周期性心跳检查服务 |
| `Service` | struct | `pkg/devices/service.go` | USB/设备事件监听服务 |
| `EventSource` | interface | `pkg/devices/events/events.go` | 设备事件源接口 |
| `DeviceEvent` | struct | `pkg/devices/events/events.go` | 设备事件数据 |

---

## 目录结构

```
geekclaw/
├── cmd/geekclaw/               # 二进制入口
│   ├── main.go                 # Cobra 命令树根
│   └── internal/               # CLI 子命令实现
│       ├── agent/              # agent 子命令（交互模式）
│       ├── auth/               # auth 子命令（登录/登出）
│       ├── cron/               # cron 子命令（定时任务管理）
│       ├── gateway/            # gateway 子命令（启动网关服务）
│       ├── skills/             # skills 子命令（技能管理）
│       ├── status/             # status 子命令
│       └── version/            # version 子命令
│
├── pkg/                        # 核心库包
│   ├── agent/                  # Agent 核心逻辑（循环、实例、上下文）
│   ├── auth/                   # OAuth 认证 + Token 存储
│   ├── bus/                    # 进程内消息总线
│   ├── channels/               # 渠道抽象 + 管理器
│   ├── commands/               # 斜杠命令系统
│   ├── config/                 # 配置加载与类型定义
│   ├── cron/                   # 定时任务调度服务
│   ├── devices/                # USB/设备事件监听
│   ├── fileutil/               # 原子文件写入工具
│   ├── health/                 # HTTP 健康检查端点
│   ├── heartbeat/              # 周期性心跳检查
│   ├── interactive/            # 交互式确认管理器
│   ├── logger/                 # 结构化日志
│   ├── mcp/                    # Model Context Protocol 客户端
│   ├── media/                  # 媒体文件生命周期管理
│   ├── memory/                 # JSONL 会话持久化
│   ├── migrate/                # 数据迁移工具
│   ├── plugin/                 # 插件子进程生命周期管理
│   ├── providers/              # LLM Provider 抽象层
│   │   ├── anthropic/          # Anthropic SDK 适配器
│   │   ├── external/           # 外部插件 Provider
│   │   ├── openai_compat/      # OpenAI 兼容 HTTP 适配器
│   │   └── protocoltypes/      # 协议共享类型
│   ├── routing/                # Agent 路由 + 模型选择
│   ├── session/                # 会话存储接口与实现
│   ├── skills/                 # 技能加载器与注册表
│   ├── state/                  # 状态持久化（最后活跃渠道）
│   ├── tasks/                  # 任务取消队列
│   ├── tools/                  # 工具接口 + 内置工具实现
│   │   └── external/           # 外部工具/搜索插件
│   ├── utils/                  # BM25、HTTP 重试、字符串、媒体工具
│   └── voice/                  # 语音转录
│       └── external/           # 外部转录插件
│
├── templates/                  # 运行环境模板（由 build.sh install 使用）
│   ├── configs/                # config.example.yaml 配置模板
│   └── scripts/                # geekclaw.sh 服务管理脚本模板
│
└── plugins/                    # Python 插件生态
    ├── sdk/                    # Python 插件 SDK
    ├── auth/contrib/           # 认证插件
    ├── channels/contrib/       # 渠道插件（telegram、discord 等 14 种）
    ├── commands/contrib/       # 命令插件
    ├── providers/contrib/      # Provider 插件
    ├── search/contrib/         # 搜索插件
    ├── tools/contrib/          # 工具插件
    ├── voice/contrib/          # 语音转录插件
    ├── memory/                 # 记忆人格文件
    ├── persona/                # 人格配置文件
    └── skills/                 # 内置技能包（github、tmux、weather 等）
```
