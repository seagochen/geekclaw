# GeekClaw 项目指南

## 项目概述

GeekClaw 是一个超轻量级个人 AI Agent，用 Go 编写，通过 JSON-RPC over stdio 与 Python 插件通信。支持 Telegram、Discord、Slack 等 14+ 渠道，18+ 内置工具，多 LLM Provider 故障转移。

## 技术栈

- **语言**: Go 1.25.7（核心）、Python 3.12（插件）
- **通信**: JSON-RPC 2.0 over stdin/stdout
- **存储**: JSONL 文件（会话历史）、JSON 文件（定时任务、状态）
- **依赖**: 见 `go.mod`，仅 8 个直接依赖

## 目录结构

```
geekclaw/
├── agent/          # Agent 核心层（消息循环、LLM 调用、工具执行）
├── bus/            # 进程内消息总线（pub/sub）
├── channels/       # 渠道抽象与外部渠道插件桥接
├── commands/       # 斜杠命令注册与分发
├── config/         # YAML 配置加载
├── cron/           # 定时任务调度
├── health/         # HTTP 健康检查端点
├── interactive/    # 交互式确认管理
├── internal/       # 网关启动逻辑
├── logger/         # 结构化日志
├── mcp/            # Model Context Protocol 客户端
├── media/          # 媒体文件存储与生命周期
├── memory/         # JSONL 会话持久化引擎
├── plugin/         # 外部插件进程管理（JSON-RPC 传输）
├── providers/      # LLM Provider 抽象与故障转移
├── routing/        # 多 Agent 路由绑定
├── session/        # 会话存储后端适配
├── skills/         # Markdown 技能文件加载
├── tasks/          # AI 任务队列（支持取消）
├── tools/          # 工具接口、注册表、内置工具
├── utils/          # 通用工具函数
└── voice/          # 语音转录抽象
```

## 构建与测试

```bash
# 构建
go build ./...

# 运行所有测试
go test ./...

# 运行特定模块测试
go test ./geekclaw/agent/...
go test ./geekclaw/cron/...
```

## 核心架构模式

### 消息流

```
Channel (Telegram) → MessageBus (Inbound) → AgentLoop → LLM + Tools → MessageBus (Outbound) → Channel
```

### 并发模型

- **消息处理**: 按 session key 分发到独立 worker，不同会话并行，同一会话串行
- **工具执行**: `sync.WaitGroup` 并发（上限 10），每个 goroutine 有 panic recovery
- **插件启动**: 搜索/工具/命令三类插件并行启动

### 关闭顺序

```
1. cancel context → 2. stopAll channels → 3. stop agentLoop → 4. stop cron → 5. close bus → 6. cleanup
```

先停消息源，再停消费者，最后关总线。

## 编码规范

- 中文注释和日志
- 所有并发操作使用 `sync.Mutex` / `sync.RWMutex`，避免裸 `sync.Map`
- 工具接口：`Name()`, `Description()`, `Parameters()`, `Execute()`
- 错误处理：使用 `providers.ClassifyError` 结构化分类，不依赖字符串匹配
- 插件环境变量：自动过滤 `LD_PRELOAD` 等危险变量

## 配置

主配置文件 `config.yaml`，支持环境变量覆盖（`GEEKCLAW_` 前缀）。关键配置项：

| 配置路径 | 说明 |
|----------|------|
| `agents.defaults.model` | 默认 LLM 模型 |
| `agents.defaults.max_tool_iterations` | 最大工具迭代次数 |
| `agents.defaults.session_timeout` | 单次会话超时（秒） |
| `tools.cron.exec_timeout_minutes` | 定时任务执行超时 |
| `channels.external[]` | 外部渠道插件配置 |

## 文档

- [架构总览](docs/architecture.md)
- [数据流](docs/data-flow.md)
- [Go-Python 协议](docs/go-python-protocol.md)
- [技能系统](docs/skills-system.md)
- 模块文档：`docs/modules/module-*.md`
