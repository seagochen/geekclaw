// 定时任务的核心数据类型定义。

use serde::{Deserialize, Serialize};

/// 定时任务的调度配置。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CronSchedule {
    /// 调度类型: "at" | "every" | "cron"
    pub kind: String,
    /// 一次性任务的执行时间戳（毫秒）
    #[serde(skip_serializing_if = "Option::is_none")]
    pub at_ms: Option<i64>,
    /// 间隔执行的周期（毫秒）
    #[serde(skip_serializing_if = "Option::is_none")]
    pub every_ms: Option<i64>,
    /// cron 表达式
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub expr: Option<String>,
    /// 时区
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tz: Option<String>,
}

/// 定时任务执行时的负载内容。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CronPayload {
    /// 负载类型: "agent_turn" | "command"
    pub kind: String,
    /// 消息内容
    pub message: String,
    /// 命令（可选）
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub command: Option<String>,
    /// 是否投递到渠道
    pub deliver: bool,
    /// 目标渠道
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub channel: Option<String>,
    /// 目标用户
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub to: Option<String>,
}

/// 定时任务的运行状态。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CronJobState {
    /// 下次执行时间戳（毫秒）
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_run_at_ms: Option<i64>,
    /// 上次执行时间戳（毫秒）
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_run_at_ms: Option<i64>,
    /// 上次执行状态: "ok" | "error"
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_status: Option<String>,
    /// 上次执行错误信息
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
}

/// 完整的定时任务定义。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CronJob {
    /// 任务唯一标识
    pub id: String,
    /// 任务名称
    pub name: String,
    /// 是否启用
    pub enabled: bool,
    /// 调度配置
    pub schedule: CronSchedule,
    /// 负载内容
    pub payload: CronPayload,
    /// 运行状态
    pub state: CronJobState,
    /// 创建时间戳（毫秒）
    pub created_at_ms: i64,
    /// 更新时间戳（毫秒）
    pub updated_at_ms: i64,
    /// 执行后是否自动删除
    pub delete_after_run: bool,
}

/// 持久化存储格式。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CronStoreData {
    pub version: i32,
    pub jobs: Vec<CronJob>,
}

impl Default for CronStoreData {
    fn default() -> Self {
        Self {
            version: 1,
            jobs: Vec::new(),
        }
    }
}
