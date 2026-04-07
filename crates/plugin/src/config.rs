//! 插件配置定义。

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// 外部插件进程的通用配置。
/// 用于命令、搜索、语音和 LLM 提供者插件。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PluginConfig {
    /// 是否启用此插件
    #[serde(default)]
    pub enabled: bool,

    /// 可执行文件路径或命令名
    pub command: String,

    /// 命令行参数
    #[serde(default)]
    pub args: Vec<String>,

    /// 额外环境变量（覆盖同名系统环境变量）
    #[serde(default)]
    pub env: HashMap<String, String>,

    /// 工作目录（可选）
    #[serde(default)]
    pub working_dir: Option<String>,

    /// 初始化时传递给插件的额外配置
    #[serde(default)]
    pub config: HashMap<String, serde_json::Value>,
}

/// 需要过滤的危险环境变量列表。
/// 防止通过配置注入恶意库加载。
pub(crate) const DANGEROUS_ENV_VARS: &[&str] = &[
    "LD_PRELOAD",
    "LD_LIBRARY_PATH",
    "DYLD_INSERT_LIBRARIES",
    "DYLD_LIBRARY_PATH",
    "DYLD_FRAMEWORK_PATH",
];

/// 检查给定的环境变量名是否为危险变量。
pub(crate) fn is_dangerous_env_var(key: &str) -> bool {
    let upper = key.to_uppercase();
    DANGEROUS_ENV_VARS.iter().any(|&v| v == upper)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_dangerous_env_vars() {
        assert!(is_dangerous_env_var("LD_PRELOAD"));
        assert!(is_dangerous_env_var("ld_preload"));
        assert!(is_dangerous_env_var("Ld_Preload"));
        assert!(is_dangerous_env_var("DYLD_INSERT_LIBRARIES"));
        assert!(!is_dangerous_env_var("PATH"));
        assert!(!is_dangerous_env_var("HOME"));
        assert!(!is_dangerous_env_var("PYTHONPATH"));
    }

    #[test]
    fn test_config_deserialization() {
        let json_str = r#"{
            "enabled": true,
            "command": "python3",
            "args": ["-m", "myplugin"],
            "env": {"PLUGIN_DEBUG": "1"},
            "working_dir": "/tmp",
            "config": {"timeout": 30}
        }"#;
        let cfg: PluginConfig = serde_json::from_str(json_str).unwrap();
        assert!(cfg.enabled);
        assert_eq!(cfg.command, "python3");
        assert_eq!(cfg.args, vec!["-m", "myplugin"]);
        assert_eq!(cfg.env.get("PLUGIN_DEBUG").unwrap(), "1");
        assert_eq!(cfg.working_dir.as_deref(), Some("/tmp"));
    }

    #[test]
    fn test_config_defaults() {
        let cfg: PluginConfig = serde_json::from_str(r#"{"command": "echo"}"#).unwrap();
        assert!(!cfg.enabled);
        assert!(cfg.args.is_empty());
        assert!(cfg.env.is_empty());
        assert!(cfg.working_dir.is_none());
    }
}
