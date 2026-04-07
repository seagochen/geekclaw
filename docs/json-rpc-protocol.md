# JSON-RPC 2.0 Plugin Protocol

GeekClaw 通过 JSON-RPC 2.0 over stdio 与外部插件进程通信。这保持了与 Go 版本 Python 插件的兼容性。

## 传输层

- **方向**: GeekClaw (parent) ↔ Plugin (child)
- **编码**: 换行符分隔的 JSON (`\n` delimited)
- **stdin**: GeekClaw → Plugin (requests)
- **stdout**: Plugin → GeekClaw (responses + notifications)
- **stderr**: 转发到 GeekClaw 日志

## 生命周期

```
1. GeekClaw 启动子进程
2. 发送 initialize 请求
3. 正常工作期间发送 RPC 调用
4. 发送 shutdown 请求
5. 等待进程退出（超时后 SIGKILL）
```

## Wire Types

### Request (GeekClaw → Plugin)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "chat",
  "params": {
    "messages": [...],
    "tools": [...],
    "model": "gpt-4o",
    "options": {"max_tokens": 8192}
  }
}
```

### Response (Plugin → GeekClaw)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": "Hello!",
    "tool_calls": [],
    "finish_reason": "stop",
    "usage": {
      "prompt_tokens": 10,
      "completion_tokens": 5,
      "total_tokens": 15
    }
  }
}
```

### Error Response

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32603,
    "message": "Internal error",
    "data": null
  }
}
```

### Notification (Plugin → GeekClaw, no id)

```json
{
  "jsonrpc": "2.0",
  "method": "log",
  "params": {"level": "info", "message": "Provider initialized"}
}
```

## LLM Provider 协议

### `initialize`

```json
// Request
{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}

// Response
{"jsonrpc": "2.0", "id": 1, "result": {"status": "ok"}}
```

### `chat`

```json
// Request
{
  "jsonrpc": "2.0", "id": 2,
  "method": "chat",
  "params": {
    "messages": [
      {"role": "system", "content": "You are helpful."},
      {"role": "user", "content": "Hello"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "shell",
          "description": "Execute shell command",
          "parameters": {"type": "object", "properties": {"command": {"type": "string"}}}
        }
      }
    ],
    "model": "gpt-4o",
    "options": {"max_tokens": 8192, "temperature": 0.7}
  }
}

// Response (text)
{
  "jsonrpc": "2.0", "id": 2,
  "result": {
    "content": "Hello! How can I help?",
    "tool_calls": [],
    "finish_reason": "stop",
    "usage": {"prompt_tokens": 20, "completion_tokens": 8, "total_tokens": 28}
  }
}

// Response (tool call)
{
  "jsonrpc": "2.0", "id": 2,
  "result": {
    "content": "",
    "tool_calls": [{
      "id": "call_abc123",
      "type": "function",
      "function": {"name": "shell", "arguments": "{\"command\": \"ls -la\"}"}
    }],
    "finish_reason": "tool_calls",
    "usage": {"prompt_tokens": 30, "completion_tokens": 15, "total_tokens": 45}
  }
}
```

## 安全

- 启动子进程时自动过滤危险环境变量（`LD_PRELOAD`, `LD_LIBRARY_PATH` 等）
- 子进程 stdout 严格按 JSON-RPC 解析，非 JSON 行被忽略
- 进程退出后自动清理资源
