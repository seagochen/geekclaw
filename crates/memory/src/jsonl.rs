// 基于 JSONL 文件的会话持久化存储实现。
//
// 每个会话存储为两个文件：
//   {sanitized_key}.jsonl      — 每行一条 JSON 编码的消息，仅追加
//   {sanitized_key}.meta.json  — 会话元数据（摘要、逻辑截断偏移量）
//
// 消息不会从 JSONL 文件中物理删除。truncate 在元数据文件中记录 "skip"
// 偏移量，get_history 忽略该偏移量之前的行。

use std::collections::HashMap;
use std::path::PathBuf;

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use tokio::fs;
use tokio::io::AsyncWriteExt;
use tokio::sync::Mutex;

use crate::error::{MemoryError, Result};
use crate::types::Message;
use crate::SessionStore;

/// 分片锁数量。使用固定大小数组，内存使用不随会话数增长。
const NUM_LOCK_SHARDS: usize = 64;

/// 每会话元数据，存储在 .meta.json 侧车文件中。
#[derive(Debug, Clone, Serialize, Deserialize)]
struct SessionMeta {
    key: String,
    #[serde(default)]
    summary: String,
    #[serde(default)]
    skip: usize,
    #[serde(default)]
    count: usize,
    #[serde(default)]
    created_at: Option<DateTime<Utc>>,
    #[serde(default)]
    updated_at: Option<DateTime<Utc>>,
}

impl SessionMeta {
    fn new(key: &str) -> Self {
        Self {
            key: key.to_string(),
            summary: String::new(),
            skip: 0,
            count: 0,
            created_at: None,
            updated_at: None,
        }
    }
}

/// JSONL 会话存储。线程安全，通过分片锁实现每会话并发控制。
pub struct JSONLStore {
    /// JSONL 数据文件目录
    dir: PathBuf,
    /// 元数据文件目录
    meta_dir: PathBuf,
    /// 分片互斥锁，通过 FNV 哈希将会话键映射到固定分片
    locks: [Mutex<()>; NUM_LOCK_SHARDS],
    /// 元数据内存缓存，避免每次操作都读磁盘
    meta_cache: Mutex<HashMap<String, SessionMeta>>,
}

impl JSONLStore {
    /// 创建新的 JSONL 存储。自动创建数据目录和元数据目录。
    pub async fn new(dir: impl Into<PathBuf>, meta_dir: impl Into<PathBuf>) -> Result<Self> {
        let dir = dir.into();
        let meta_dir = meta_dir.into();
        fs::create_dir_all(&dir).await?;
        fs::create_dir_all(&meta_dir).await?;

        Ok(Self {
            dir,
            meta_dir,
            locks: std::array::from_fn(|_| Mutex::new(())),
            meta_cache: Mutex::new(HashMap::new()),
        })
    }

    /// 返回给定会话键对应的分片锁索引。使用 FNV-1a 哈希。
    fn shard_index(key: &str) -> usize {
        let mut h: u32 = 0x811c_9dc5; // FNV offset basis
        for b in key.as_bytes() {
            h ^= *b as u32;
            h = h.wrapping_mul(0x0100_0193); // FNV prime
        }
        h as usize % NUM_LOCK_SHARDS
    }

    /// JSONL 数据文件路径。
    fn jsonl_path(&self, key: &str) -> PathBuf {
        self.dir.join(format!("{}.jsonl", sanitize_key(key)))
    }

    /// 元数据文件路径。
    fn meta_path(&self, key: &str) -> PathBuf {
        self.meta_dir
            .join(format!("{}.meta.json", sanitize_key(key)))
    }

    /// 读取元数据，优先从内存缓存获取。缓存未命中则从磁盘加载。
    async fn read_meta(&self, key: &str) -> Result<SessionMeta> {
        // 先查缓存
        {
            let cache = self.meta_cache.lock().await;
            if let Some(cached) = cache.get(key) {
                return Ok(cached.clone());
            }
        }

        // 缓存未命中，从磁盘读取
        let path = self.meta_path(key);
        let meta = match fs::read(&path).await {
            Ok(data) => serde_json::from_slice::<SessionMeta>(&data)?,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => SessionMeta::new(key),
            Err(e) => return Err(MemoryError::Io(e)),
        };

        // 写入缓存
        let mut cache = self.meta_cache.lock().await;
        cache.insert(key.to_string(), meta.clone());
        Ok(meta)
    }

    /// 原子写入元数据文件（临时文件 + 重命名）并更新缓存。
    async fn write_meta(&self, key: &str, meta: SessionMeta) -> Result<()> {
        let path = self.meta_path(key);
        let data = serde_json::to_vec_pretty(&meta)?;

        // 原子写入：先写临时文件，再重命名
        let tmp_path = path.with_extension("meta.json.tmp");
        let mut f = fs::File::create(&tmp_path).await?;
        f.write_all(&data).await?;
        f.sync_all().await?;
        fs::rename(&tmp_path, &path).await?;

        // 写入成功后更新缓存
        let mut cache = self.meta_cache.lock().await;
        cache.insert(key.to_string(), meta);
        Ok(())
    }

    /// 统计 JSONL 文件中非空行的总数，无需反序列化。
    async fn count_lines(&self, key: &str) -> Result<usize> {
        let path = self.jsonl_path(key);
        let content = match fs::read_to_string(&path).await {
            Ok(c) => c,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(0),
            Err(e) => return Err(MemoryError::Io(e)),
        };

        Ok(content.lines().filter(|line| !line.is_empty()).count())
    }
}

/// 将会话键转换为安全的文件名组件。
/// 替换 ':', '/', '\' 为 '_'，确保复合 ID 不会创建子目录。
fn sanitize_key(key: &str) -> String {
    key.replace(':', "_")
        .replace('/', "_")
        .replace('\\', "_")
}

/// 从 JSONL 文件读取消息，跳过前 `skip` 行而不进行反序列化。
/// 格式错误的行会被静默跳过并记录日志。
fn read_messages(content: &str, skip: usize) -> Vec<Message> {
    let mut msgs = Vec::new();
    let mut line_num: usize = 0;

    for line in content.lines() {
        if line.is_empty() {
            continue;
        }
        line_num += 1;
        if line_num <= skip {
            continue;
        }
        match serde_json::from_str::<Message>(line) {
            Ok(msg) => msgs.push(msg),
            Err(e) => {
                // 损坏的行 — 可能是崩溃导致的不完整写入。
                // 记录日志但不使整个读取失败，标准 JSONL 恢复模式。
                tracing::warn!(
                    line_num = line_num,
                    error = %e,
                    "memory: 跳过损坏的行"
                );
            }
        }
    }

    msgs
}

#[async_trait::async_trait]
impl SessionStore for JSONLStore {
    async fn append(&self, key: &str, msg: &Message) -> Result<()> {
        let _guard = self.locks[Self::shard_index(key)].lock().await;

        // 将消息序列化为单行 JSON 并追加
        let mut line = serde_json::to_vec(msg)?;
        line.push(b'\n');

        let path = self.jsonl_path(key);
        let mut f = fs::OpenOptions::new()
            .create(true)
            .append(true)
            .open(&path)
            .await?;
        f.write_all(&line).await?;
        // 刷新到物理存储，确保断电不丢数据
        f.sync_all().await?;

        // 更新元数据
        let mut meta = self.read_meta(key).await?;
        let now = Utc::now();
        if meta.count == 0 && meta.created_at.is_none() {
            meta.created_at = Some(now);
        }
        meta.count += 1;
        meta.updated_at = Some(now);

        self.write_meta(key, meta).await
    }

    async fn get_history(&self, key: &str, limit: usize) -> Result<Vec<Message>> {
        let _guard = self.locks[Self::shard_index(key)].lock().await;

        let meta = self.read_meta(key).await?;

        let path = self.jsonl_path(key);
        let content = match fs::read_to_string(&path).await {
            Ok(c) => c,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(e) => return Err(MemoryError::Io(e)),
        };

        let msgs = read_messages(&content, meta.skip);

        // 如果 limit > 0，只返回最后 limit 条消息
        if limit > 0 && msgs.len() > limit {
            Ok(msgs[msgs.len() - limit..].to_vec())
        } else {
            Ok(msgs)
        }
    }

    async fn truncate(&self, key: &str, keep: usize) -> Result<()> {
        let _guard = self.locks[Self::shard_index(key)].lock().await;

        let mut meta = self.read_meta(key).await?;

        // 始终与磁盘上的实际行数校正，防止崩溃导致 meta.count 过时
        let n = self.count_lines(key).await?;
        meta.count = n;

        if keep == 0 {
            meta.skip = meta.count;
        } else {
            let effective = meta.count.saturating_sub(meta.skip);
            if keep < effective {
                meta.skip = meta.count - keep;
            }
        }
        meta.updated_at = Some(Utc::now());

        self.write_meta(key, meta).await
    }

    async fn set_summary(&self, key: &str, summary: &str) -> Result<()> {
        let _guard = self.locks[Self::shard_index(key)].lock().await;

        let mut meta = self.read_meta(key).await?;
        let now = Utc::now();
        if meta.created_at.is_none() {
            meta.created_at = Some(now);
        }
        meta.summary = summary.to_string();
        meta.updated_at = Some(now);

        self.write_meta(key, meta).await
    }

    async fn get_summary(&self, key: &str) -> Result<Option<String>> {
        let _guard = self.locks[Self::shard_index(key)].lock().await;

        let meta = self.read_meta(key).await?;
        if meta.summary.is_empty() {
            Ok(None)
        } else {
            Ok(Some(meta.summary))
        }
    }

    async fn count(&self, key: &str) -> Result<usize> {
        let _guard = self.locks[Self::shard_index(key)].lock().await;

        let meta = self.read_meta(key).await?;
        // 返回逻辑上可见的消息数
        Ok(meta.count.saturating_sub(meta.skip))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 辅助函数：创建临时目录并返回 JSONLStore
    async fn setup() -> (JSONLStore, tempfile::TempDir) {
        let tmp = tempfile::tempdir().unwrap();
        let dir = tmp.path().join("data");
        let meta_dir = tmp.path().join("meta");
        let store = JSONLStore::new(&dir, &meta_dir).await.unwrap();
        (store, tmp)
    }

    fn msg(role: &str, content: &str) -> Message {
        Message {
            role: role.to_string(),
            content: content.to_string(),
            ..Default::default()
        }
    }

    #[tokio::test]
    async fn test_append_and_get_history() {
        let (store, _tmp) = setup().await;

        store.append("s1", &msg("user", "你好")).await.unwrap();
        store
            .append("s1", &msg("assistant", "你好！有什么可以帮你的？"))
            .await
            .unwrap();

        let history = store.get_history("s1", 0).await.unwrap();
        assert_eq!(history.len(), 2);
        assert_eq!(history[0].role, "user");
        assert_eq!(history[0].content, "你好");
        assert_eq!(history[1].role, "assistant");
    }

    #[tokio::test]
    async fn test_get_history_with_limit() {
        let (store, _tmp) = setup().await;

        for i in 0..5 {
            store
                .append("s1", &msg("user", &format!("msg {i}")))
                .await
                .unwrap();
        }

        let history = store.get_history("s1", 2).await.unwrap();
        assert_eq!(history.len(), 2);
        assert_eq!(history[0].content, "msg 3");
        assert_eq!(history[1].content, "msg 4");
    }

    #[tokio::test]
    async fn test_truncate() {
        let (store, _tmp) = setup().await;

        for i in 0..10 {
            store
                .append("s1", &msg("user", &format!("msg {i}")))
                .await
                .unwrap();
        }

        // 截断保留最后 3 条
        store.truncate("s1", 3).await.unwrap();

        let history = store.get_history("s1", 0).await.unwrap();
        assert_eq!(history.len(), 3);
        assert_eq!(history[0].content, "msg 7");
        assert_eq!(history[1].content, "msg 8");
        assert_eq!(history[2].content, "msg 9");
    }

    #[tokio::test]
    async fn test_truncate_all() {
        let (store, _tmp) = setup().await;

        store.append("s1", &msg("user", "hi")).await.unwrap();
        store.truncate("s1", 0).await.unwrap();

        let history = store.get_history("s1", 0).await.unwrap();
        assert!(history.is_empty());
    }

    #[tokio::test]
    async fn test_set_and_get_summary() {
        let (store, _tmp) = setup().await;

        // 空摘要返回 None
        let summary = store.get_summary("s1").await.unwrap();
        assert!(summary.is_none());

        store
            .set_summary("s1", "这是一段关于 Rust 的对话")
            .await
            .unwrap();

        let summary = store.get_summary("s1").await.unwrap();
        assert_eq!(summary.unwrap(), "这是一段关于 Rust 的对话");
    }

    #[tokio::test]
    async fn test_count() {
        let (store, _tmp) = setup().await;

        assert_eq!(store.count("s1").await.unwrap(), 0);

        store.append("s1", &msg("user", "a")).await.unwrap();
        store.append("s1", &msg("assistant", "b")).await.unwrap();
        assert_eq!(store.count("s1").await.unwrap(), 2);

        store.truncate("s1", 1).await.unwrap();
        assert_eq!(store.count("s1").await.unwrap(), 1);
    }

    #[tokio::test]
    async fn test_sanitize_key() {
        assert_eq!(sanitize_key("chat:123/456"), "chat_123_456");
        assert_eq!(sanitize_key("simple"), "simple");
        assert_eq!(sanitize_key("a\\b"), "a_b");
    }

    #[tokio::test]
    async fn test_empty_history() {
        let (store, _tmp) = setup().await;

        let history = store.get_history("nonexistent", 0).await.unwrap();
        assert!(history.is_empty());
    }

    #[tokio::test]
    async fn test_corrupt_line_skipped() {
        let (store, _tmp) = setup().await;

        // 手动写入一行损坏数据
        let path = store.jsonl_path("s1");
        fs::write(&path, "not valid json\n").await.unwrap();

        // 应该跳过损坏的行而不报错
        let history = store.get_history("s1", 0).await.unwrap();
        assert!(history.is_empty());
    }

    #[tokio::test]
    async fn test_concurrent_sessions() {
        let (store, _tmp) = setup().await;
        let store = std::sync::Arc::new(store);

        let mut handles = Vec::new();
        for i in 0..10 {
            let s = store.clone();
            handles.push(tokio::spawn(async move {
                let key = format!("session_{i}");
                s.append(&key, &msg("user", &format!("hello from {i}")))
                    .await
                    .unwrap();
                let h = s.get_history(&key, 0).await.unwrap();
                assert_eq!(h.len(), 1);
            }));
        }
        for h in handles {
            h.await.unwrap();
        }
    }
}
