// geekclaw-cron: 定时任务调度模块。
//
// 提供基于 JSON 文件持久化的定时任务管理，支持三种调度方式：
// - "at": 一次性定时执行
// - "every": 固定间隔重复执行
// - "cron": 基于 cron 表达式调度

pub mod error;
pub mod service;
pub mod store;
pub mod types;

// 重新导出常用类型
pub use error::CronError;
pub use service::{CronService, JobCallback, ServiceStatus};
pub use store::CronStore;
pub use types::{CronJob, CronJobState, CronPayload, CronSchedule};
