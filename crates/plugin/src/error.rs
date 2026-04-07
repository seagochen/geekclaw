//! 插件模块的错误类型定义。

use crate::wire::RpcError;

/// 插件操作中可能发生的错误。
#[derive(Debug, thiserror::Error)]
pub enum PluginError {
    /// JSON 序列化/反序列化失败
    #[error("JSON 错误: {0}")]
    Json(#[from] serde_json::Error),

    /// IO 操作失败
    #[error("IO 错误: {0}")]
    Io(#[from] std::io::Error),

    /// JSON-RPC 远端返回错误
    #[error("RPC 错误 (code={code}): {message}")]
    Rpc {
        code: i32,
        message: String,
        data: Option<serde_json::Value>,
    },

    /// 请求超时或被取消
    #[error("请求已取消")]
    Cancelled,

    /// 插件进程已退出
    #[error("插件进程已退出")]
    ProcessExited,

    /// 响应通道关闭（内部错误）
    #[error("响应通道关闭")]
    ChannelClosed,

    /// 配置错误
    #[error("配置错误: {0}")]
    Config(String),
}

impl From<RpcError> for PluginError {
    fn from(e: RpcError) -> Self {
        PluginError::Rpc {
            code: e.code,
            message: e.message,
            data: e.data,
        }
    }
}

pub type Result<T> = std::result::Result<T, PluginError>;
