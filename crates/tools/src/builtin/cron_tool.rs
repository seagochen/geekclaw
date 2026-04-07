//! 定时任务管理工具。
//!
//! 允许 LLM 通过 cron 模块管理定时任务（列出/添加/删除/启用/禁用）。

use async_trait::async_trait;
use geekclaw_cron::{CronSchedule, CronStore};
use serde_json::{json, Value};

use crate::{Tool, ToolContext, ToolResult};

/// 定时任务管理工具。
pub struct CronTool;

#[async_trait]
impl Tool for CronTool {
    fn name(&self) -> &str {
        "cron"
    }

    fn description(&self) -> &str {
        "管理定时任务。支持的操作：list（列出所有任务）、add（添加任务）、\
         remove（删除任务）、enable（启用任务）、disable（禁用任务）。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "action": {
                    "type": "string",
                    "enum": ["list", "add", "remove", "enable", "disable"],
                    "description": "操作类型"
                },
                "id": {
                    "type": "string",
                    "description": "任务 ID（remove/enable/disable 时需要）"
                },
                "name": {
                    "type": "string",
                    "description": "任务名称（add 时需要）"
                },
                "schedule_kind": {
                    "type": "string",
                    "enum": ["cron", "every", "at"],
                    "description": "调度类型：cron 表达式 / every 间隔 / at 一次性"
                },
                "schedule_expr": {
                    "type": "string",
                    "description": "调度表达式：cron 表达式 / 间隔毫秒数 / 时间戳毫秒"
                },
                "message": {
                    "type": "string",
                    "description": "任务触发时发送的消息"
                }
            },
            "required": ["action"]
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> ToolResult {
        let action = match args["action"].as_str() {
            Some(a) => a,
            None => return ToolResult::err("缺少 action 参数"),
        };

        let store_path = ctx
            .metadata
            .get("cron_store_path")
            .cloned()
            .unwrap_or_else(|| ".geekclaw/cron.json".into());

        let mut store = match CronStore::new(&store_path) {
            Ok(s) => s,
            Err(e) => return ToolResult::err(format!("打开 cron 存储失败: {e}")),
        };

        match action {
            "list" => list_jobs(&store),
            "add" => add_job(&mut store, &args),
            "remove" => remove_job(&mut store, &args),
            "enable" => toggle_job(&mut store, &args, true),
            "disable" => toggle_job(&mut store, &args, false),
            _ => ToolResult::err(format!("未知操作: {action}")),
        }
    }
}

fn list_jobs(store: &CronStore) -> ToolResult {
    let jobs = store.list_jobs(true);
    if jobs.is_empty() {
        return ToolResult::ok("当前没有定时任务。");
    }

    let mut lines = vec![format!("共 {} 个定时任务：\n", jobs.len())];
    for job in &jobs {
        let status = if job.enabled { "启用" } else { "禁用" };
        let schedule = match job.schedule.kind.as_str() {
            "cron" => format!("cron: {}", job.schedule.expr.as_deref().unwrap_or("?")),
            "every" => format!("每 {}s", job.schedule.every_ms.unwrap_or(0) / 1000),
            "at" => format!("一次性 @{}", job.schedule.at_ms.unwrap_or(0)),
            other => other.to_string(),
        };
        lines.push(format!(
            "  [{status}] {id}  {name}  ({schedule})  \"{msg}\"",
            id = &job.id,
            name = job.name,
            msg = job.payload.message,
        ));
    }
    ToolResult::ok(lines.join("\n"))
}

fn add_job(store: &mut CronStore, args: &Value) -> ToolResult {
    let name = match args["name"].as_str() {
        Some(n) => n,
        None => return ToolResult::err("add 需要 name 参数"),
    };
    let kind = match args["schedule_kind"].as_str() {
        Some(k) => k,
        None => return ToolResult::err("add 需要 schedule_kind 参数"),
    };
    let expr = match args["schedule_expr"].as_str() {
        Some(e) => e,
        None => return ToolResult::err("add 需要 schedule_expr 参数"),
    };
    let message = args["message"].as_str().unwrap_or("").to_string();

    let schedule = match kind {
        "cron" => CronSchedule {
            kind: "cron".into(),
            at_ms: None,
            every_ms: None,
            expr: Some(expr.into()),
            tz: None,
        },
        "every" => {
            let ms: i64 = expr.parse().unwrap_or(60000);
            CronSchedule {
                kind: "every".into(),
                at_ms: None,
                every_ms: Some(ms),
                expr: None,
                tz: None,
            }
        }
        "at" => {
            let ms: i64 = expr.parse().unwrap_or(0);
            CronSchedule {
                kind: "at".into(),
                at_ms: Some(ms),
                every_ms: None,
                expr: None,
                tz: None,
            }
        }
        _ => return ToolResult::err(format!("未知调度类型: {kind}")),
    };

    match store.add_job(name.into(), schedule, message, true, None, None) {
        Ok(job) => ToolResult::ok(format!("已添加定时任务: {} (ID: {})", job.name, job.id)),
        Err(e) => ToolResult::err(format!("添加任务失败: {e}")),
    }
}

fn remove_job(store: &mut CronStore, args: &Value) -> ToolResult {
    let id = match args["id"].as_str() {
        Some(i) => i,
        None => return ToolResult::err("remove 需要 id 参数"),
    };
    match store.remove_job(id) {
        Ok(true) => ToolResult::ok(format!("已删除任务 {id}")),
        Ok(false) => ToolResult::err(format!("未找到任务 {id}")),
        Err(e) => ToolResult::err(format!("删除失败: {e}")),
    }
}

fn toggle_job(store: &mut CronStore, args: &Value, enable: bool) -> ToolResult {
    let id = match args["id"].as_str() {
        Some(i) => i,
        None => {
            let action = if enable { "enable" } else { "disable" };
            return ToolResult::err(format!("{action} 需要 id 参数"));
        }
    };
    match store.enable_job(id, enable) {
        Ok(Some(job)) => {
            let action = if enable { "启用" } else { "禁用" };
            ToolResult::ok(format!("已{action}任务 {} ({})", job.name, job.id))
        }
        Ok(None) => ToolResult::err(format!("未找到任务 {id}")),
        Err(e) => ToolResult::err(format!("操作失败: {e}")),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn test_ctx() -> ToolContext {
        let tmp = tempfile::tempdir().unwrap();
        let path = tmp.path().join("cron.json").to_str().unwrap().to_string();
        std::mem::forget(tmp);
        let mut metadata = HashMap::new();
        metadata.insert("cron_store_path".into(), path);
        ToolContext {
            session_key: "test".into(),
            channel: "test".into(),
            chat_id: "1".into(),
            working_dir: "/tmp".into(),
            metadata,
        }
    }

    #[tokio::test]
    async fn test_list_empty() {
        let tool = CronTool;
        let result = tool.execute(json!({"action": "list"}), &test_ctx()).await;
        assert!(result.success);
        assert!(result.content.contains("没有"));
    }

    #[tokio::test]
    async fn test_add_and_list() {
        let tool = CronTool;
        let ctx = test_ctx();

        let result = tool
            .execute(
                json!({
                    "action": "add",
                    "name": "每日提醒",
                    "schedule_kind": "every",
                    "schedule_expr": "60000",
                    "message": "该起床了"
                }),
                &ctx,
            )
            .await;
        assert!(result.success, "add failed: {}", result.content);
        assert!(result.content.contains("已添加"));

        let result = tool.execute(json!({"action": "list"}), &ctx).await;
        assert!(result.success);
        assert!(result.content.contains("每日提醒"));
    }

    #[tokio::test]
    async fn test_missing_action() {
        let tool = CronTool;
        let result = tool.execute(json!({}), &test_ctx()).await;
        assert!(!result.success);
    }

    #[tokio::test]
    async fn test_add_missing_name() {
        let tool = CronTool;
        let result = tool
            .execute(json!({"action": "add"}), &test_ctx())
            .await;
        assert!(!result.success);
        assert!(result.content.contains("name"));
    }
}
