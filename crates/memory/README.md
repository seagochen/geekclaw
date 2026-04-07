# geekclaw-memory — JSONL 会话持久化

## 模块概述

基于 JSONL 文件的会话历史存储引擎。每个会话存储为追加式 `.jsonl` 文件 + `.meta.json` 元数据侧车文件。通过分片锁实现高并发，通过逻辑截断保证崩溃安全。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/lib.rs` | `SessionStore` trait 定义（6 个异步方法），模块导出 |
| `src/jsonl.rs` | `JSONLStore` 实现：分片锁、JSONL 读写、元数据缓存、原子写入。包含 10 个单元测试 |
| `src/types.rs` | 类型复用：直接 re-export `geekclaw_providers::Message`，避免 agent 层做类型转换 |
| `src/error.rs` | `MemoryError`：IO、JSON 序列化错误 |
| `benches/jsonl_bench.rs` | 性能基准测试：append、get_history、并发会话 |

## 核心 Trait

```rust
#[async_trait]
pub trait SessionStore: Send + Sync {
    async fn append(&self, key: &str, msg: &Message) -> Result<()>;
    async fn get_history(&self, key: &str, limit: usize) -> Result<Vec<Message>>;
    async fn truncate(&self, key: &str, keep: usize) -> Result<()>;
    async fn set_summary(&self, key: &str, summary: &str) -> Result<()>;
    async fn get_summary(&self, key: &str) -> Result<Option<String>>;
    async fn count(&self, key: &str) -> Result<usize>;
}
```

## 算法说明

### FNV-1a 分片哈希

将会话 key 映射到 64 个互斥锁分片，实现不同会话的并行访问：

```
hash = 0x811c9dc5 (FNV offset basis)
for each byte in key:
    hash ^= byte
    hash *= 0x01000193 (FNV prime)
shard_index = hash % 64
```

选择 64 个分片是因为：
- 固定大小数组，内存使用不随会话数增长（对长时间运行的守护进程很重要）
- 64 足够分散锁竞争，同时不浪费内存

### 逻辑截断

`truncate(key, keep)` 不删除 JSONL 文件中的任何行，而是在 meta 文件中记录 `skip` 偏移量：

```
truncate(keep=3):
  total_lines = 10
  skip = 10 - 3 = 7

get_history():
  读取所有行 → 跳过前 7 行 → 返回后 3 行
```

这保证了：
- 所有写入都是追加式的，崩溃时最多丢失最后一行
- 不需要重写文件，性能好
- meta 文件是独立的，即使损坏也不影响消息数据

### 原子写入

元数据文件使用"写临时文件 + rename"模式：

```
write(meta.json.tmp, data)
fsync(meta.json.tmp)
rename(meta.json.tmp → meta.json)
```

JSONL 文件使用 append + fsync，每次写入一行后立即同步到磁盘。

## 性能数据

（在 WSL2 环境下的基准测试结果）

| 操作 | 性能 |
|------|------|
| append (含 fsync) | ~8.5ms/op |
| get_history (1000 条) | ~0.23ms/op |
| get_history (limit=10, 1000 条) | ~0.20ms/op |
| 50 并发会话 × 20 writes | ~909ms |

append 较慢是因为每次都 fsync，这是数据持久性的保证。

## 设计决策

- 复用 `geekclaw_providers::Message` 类型而非定义自己的，避免 agent 层做转换
- 元数据内存缓存（`Mutex<HashMap>`）减少磁盘读取
- 会话 key 中的 `:`, `/`, `\` 替换为 `_` 用于生成安全的文件名

## 不完善之处

- **无自动压缩**：JSONL 文件只增不减，长时间运行会膨胀。Go 版本有 summarize 机制（LLM 摘要 + 截断），Rust 版暂未实现
- **无并发写入优化**：同一分片的写入是串行的，高频同会话写入可能成为瓶颈
- **全量读取**：`get_history` 读取整个文件然后跳过，大文件时效率低。可优化为从文件尾部反向读取
- **无迁移机制**：Go 版本有 `migration.go` 处理格式变更，Rust 版没有
- **无内存上限**：元数据缓存没有 LRU 淘汰，会话数很多时缓存会持续增长
