# 架构文档

## 模块划分

| 模块名 | 目录 | 职责 | 核心类型 |
|--------|------|------|----------|
| 消息总线 | `pkg/bus` | 进程内 pub/sub，解耦渠道与 Agent | `MessageBus`, `InboundMessage`, `OutboundMessage` |
| 配置管理 | `pkg/config` | YAML 配置加载、模型解析、环境变量集中管理 | `Config`, `ModelConfig`, `RoutingConfig` |
| Agent 核心 | `pkg/agent` | 消息处理主循环、上下文构建、会话管理 | `AgentLoop`, `AgentInstance`, `ContextBuilder` |
| Provider 层 | `pkg/providers` | LLM 调用抽象、多协议适配、故障转移 | `LLMProvider`, `FallbackChain`, openai_compat |
| 工具系统 | `pkg/tools` | 工具接口定义、注册表、内置工具实现 | `Tool`, `ToolRegistry`, `ToolResult` |
| 渠道系统 | `pkg/channels` | 渠道抽象、生命周期管理、HTTP Webhook 分发 | `Channel`, `BaseChannel`, `Manager` |
| 会话存储 | `pkg/session` | 对话历史持久化（JSONL）| `SessionStore`, `JSONLBackend` |
| 路由系统 | `pkg/routing` | 复杂度路由选模型、多 Agent 路由绑定 | `Router`, `RouteResolver` |
| 技能系统 | `pkg/skills` | Markdown 技能文件加载与注入 | `SkillsLoader` |
| MCP 客户端 | `pkg/mcp` | Model Context Protocol 协议客户端 | `Manager` |
| 插件管理 | `pkg/plugin` | 外部插件子进程生命周期（启动/通信/关闭）| `PluginProcess` |
| 心跳服务 | `pkg/heartbeat` | 定期读取 HEARTBEAT.md 触发 Agent | `HeartbeatService` |
| 设备监听 | `pkg/devices` | USB 等设备事件监听并推送消息 | `Service`, `EventSource` |
| 认证系统 | `pkg/auth` | OAuth Token 存储与刷新 | `TokenStore` |
| 基础设施 | `pkg/{bus,logger,fileutil,state,health}` | 日志、状态持久化、健康检查 | — |

---

## 模块级别依赖关系图

```mermaid
flowchart TD
    subgraph CMD["命令层 (cmd/geekclaw)"]
        Main[main.go\n命令树入口]
        GW[gateway.go\n网关启动]
    end

    subgraph AGENT["Agent 核心层 (pkg/agent)"]
        AL[AgentLoop\n消息主循环]
        AI[AgentInstance\n模型+工具+会话]
        AR[AgentRegistry\n多 Agent 管理]
        CB[ContextBuilder\n系统提示词]
    end

    subgraph PROV["Provider 层 (pkg/providers)"]
        LLMI[LLMProvider\n接口]
        FC[FallbackChain\n故障转移]
        OAI[openai_compat\nOpenAI 兼容适配]
        ANT[anthropic\nAnthropic SDK]
        EXT_P[external\n外部插件 Provider]
    end

    subgraph TOOLS["工具层 (pkg/tools)"]
        TR[ToolRegistry\n工具注册+分发]
        TL[RunToolLoop\n工具执行循环]
        BT[内置工具\nexec/web/message/...]
        EXT_T[external\n外部工具插件]
        MCP_T[MCPTool\nMCP 工具包装]
    end

    subgraph CHAN["渠道层 (pkg/channels)"]
        CM[Manager\n渠道生命周期]
        CH[Channel 接口]
        EXT_C[external\n外部渠道插件]
    end

    subgraph INFRA["基础设施"]
        BUS[MessageBus\n消息总线]
        CFG[Config\n配置]
        SESS[SessionStore\n会话存储]
        ROUTE[Router\n路由器]
        SKILLS[SkillsLoader\n技能加载]
        MCP[MCP Manager\n协议客户端]
        HB[HeartbeatService\n心跳]
        DEV[devices.Service\n设备监听]
        PLUGIN[pkg/plugin\n插件进程管理]
    end

    Main --> GW
    GW --> AL
    GW --> CM
    GW --> HB
    GW --> DEV

    AL --> AR
    AL --> BUS
    AL --> CM
    AL --> SESS

    AR --> AI
    AI --> CB
    AI --> TR
    AI --> LLMI
    AI --> SESS
    AI --> ROUTE

    TR --> BT
    TR --> MCP_T
    TR --> EXT_T
    TL --> TR
    TL --> LLMI

    MCP_T --> MCP
    EXT_T --> PLUGIN

    LLMI --> FC
    FC --> OAI
    FC --> ANT
    FC --> EXT_P

    EXT_P --> PLUGIN
    EXT_C --> PLUGIN

    CM --> CH
    CM --> EXT_C
    CM --> BUS

    CH --> BUS
    CB --> SKILLS
    AL --> CFG
    AI --> CFG

    HB --> BUS
    DEV --> BUS
```

---

## 类级别依赖关系图

### Agent 核心

```mermaid
classDiagram
    direction TB

    namespace agent {
        class AgentLoop {
            -bus MessageBus
            -registry AgentRegistry
            -channelManager Manager
            -fallback FallbackChain
            +Run(ctx)
            -processMessage(ctx, msg)
            -runAgentLoop(ctx, agent, opts)
        }
        class AgentRegistry {
            -agents map~string~AgentInstance
            -resolver RouteResolver
            +ResolveRoute(input) ResolvedRoute
            +GetAgent(id) AgentInstance
        }
        class AgentInstance {
            +ID string
            +Provider LLMProvider
            +Sessions SessionStore
            +Tools ToolRegistry
            +ContextBuilder ContextBuilder
            +Router Router
            +Candidates []FallbackCandidate
        }
        class ContextBuilder {
            -pluginsDir string
            -skillsLoader SkillsLoader
            -memory MemoryStore
            -cachedSystemPrompt string
            +BuildMessages() []Message
            +BuildSystemPromptWithCache() string
        }
    }

    AgentLoop --> AgentRegistry : 查询路由
    AgentLoop --> AgentInstance : 调用
    AgentRegistry --> AgentInstance : 管理
    AgentInstance --> ContextBuilder : 构建上下文
    AgentInstance --> LLMProvider : LLM 调用
    AgentInstance --> ToolRegistry : 工具执行
    AgentInstance --> SessionStore : 会话读写
```

### Provider 层

```mermaid
classDiagram
    direction TB

    namespace providers {
        class LLMProvider {
            <<interface>>
            +Chat(ctx, messages, tools, model, opts) LLMResponse
            +GetDefaultModel() string
        }
        class StatefulProvider {
            <<interface>>
            +Close()
        }
        class ThinkingCapable {
            <<interface>>
            +SupportsThinking() bool
        }
        class FallbackChain {
            -tracker CooldownTracker
            +Execute(ctx, candidates, runFn) FallbackResult
        }
        class CooldownTracker {
            -windows map~string~[]time.Time
            +IsOnCooldown(key) bool
            +RecordFailure(key)
        }
    }

    namespace openai_compat {
        class Provider {
            -apiBase string
            -apiKey string
            -httpClient http.Client
            +Chat(ctx, ...) LLMResponse
        }
    }

    namespace anthropic {
        class Provider {
            -client anthropic.Client
            +Chat(ctx, ...) LLMResponse
            +SupportsThinking() bool
        }
    }

    StatefulProvider --|> LLMProvider : 扩展
    ThinkingCapable --|> LLMProvider : 扩展
    Provider ..|> LLMProvider : 实现 (openai_compat)
    Provider ..|> LLMProvider : 实现 (anthropic)
    Provider ..|> ThinkingCapable : 实现 (anthropic)
    FallbackChain --> LLMProvider : 调用候选
    FallbackChain --> CooldownTracker : 检查冷却
```

### 工具系统

```mermaid
classDiagram
    direction TB

    namespace tools {
        class Tool {
            <<interface>>
            +Name() string
            +Description() string
            +Parameters() map
            +Execute(ctx, args) ToolResult
        }
        class AsyncExecutor {
            <<interface>>
            +ExecuteAsync(ctx, args, cb) ToolResult
        }
        class ToolRegistry {
            -tools map~string~ToolEntry
            +Register(tool)
            +RegisterHidden(tool)
            +PromoteTools(names, ttl)
            +TickTTL()
            +ExecuteWithContext(ctx, name, args, ch, chatID, cb) ToolResult
            +ToProviderDefs() []ToolDefinition
        }
        class ToolResult {
            +ForLLM string
            +ForUser string
            +Silent bool
            +IsError bool
            +Async bool
            +Media []string
            +Err error
        }
        class ExecTool {
            -workingDir string
            -timeout Duration
            -denyPatterns []Regexp
            -customAllowPatterns []Regexp
            +Execute(ctx, args) ToolResult
            +ExecuteUnrestricted(ctx, args) ToolResult
            -guardCommand(cmd, cwd, admin) string
        }
        class MCPTool {
            -manager MCPManager
            -serverName string
            -tool mcp.Tool
            +Name() string
            +Execute(ctx, args) ToolResult
        }
        class SubagentTool {
            -manager SubagentManager
            +Execute(ctx, args) ToolResult
        }
    }

    AsyncExecutor --|> Tool : 扩展
    ToolRegistry --> Tool : 注册与分发
    ExecTool ..|> Tool : 实现
    MCPTool ..|> Tool : 实现
    SubagentTool ..|> Tool : 实现
    WebSearchTool ..|> Tool : 实现
    WebFetchTool ..|> Tool : 实现
    ToolRegistry --> ToolResult : 返回
```

### 渠道系统

```mermaid
classDiagram
    direction TB

    namespace channels {
        class Channel {
            <<interface>>
            +Name() string
            +Start(ctx)
            +Stop(ctx)
            +Send(ctx, msg)
            +IsRunning() bool
            +IsAllowed(senderID) bool
        }
        class TypingCapable {
            <<interface>>
            +StartTyping(ctx, chatID) func
        }
        class MessageEditor {
            <<interface>>
            +EditMessage(ctx, chatID, msgID, content)
        }
        class ReactionCapable {
            <<interface>>
            +ReactToMessage(ctx, chatID, msgID) func
        }
        class PlaceholderCapable {
            <<interface>>
            +SendPlaceholder(ctx, chatID) string
        }
        class WebhookHandler {
            <<interface>>
            +WebhookPath() string
            +HandleWebhook(w, r)
        }
        class BaseChannel {
            -name string
            -allowList []string
            -bus MessageBus
            +HandleMessage(ctx, peer, msgID, ...)
            +IsAllowed(senderID) bool
        }
        class Manager {
            -channels map~string~Channel
            -bus MessageBus
            -mediaStore MediaStore
            +StartAll(ctx)
            +StopAll(ctx)
            +SendMessage(ctx, msg)
            +SetupHTTPServer(addr, health)
        }
    }

    TypingCapable --|> Channel : 可选能力
    MessageEditor --|> Channel : 可选能力
    ReactionCapable --|> Channel : 可选能力
    PlaceholderCapable --|> Channel : 可选能力
    WebhookHandler --|> Channel : 可选能力
    BaseChannel ..|> Channel : 基类实现
    Manager --> Channel : 管理生命周期
    Manager --> BaseChannel : 注入占位/输入状态
```

---

## 第三方依赖

| 库名 | 用途 |
|------|------|
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API 官方 SDK |
| `github.com/openai/openai-go/v3` | OpenAI API 官方 SDK |
| `github.com/modelcontextprotocol/go-sdk` | MCP 协议客户端 |
| `github.com/spf13/cobra` | CLI 命令框架 |
| `github.com/gorilla/mux` | HTTP 路由 |
| `github.com/dromara/carbon/v2` | 时间处理 |
| `github.com/adhocore/gronx` | Cron 表达式解析 |
| `gopkg.in/yaml.v3` | YAML 配置解析 |
| `golang.org/x/net` | 网络工具（HTML 解析等）|
