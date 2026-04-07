# geekclaw-plugin — JSON-RPC 2.0 插件通信

## 模块概述

与外部插件进程（如 Python 脚本）的通信基础设施。通过 JSON-RPC 2.0 over stdin/stdout 协议通信，管理子进程的完整生命周期：启动 → 握手 → RPC 调用 → 关闭。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/wire.rs` | JSON-RPC 2.0 线路类型：`JsonRpcRequest`、`JsonRpcResponse`、`JsonRpcNotification`、`RpcError`。含序列化/反序列化测试 |
| `src/transport.rs` | stdio 传输层：`Transport` 管理 stdin writer 和 stdout reader；`ServiceHandler` 类型处理反向调用；后台 read loop 三路分发（响应 / 反向调用 / 通知） |
| `src/process.rs` | `PluginProcess`：子进程生命周期管理（spawn / call / stop）；`SpawnOpts` 配置初始化方法名、日志方法名、通知回调等 |
| `src/config.rs` | `PluginConfig`：命令、参数、环境变量、工作目录；`is_dangerous_env_var()` 过滤危险环境变量 |
| `src/error.rs` | `PluginError`：IO、JSON、RPC、超时、配置错误 |
| `src/lib.rs` | 模块导出 |

## 协议说明

### 通信方式

```
GeekClaw (Rust parent) ←→ Plugin (Python child)
         stdin  →  请求（JSON-RPC request，每行一个 JSON）
         stdout ←  响应（JSON-RPC response）+ 通知（notification）
         stderr ←  日志（转发到 tracing）
```

### JSON-RPC 2.0 格式

**Request** (GeekClaw → Plugin):
```json
{"jsonrpc": "2.0", "id": 1, "method": "chat", "params": {...}}
```

**Response** (Plugin → GeekClaw):
```json
{"jsonrpc": "2.0", "id": 1, "result": {...}}
{"jsonrpc": "2.0", "id": 1, "error": {"code": -32603, "message": "..."}}
```

**Notification** (Plugin → GeekClaw, 无 id):
```json
{"jsonrpc": "2.0", "method": "log", "params": {"level": "info", "message": "..."}}
```

### 生命周期协议

```
1. spawn: 启动子进程（过滤危险环境变量）
2. 建立 Transport（BufReader stdout / BufWriter stdin）
3. 启动后台 read loop（三路分发）
4. 发送初始化握手：call(init_method, init_params)
5. 正常工作：call(method, params) → await response
6. 关闭：call(stop_method) → wait(5s) → SIGKILL
```

### 请求分发机制

Transport 内部维护一个 `pending: HashMap<i64, oneshot::Sender>`：

```
call(method, params):
    id = atomic_counter.fetch_add(1)
    tx, rx = oneshot::channel()
    pending.insert(id, tx)
    write_request(id, method, params)
    response = rx.await  // 等待后台 read loop 分发

read_loop:
    for line in stdout.lines():
        parsed = parse_json(line)
        if has id:                    // Response
            tx = pending.remove(id)
            tx.send(parsed.result)
        else if method == log_method: // 日志通知
            tracing::info!(...)
        else if has services[method]: // 反向调用
            result = services[method](params)
            write_response(...)
        else:                         // 自定义通知
            on_notification(parsed)
```

### 危险环境变量过滤

启动子进程时，以下环境变量会被自动移除：

```
LD_PRELOAD, LD_LIBRARY_PATH, DYLD_INSERT_LIBRARIES,
DYLD_LIBRARY_PATH, PYTHONPATH (如果未显式指定)
```

防止恶意插件通过动态链接劫持加载恶意代码。

## 设计决策

- 使用 `oneshot::channel` 分发响应，每个请求独立等待，不阻塞其他请求
- Atomic 递增的请求 ID，保证并发安全
- read loop 在独立 tokio task 中运行，非 JSON 行静默忽略（子进程可能输出调试信息）
- SpawnOpts 让调用者自定义初始化/关闭方法名，适配不同类型的插件

## 不完善之处

- **无请求超时**：`call()` 会无限等待响应，如果插件卡死会永远阻塞。Go 版本通过 context timeout 处理
- **无重连机制**：插件进程崩溃后不会自动重启
- **无流式响应**：不支持 streaming（逐 token 输出），只能等完整响应
- **无批量请求**：JSON-RPC 2.0 支持批量请求，但未实现
- **stderr 处理简单**：只是转发到日志，不解析结构
