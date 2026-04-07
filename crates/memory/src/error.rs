// 会话存储错误类型定义。

use thiserror::Error;

/// 会话存储操作可能产生的错误。
#[derive(Debug, Error)]
pub enum MemoryError {
    /// 文件 I/O 错误
    #[error("memory: io error: {0}")]
    Io(#[from] std::io::Error),

    /// JSON 序列化/反序列化错误
    #[error("memory: json error: {0}")]
    Json(#[from] serde_json::Error),

    /// 其他错误
    #[error("memory: {0}")]
    Other(String),
}

pub type Result<T> = std::result::Result<T, MemoryError>;
