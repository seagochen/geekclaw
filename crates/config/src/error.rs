use std::path::PathBuf;

/// 配置加载错误。
#[derive(Debug, thiserror::Error)]
pub enum ConfigError {
    #[error("读取配置文件失败 {0}: {1}")]
    Io(PathBuf, #[source] std::io::Error),

    #[error("解析配置文件失败: {0}")]
    Parse(#[from] serde_yaml::Error),
}
