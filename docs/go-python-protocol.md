# Go-Python 插件通信协议

本文档详细描述 GeekClaw 的 Go 主进程如何通过 JSON-RPC 协议与 Python 外部插件进行双向通信。

## 1. 架构概览

```
┌──────────────────────────────────┐     stdin (JSON-RPC)     ┌─────────────────────────────┐
│          Go 主进程                │ ──────────────────────► │       Python 插件进程         │
│                                  │                          │                             │
│  pkg/channels/external/          │     stdout (JSON-RPC)    │  plugins/channels/contrib/  │
│  pkg/plugin/                     │ ◄────────────────────── │  plugins/sdk/               │
│                                  │                          │                             │
│  ExternalChannel                 │     stderr (日志)        │  BaseChannel                │
│  Transport                       │ ◄────────────────────── │  StdioTransport             │
└──────────────────────────────────┘                          └─────────────────────────────┘
```

- **Go → Python**：通过 stdin 发送 JSON-RPC **请求**（如发送消息、开始输入指示器）
- **Python → Go**：通过 stdout 发送 JSON-RPC **响应**和**通知**（如收到新消息）
- **Python stderr**：日志输出，被 Go 捕获并记录到主日志

## 2. 线路格式：JSON-RPC 2.0

**核心代码**：`pkg/plugin/wire.go`

所有消息均为换行符分隔的 JSON（每行一个完整的 JSON 对象）。

### 2.1 请求（Go → Python）

```json
{"jsonrpc":"2.0","id":1,"method":"channel.send","params":{"chat_id":"123","content":"Hello"}}
```

```go
type Request struct {
    JSONRPC string `json:"jsonrpc"`          // 固定 "2.0"
    ID      int64  `json:"id"`               // 自增请求 ID
    Method  string `json:"method"`           // 方法名
    Params  any    `json:"params,omitempty"` // 参数
}
```

### 2.2 响应（Python → Go）

```json
{"jsonrpc":"2.0","id":1,"result":{}}
```

```go
type Response struct {
    JSONRPC string    `json:"jsonrpc"`
    ID      int64     `json:"id"`               // 匹配请求 ID
    Result  any       `json:"result,omitempty"`
    Error   *RPCError `json:"error,omitempty"`
}
```

### 2.3 通知（Python → Go，无 ID）

```json
{"jsonrpc":"2.0","method":"channel.message","params":{"sender_id":"alice","chat_id":"123","content":"Hi"}}
```

```go
type Notification struct {
    JSONRPC string `json:"jsonrpc"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}
```

### 2.4 错误

```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"rate limited","data":null}}
```

```go
type RPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data,omitempty"`
}
```

## 3. RPC 方法定义

**核心代码**：`pkg/channels/external/protocol.go`

### 3.1 Go → Python（RPC 请求）

| 方法 | 说明 | 参数 |
|------|------|------|
| `channel.initialize` | 初始化握手 | `{name, config}` |
| `channel.start` | 激活通道 | 无 |
| `channel.stop` | 优雅关闭 | 无 |
| `channel.send` | 发送文本消息 | `{chat_id, content, reply_to_message_id?}` |
| `channel.send_media` | 发送媒体附件 | `{chat_id, parts[]}` |
| `channel.start_typing` | 显示输入指示器 | `{chat_id}` |
| `channel.stop_typing` | 隐藏输入指示器 | `{chat_id, stop_id}` |
| `channel.edit_message` | 编辑已发送消息 | `{chat_id, message_id, content}` |
| `channel.react` | 添加表情回应 | `{chat_id, message_id, emoji}` |
| `channel.undo_react` | 移除表情回应 | `{chat_id, message_id, emoji}` |
| `channel.send_placeholder` | 发送占位消息 | `{chat_id, content}` |
| `channel.register_commands` | 注册 Bot 命令 | `{commands[]}` |

### 3.2 Python → Go（通知）

| 方法 | 说明 | 参数 |
|------|------|------|
| `channel.message` | 收到新消息 | `InboundMessageParams` |
| `channel.log` | 日志消息 | `{level, message}` |

## 4. 核心数据结构

### 4.1 初始化参数与结果

```go
// Go → Python
type InitializeParams struct {
    Name   string         `json:"name"`   // 通道名称
    Config map[string]any `json:"config"` // 通道配置（来自 config.yaml）
}

// Python → Go
type InitializeResult struct {
    Capabilities     []string `json:"capabilities"`       // 能力列表
    MaxMessageLength int      `json:"max_message_length"` // 消息长度限制
}
```

支持的 capabilities：`typing`、`edit`、`reaction`、`placeholder`、`media`、`commands`

### 4.2 发送消息参数

```go
type SendParams struct {
    ChatID           string `json:"chat_id"`
    Content          string `json:"content"`
    ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}
```

### 4.3 入站消息参数

```go
type InboundMessageParams struct {
    SenderID    string            `json:"sender_id"`
    ChatID      string            `json:"chat_id"`
    Content     string            `json:"content"`
    MessageID   string            `json:"message_id,omitempty"`
    Media       []string          `json:"media,omitempty"`
    PeerKind    string            `json:"peer_kind,omitempty"`   // "direct" | "group"
    PeerID      string            `json:"peer_id,omitempty"`
    Platform    string            `json:"platform,omitempty"`
    PlatformID  string            `json:"platform_id,omitempty"`
    Username    string            `json:"username,omitempty"`
    DisplayName string            `json:"display_name,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
}
```

### 4.4 媒体部分

```go
type MediaPart struct {
    Type        string `json:"type"`         // "image" | "audio" | "video" | "file"
    Data        string `json:"data"`         // base64 编码
    Caption     string `json:"caption,omitempty"`
    Filename    string `json:"filename,omitempty"`
    ContentType string `json:"content_type,omitempty"`
}
```

## 5. 进程生命周期

**核心代码**：`pkg/channels/external/channel.go`

### 5.1 启动流程

```
ExternalChannel.Start()
  │
  ├─ 1. 创建子进程
  │     exec.CommandContext(ctx, "python3", "-m", "channels.contrib.telegram_httpx")
  │     设置环境变量（OS env + config.Env）
  │     创建 stdin/stdout 管道
  │     stderr → logWriter（日志捕获）
  │
  ├─ 2. 创建 Transport
  │     Transport(stdout_reader, stdin_writer)
  │     启动 ReadLoop goroutine（持续读取 stdout）
  │     启动通知处理 goroutine
  │
  ├─ 3. 初始化握手
  │     发送 channel.initialize → 等待响应
  │     解析 capabilities
  │
  ├─ 4. 激活通道
  │     发送 channel.start → 等待响应
  │
  └─ 5. 标记运行中
        running = true
```

### 5.2 关闭流程

```
ExternalChannel.Stop()
  │
  ├─ 1. 优雅关闭（5 秒超时）
  │     发送 channel.stop RPC（best effort）
  │
  ├─ 2. 取消上下文
  │     cancel() → 子进程收到信号
  │
  ├─ 3. 等待退出
  │     cmd.Wait()
  │     wg.Wait()（等待所有 goroutine）
  │
  └─ 4. 标记停止
        running = false
```

## 6. 完整消息生命周期

以 Telegram 消息为例，展示从用户发送到收到回复的完整流程：

```
阶段 1：用户发送消息
═══════════════════

Telegram 服务器                    Python 插件                         Go 主进程
     │                               │                                   │
     │ ── HTTP long-poll 响应 ──►     │                                   │
     │    {message: "你好"}           │                                   │
     │                               │                                   │
     │                               │ ── channel.message 通知 ──────►    │
     │                               │    (via stdout)                    │
     │                               │    {"jsonrpc":"2.0",              │
     │                               │     "method":"channel.message",   │
     │                               │     "params":{                    │
     │                               │       "sender_id":"12345",        │
     │                               │       "chat_id":"12345",          │
     │                               │       "content":"你好",            │
     │                               │       "platform":"telegram"       │
     │                               │     }}                            │
     │                               │                                   │
     │                               │                                   ├─ handleInboundMessage()
     │                               │                                   ├─ 检查 allow_from
     │                               │                                   ├─ 发布到消息总线
     │                               │                                   │

阶段 2：Agent 处理
═══════════════════

     │                               │                                   │
     │                               │                                   ├─ Agent 从总线读取
     │                               │                                   ├─ 构建提示词 + 调用 LLM
     │                               │                                   ├─ 执行工具（如需要）
     │                               │                                   ├─ 生成回复
     │                               │                                   │

阶段 3：发送回复
═══════════════════

     │                               │                                   │
     │                               │ ◄── channel.send 请求 ──────────  │
     │                               │    (via stdin)                    │
     │                               │    {"jsonrpc":"2.0","id":2,      │
     │                               │     "method":"channel.send",     │
     │                               │     "params":{                    │
     │                               │       "chat_id":"12345",          │
     │                               │       "content":"你好！有什么..."  │
     │                               │     }}                            │
     │                               │                                   │
     │ ◄── HTTP sendMessage ────     │                                   │
     │    Telegram Bot API 调用       │                                   │
     │                               │                                   │
     │                               │ ── 响应 ──────────────────────►   │
     │                               │    {"jsonrpc":"2.0","id":2,      │
     │                               │     "result":{}}                  │
```

## 7. 错误处理与重试

**核心代码**：`pkg/channels/rate_limiter.go`

### 7.1 错误码定义

| 错误码 | 常量 | 说明 | 是否重试 |
|--------|------|------|----------|
| `-32001` | `RATE_LIMITED` | 平台限流（如 Telegram 429） | 是，固定 1 秒延迟 |
| `-32002` | `TEMPORARY` | 临时错误（网络超时、5xx） | 是，指数退避 |
| `-32003` | `NOT_RUNNING` | 通道未运行 | 否 |
| `-32004` | `SEND_FAILED` | 永久发送失败（无效 chat_id、4xx） | 否 |

### 7.2 重试策略

```go
maxRetries  = 3
baseBackoff = 500ms

// 指数退避序列：500ms → 1s → 2s → 4s → 8s（上限）
```

```
发送消息
  │
  ├─ 成功 → 完成
  │
  ├─ RATE_LIMITED → 等 1 秒 → 重试
  │
  ├─ TEMPORARY → 指数退避 → 重试（最多 3 次）
  │
  ├─ NOT_RUNNING → 立即失败
  │
  └─ SEND_FAILED → 立即失败
```

### 7.3 速率限制

每个通道有独立的速率限制器：

| 通道 | 速率（消息/秒） |
|------|-----------------|
| Telegram | 20 |
| Discord | 1 |
| Slack | 1 |
| Matrix | 2 |
| LINE | 10 |
| QQ | 5 |
| IRC | 2 |

## 8. Python SDK 层

### 8.1 传输层

**核心代码**：`plugins/sdk/transport.py`

```python
class StdioTransport:
    async def start(self):
        """初始化异步 stdin/stdout 流"""

    def register_handler(self, method: str, handler: Callable):
        """注册 RPC 方法处理器"""

    async def read_loop(self):
        """主事件循环：读取 JSON → 分发到处理器 → 发送响应"""

    async def send_notification(self, method: str, params: Any = None):
        """发送通知到 Go（无需响应）"""

    async def send_response(self, req_id: int, result: Any = None):
        """发送 RPC 响应"""

    async def send_error(self, req_id: int, code: int, message: str):
        """发送错误响应"""
```

### 8.2 BasePlugin 基类

**核心代码**：`plugins/sdk/plugin.py`

```python
class BasePlugin:
    def __init__(self):
        self.transport = StdioTransport()
        self.config: dict = {}

    async def on_start(self):
        """子类重写：启动时初始化（如连接平台 API）"""

    async def on_stop(self):
        """子类重写：清理资源"""

    @classmethod
    def run(cls):
        """入口点：创建实例 → 注册处理器 → 启动传输 → 进入事件循环"""
```

### 8.3 BaseChannel 通道基类

**核心代码**：`plugins/channels/channel.py`

```python
class BaseChannel(BasePlugin):
    capabilities: list[str] = []
    max_message_length: int = 0

    # 子类需实现的方法
    async def on_send(self, chat_id, content, reply_to): ...
    async def on_send_media(self, chat_id, parts): ...
    async def on_start_typing(self, chat_id) -> str: ...
    async def on_stop_typing(self, chat_id, stop_id): ...
    async def on_edit_message(self, chat_id, message_id, content): ...
    async def on_react(self, chat_id, message_id, emoji): ...

    # 内置方法
    async def publish_message(self, message: InboundMessage):
        """将入站消息作为通知发送给 Go"""
        await self.transport.send_notification("channel.message", message.to_params())

    def _register_handlers(self):
        """自动注册所有 12 个 RPC 方法处理器"""
        self.transport.register_handler("channel.initialize", self._handle_initialize)
        self.transport.register_handler("channel.start", self._handle_start)
        self.transport.register_handler("channel.send", self._handle_send)
        # ...
```

### 8.4 实现示例：Telegram

```python
class TelegramChannel(BaseChannel):
    capabilities = ["typing", "edit", "reaction", "placeholder", "media"]

    async def on_start(self):
        token = os.environ["GEEKCLAW_TELEGRAM_TOKEN"]
        self.api = TelegramAPI(token)
        asyncio.create_task(self._polling_loop())

    async def on_send(self, chat_id, content, reply_to):
        html = markdown_to_telegram_html(content)
        await self.api.send_message(chat_id=chat_id, text=html, parse_mode="HTML")

    async def _polling_loop(self):
        offset = 0
        while self._running:
            updates = await self.api.get_updates(offset, timeout=30)
            for update in updates:
                msg = update["message"]
                await self.publish_message(InboundMessage(
                    sender_id=str(msg["from"]["id"]),
                    chat_id=str(msg["chat"]["id"]),
                    content=msg.get("text", ""),
                    ...
                ))
                offset = update["update_id"] + 1

if __name__ == "__main__":
    TelegramChannel.run()
```

## 9. 关键源文件索引

### Go 侧

| 文件 | 职责 |
|------|------|
| `pkg/plugin/wire.go` | JSON-RPC 类型定义 |
| `pkg/plugin/transport.go` | 传输层（请求/响应匹配、通知分发） |
| `pkg/plugin/process.go` | 通用进程生命周期管理 |
| `pkg/channels/external/protocol.go` | 方法名和消息结构定义 |
| `pkg/channels/external/channel.go` | 进程启动、RPC 调用、通知处理 |
| `pkg/channels/rate_limiter.go` | 错误分类、重试逻辑、速率限制 |
| `pkg/channels/dispatch.go` | Worker 分发和路由 |
| `pkg/bus/bus.go` | 消息总线（入站/出站通道） |

### Python 侧

| 文件 | 职责 |
|------|------|
| `plugins/sdk/transport.py` | StdioTransport（异步 JSON-RPC over stdio） |
| `plugins/sdk/plugin.py` | BasePlugin 生命周期和入口点 |
| `plugins/channels/channel.py` | BaseChannel（处理器注册、能力声明） |
| `plugins/channels/types.py` | 数据类定义（InboundMessage、SendRequest 等） |
| `plugins/channels/contrib/telegram_httpx.py` | Telegram 通道具体实现 |

## 10. 设计总结

| 特点 | 说明 |
|------|------|
| **标准协议** | JSON-RPC 2.0，成熟可靠，避免自定义线路格式 |
| **双向通信** | 请求（Go→Python）和通知（Python→Go）共用同一管道 |
| **进程隔离** | 每个外部通道是独立子进程，崩溃不影响主进程 |
| **能力协商** | 初始化时声明能力，Go 侧按能力决定是否调用特定方法 |
| **弹性重试** | 错误分级处理：限流固定延迟、临时错误指数退避、永久错误直接失败 |
| **语言无关** | 协议足够简单，任何语言都可以实现插件 |
