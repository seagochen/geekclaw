// 基于 JSONL 文件的会话持久化存储模块。
//
// 提供 SessionStore trait 和 JSONLStore 实现，用于 AI Agent 的
// 会话历史管理。每个会话存储为追加式 JSONL 文件，配合 .meta.json
// 侧车文件记录元数据（摘要、逻辑截断偏移量）。

pub mod error;
pub mod jsonl;
pub mod types;

pub use error::{MemoryError, Result};
pub use jsonl::JSONLStore;
pub use types::{FunctionCall, Message, ToolCall};

/// 会话存储抽象接口。
///
/// 所有方法都是异步的，实现必须是线程安全的（Send + Sync）。
#[async_trait::async_trait]
pub trait SessionStore: Send + Sync {
    /// 向会话追加一条消息。
    async fn append(&self, key: &str, msg: &Message) -> Result<()>;

    /// 获取会话历史。limit=0 表示不限制，否则返回最后 limit 条。
    async fn get_history(&self, key: &str, limit: usize) -> Result<Vec<Message>>;

    /// 截断历史，仅保留最后 keep 条消息（逻辑截断，不删除文件内容）。
    async fn truncate(&self, key: &str, keep: usize) -> Result<()>;

    /// 设置会话摘要。
    async fn set_summary(&self, key: &str, summary: &str) -> Result<()>;

    /// 获取会话摘要。如果未设置返回 None。
    async fn get_summary(&self, key: &str) -> Result<Option<String>>;

    /// 返回会话中逻辑上可见的消息数量。
    async fn count(&self, key: &str) -> Result<usize>;
}
