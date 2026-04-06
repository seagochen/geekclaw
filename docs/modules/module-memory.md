# 模块：会话存储（Memory / Session）

## 模块概述

| 项目 | 内容 |
|------|------|
| 目录 | `pkg/memory/`（存储引擎）、`pkg/session/`（后端适配）|
| 职责 | 对话历史的持久化存储、摘要管理、历史截断与压缩 |
| 核心类型 | `JSONLStore`, `SessionStore`, `sessionMeta` |
| 依赖模块 | fileutil, providers（Message 类型）|

---

## 文件清单

| 文件 | 职责 |
|------|------|
| `memory/jsonl.go` | `JSONLStore` — 基于 JSONL 的追加式消息存储 |
| `session/jsonl_backend.go` | `JSONLBackend` — 将 JSONLStore 适配为 SessionStore 接口 |

---

## 存储结构

每个会话对应两个文件：

```
sessions/
├── agent_main_telegram_12345.jsonl       # 消息数据（追加式）
├── ...
logs/sessions/
├── agent_main_telegram_12345.meta.json   # 元数据（摘要、偏移量）
```

### JSONL 文件格式

每行一条 JSON 编码的 `providers.Message`：

```json
{"role":"user","content":"你好"}
{"role":"assistant","content":"你好！有什么可以帮你的？"}
{"role":"user","content":"帮我查天气"}
```

### Meta 文件格式

```json
{
  "key": "agent:main:telegram:12345",
  "summary": "用户询问了天气和新闻...",
  "skip": 42,
  "count": 50,
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-04-06T12:30:00Z"
}
```

- `skip`: 逻辑截断偏移量，`GetHistory` 跳过前 N 行不反序列化
- `count`: 文件中的总行数

---

## 核心操作

| 方法 | 说明 |
|------|------|
| `AddMessage` / `AddFullMessage` | 追加消息到 JSONL（带 fsync）|
| `GetHistory` | 读取 skip 之后的所有消息 |
| `GetSummary` / `SetSummary` | 读写会话摘要 |
| `TruncateHistory(keepLast)` | 逻辑截断，仅更新 meta.skip |
| `Compact` | 物理重写 JSONL，丢弃已跳过的行 |
| `SetHistory` | 完全替换消息历史 |

---

## 关键实现说明

### 追加式写入

消息只追加不修改，配合 fsync 保证崩溃安全。`TruncateHistory` 不删除物理行，而是在 meta 中记录 `skip` 偏移量，`GetHistory` 跳过这些行。

### 元数据内存缓存

`readMeta` 优先从内存 `metaCache` 读取，缓存未命中才读磁盘。`writeMeta` 成功后同步更新缓存。这避免了每次 `AddMessage` 都产生两次磁盘读写。

### 分片锁

使用 64 个固定的 `sync.Mutex` 分片（通过 FNV 哈希映射），确保并发访问不同会话时不互相阻塞，同时内存使用为 O(1) 不随会话数增长。

### 崩溃恢复

- JSONL 追加与 meta 更新之间的崩溃：`TruncateHistory` 会重新计数修正 `meta.Count`
- `SetHistory` / `Compact` 先写 meta 再重写 JSONL：如果中间崩溃，最坏情况是返回"过多"消息而非丢失数据
