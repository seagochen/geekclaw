# 模块：健康检查（Health）

## 模块概述

| 项目 | 内容 |
|------|------|
| 目录 | `pkg/health/` |
| 职责 | 提供 HTTP 健康检查端点，用于容器编排和负载均衡 |
| 核心类型 | `Server`, `Check`, `StatusResponse` |
| 依赖模块 | 标准库 `net/http` |

---

## 端点

| 路径 | 方法 | 用途 | 成功状态 | 失败状态 |
|------|------|------|----------|----------|
| `/health` | GET | 存活检查（Liveness）| 200 | — |
| `/ready` | GET | 就绪检查（Readiness）| 200 | 503 |

### 响应格式

```json
{
  "status": "ready",
  "uptime": "2h30m15s",
  "checks": {
    "database": {"name": "database", "status": "ok", "timestamp": "..."},
    "llm": {"name": "llm", "status": "ok", "message": "provider reachable", "timestamp": "..."}
  }
}
```

---

## 使用方式

```go
// 创建健康检查服务器
healthServer := health.NewServer("0.0.0.0", 8080)

// 注册检查项（checkFn 在锁外执行，不阻塞端点）
healthServer.RegisterCheck("llm", func() (bool, string) {
    return provider.Ping(), "provider reachable"
})

// 将端点注册到共享 HTTP mux
healthServer.RegisterOnMux(sharedMux)

// 启动（支持 context 取消）
healthServer.StartContext(ctx)

// 优雅关闭（带 10 秒超时）
healthServer.Stop(shutdownCtx)
```

---

## 关键实现说明

### RegisterCheck 不阻塞

`RegisterCheck` 在锁外执行 `checkFn()`，仅在存储结果时加锁。这避免了慢检查（如网络探测）阻塞整个 `/ready` 端点的读请求。

### 优雅关闭

`StartContext` 在 context 取消时使用 **带 10 秒超时**的 `server.Shutdown()`，确保正在处理的请求有时间完成，同时不会无限等待。
