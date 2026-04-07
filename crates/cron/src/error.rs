// 定时任务模块的错误类型定义。

use thiserror::Error;

/// 定时任务模块统一错误类型。
#[derive(Debug, Error)]
pub enum CronError {
    /// IO 操作失败（文件读写等）
    #[error("IO 错误: {0}")]
    Io(#[from] std::io::Error),

    /// JSON 序列化/反序列化失败
    #[error("JSON 错误: {0}")]
    Json(#[from] serde_json::Error),

    /// 任务未找到
    #[error("任务未找到: {0}")]
    JobNotFound(String),

    /// 无效的 cron 表达式
    #[error("无效的 cron 表达式: {0}")]
    InvalidCronExpr(String),

    /// 服务已在运行
    #[error("服务已在运行")]
    AlreadyRunning,
}

pub type Result<T> = std::result::Result<T, CronError>;
