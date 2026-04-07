// 定时任务的 JSON 文件持久化存储。

use std::path::PathBuf;

use chrono::Utc;
use uuid::Uuid;

use crate::error::{CronError, Result};
use crate::types::{CronJob, CronJobState, CronPayload, CronSchedule, CronStoreData};

/// JSON 文件存储后端，负责任务的 CRUD 和持久化。
#[derive(Debug)]
pub struct CronStore {
    /// 存储文件路径
    path: PathBuf,
    /// 内存中的任务数据
    data: CronStoreData,
}

impl CronStore {
    /// 创建新的存储实例并从磁盘加载数据。
    pub fn new(path: impl Into<PathBuf>) -> Result<Self> {
        let path = path.into();
        let mut store = Self {
            path,
            data: CronStoreData::default(),
        };
        store.load()?;
        Ok(store)
    }

    /// 从磁盘加载存储数据。
    pub fn load(&mut self) -> Result<()> {
        if !self.path.exists() {
            self.data = CronStoreData::default();
            return Ok(());
        }

        let content = std::fs::read_to_string(&self.path)?;
        self.data = serde_json::from_str(&content)?;
        Ok(())
    }

    /// 将存储数据原子写入磁盘（先写临时文件再重命名）。
    pub fn save(&self) -> Result<()> {
        let data = serde_json::to_string_pretty(&self.data)?;

        // 原子写入：写临时文件，然后重命名
        let tmp_path = self.path.with_extension("tmp");

        // 确保父目录存在
        if let Some(parent) = self.path.parent() {
            std::fs::create_dir_all(parent)?;
        }

        std::fs::write(&tmp_path, data.as_bytes())?;
        std::fs::rename(&tmp_path, &self.path)?;

        Ok(())
    }

    /// 生成唯一任务 ID。
    fn generate_id() -> String {
        Uuid::new_v4().to_string()[..16].to_string()
    }

    /// 当前时间戳（毫秒）。
    fn now_ms() -> i64 {
        Utc::now().timestamp_millis()
    }

    /// 添加新任务，返回创建的任务副本。
    pub fn add_job(
        &mut self,
        name: String,
        schedule: CronSchedule,
        message: String,
        deliver: bool,
        channel: Option<String>,
        to: Option<String>,
    ) -> Result<CronJob> {
        let now = Self::now_ms();
        let delete_after_run = schedule.kind == "at";
        let next_run = compute_next_run(&schedule, now);

        let job = CronJob {
            id: Self::generate_id(),
            name,
            enabled: true,
            schedule,
            payload: CronPayload {
                kind: "agent_turn".to_string(),
                message,
                command: None,
                deliver,
                channel,
                to,
            },
            state: CronJobState {
                next_run_at_ms: next_run,
                ..Default::default()
            },
            created_at_ms: now,
            updated_at_ms: now,
            delete_after_run,
        };

        self.data.jobs.push(job.clone());
        self.save()?;
        Ok(job)
    }

    /// 移除指定 ID 的任务，返回是否成功移除。
    pub fn remove_job(&mut self, job_id: &str) -> Result<bool> {
        let before = self.data.jobs.len();
        self.data.jobs.retain(|j| j.id != job_id);
        let removed = self.data.jobs.len() < before;
        if removed {
            self.save()?;
        }
        Ok(removed)
    }

    /// 启用或禁用任务，返回更新后的任务副本。
    pub fn enable_job(&mut self, job_id: &str, enabled: bool) -> Result<Option<CronJob>> {
        let now = Self::now_ms();
        let job = self.data.jobs.iter_mut().find(|j| j.id == job_id);

        let Some(job) = job else {
            return Ok(None);
        };

        job.enabled = enabled;
        job.updated_at_ms = now;

        if enabled {
            job.state.next_run_at_ms = compute_next_run(&job.schedule, now);
        } else {
            job.state.next_run_at_ms = None;
        }

        let result = job.clone();
        self.save()?;
        Ok(Some(result))
    }

    /// 获取指定 ID 的任务。
    pub fn get_job(&self, job_id: &str) -> Option<&CronJob> {
        self.data.jobs.iter().find(|j| j.id == job_id)
    }

    /// 获取指定 ID 的可变引用。
    pub fn get_job_mut(&mut self, job_id: &str) -> Option<&mut CronJob> {
        self.data.jobs.iter_mut().find(|j| j.id == job_id)
    }

    /// 列出所有任务，可选是否包含已禁用的任务。
    pub fn list_jobs(&self, include_disabled: bool) -> Vec<&CronJob> {
        if include_disabled {
            self.data.jobs.iter().collect()
        } else {
            self.data.jobs.iter().filter(|j| j.enabled).collect()
        }
    }

    /// 获取所有任务（可变引用），供内部调度使用。
    pub fn jobs_mut(&mut self) -> &mut Vec<CronJob> {
        &mut self.data.jobs
    }

    /// 获取所有任务的只读引用。
    pub fn jobs(&self) -> &[CronJob] {
        &self.data.jobs
    }

    /// 重新计算所有已启用任务的下次执行时间。
    pub fn recompute_next_runs(&mut self) {
        let now = Self::now_ms();
        for job in &mut self.data.jobs {
            if job.enabled {
                job.state.next_run_at_ms = compute_next_run(&job.schedule, now);
            }
        }
    }

    /// 更新整个任务（替换）。
    pub fn update_job(&mut self, updated: CronJob) -> Result<()> {
        let pos = self
            .data
            .jobs
            .iter()
            .position(|j| j.id == updated.id)
            .ok_or_else(|| CronError::JobNotFound(updated.id.clone()))?;

        self.data.jobs[pos] = updated;
        self.data.jobs[pos].updated_at_ms = Self::now_ms();
        self.save()?;
        Ok(())
    }

    /// 获取所有任务中最早的下次执行时间。
    pub fn next_wake_ms(&self) -> Option<i64> {
        self.data
            .jobs
            .iter()
            .filter(|j| j.enabled && j.state.next_run_at_ms.is_some())
            .filter_map(|j| j.state.next_run_at_ms)
            .min()
    }

    /// 返回任务总数。
    pub fn job_count(&self) -> usize {
        self.data.jobs.len()
    }
}

/// 根据调度配置计算下次执行时间（毫秒时间戳）。
pub fn compute_next_run(schedule: &CronSchedule, now_ms: i64) -> Option<i64> {
    match schedule.kind.as_str() {
        "at" => {
            // 一次性任务：如果目标时间在未来则返回，否则为 None
            schedule.at_ms.filter(|&at| at > now_ms)
        }
        "every" => {
            // 间隔任务：当前时间加上间隔
            schedule
                .every_ms
                .filter(|&ms| ms > 0)
                .map(|ms| now_ms + ms)
        }
        "cron" => {
            // Cron 表达式：使用 cron 库计算下次执行时间
            let expr = schedule.expr.as_deref()?;
            parse_cron_next(expr, now_ms)
        }
        _ => None,
    }
}

/// 解析 cron 表达式并计算给定时间之后的下次执行时间。
fn parse_cron_next(expr: &str, now_ms: i64) -> Option<i64> {
    use chrono::DateTime;
    use cron::Schedule;
    use std::str::FromStr;

    let schedule = Schedule::from_str(expr).ok()?;
    let now = DateTime::from_timestamp_millis(now_ms)?;
    let next = schedule.upcoming(Utc).next()?;

    // 确保下次执行时间在 now 之后
    if next <= now {
        return None;
    }

    Some(next.timestamp_millis())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::NamedTempFile;

    fn make_schedule_at(at_ms: i64) -> CronSchedule {
        CronSchedule {
            kind: "at".to_string(),
            at_ms: Some(at_ms),
            every_ms: None,
            expr: None,
            tz: None,
        }
    }

    fn make_schedule_every(every_ms: i64) -> CronSchedule {
        CronSchedule {
            kind: "every".to_string(),
            at_ms: None,
            every_ms: Some(every_ms),
            expr: None,
            tz: None,
        }
    }

    fn make_schedule_cron(expr: &str) -> CronSchedule {
        CronSchedule {
            kind: "cron".to_string(),
            at_ms: None,
            every_ms: None,
            expr: Some(expr.to_string()),
            tz: None,
        }
    }

    #[test]
    fn test_compute_next_run_at_future() {
        let now = CronStore::now_ms();
        let future = now + 60_000;
        let result = compute_next_run(&make_schedule_at(future), now);
        assert_eq!(result, Some(future));
    }

    #[test]
    fn test_compute_next_run_at_past() {
        let now = CronStore::now_ms();
        let past = now - 60_000;
        let result = compute_next_run(&make_schedule_at(past), now);
        assert!(result.is_none());
    }

    #[test]
    fn test_compute_next_run_every() {
        let now = 1_000_000i64;
        let result = compute_next_run(&make_schedule_every(5_000), now);
        assert_eq!(result, Some(1_005_000));
    }

    #[test]
    fn test_compute_next_run_every_zero() {
        let now = 1_000_000i64;
        let result = compute_next_run(&make_schedule_every(0), now);
        assert!(result.is_none());
    }

    #[test]
    fn test_compute_next_run_cron() {
        // "每分钟执行" — 7 字段格式 (sec min hour dom month dow year)
        let now = CronStore::now_ms();
        let result = compute_next_run(&make_schedule_cron("0 * * * * * *"), now);
        assert!(result.is_some());
        assert!(result.unwrap() > now);
    }

    #[test]
    fn test_compute_next_run_invalid_cron() {
        let result = compute_next_run(&make_schedule_cron("invalid expr"), 1_000_000);
        assert!(result.is_none());
    }

    #[test]
    fn test_store_crud() {
        let tmp = NamedTempFile::new().unwrap();
        let path = tmp.path().to_path_buf();
        drop(tmp); // 删除文件，让 store 自己创建

        let mut store = CronStore::new(&path).unwrap();
        assert_eq!(store.job_count(), 0);

        // 添加任务
        let job = store
            .add_job(
                "test-job".into(),
                make_schedule_every(60_000),
                "hello".into(),
                false,
                None,
                None,
            )
            .unwrap();
        assert_eq!(store.job_count(), 1);
        assert_eq!(job.name, "test-job");
        assert!(job.enabled);

        // 获取任务
        let fetched = store.get_job(&job.id).unwrap();
        assert_eq!(fetched.name, "test-job");

        // 禁用任务
        let disabled = store.enable_job(&job.id, false).unwrap().unwrap();
        assert!(!disabled.enabled);
        assert!(disabled.state.next_run_at_ms.is_none());

        // 启用任务
        let enabled = store.enable_job(&job.id, true).unwrap().unwrap();
        assert!(enabled.enabled);
        assert!(enabled.state.next_run_at_ms.is_some());

        // 列出已启用任务
        let list = store.list_jobs(false);
        assert_eq!(list.len(), 1);

        // 移除任务
        let removed = store.remove_job(&job.id).unwrap();
        assert!(removed);
        assert_eq!(store.job_count(), 0);

        // 重复移除返回 false
        let removed_again = store.remove_job(&job.id).unwrap();
        assert!(!removed_again);
    }

    #[test]
    fn test_store_persistence() {
        let tmp = NamedTempFile::new().unwrap();
        let path = tmp.path().to_path_buf();
        drop(tmp);

        // 创建 store 并添加任务
        {
            let mut store = CronStore::new(&path).unwrap();
            store
                .add_job(
                    "persistent".into(),
                    make_schedule_every(30_000),
                    "msg".into(),
                    true,
                    Some("telegram".into()),
                    None,
                )
                .unwrap();
        }

        // 重新加载验证持久化
        let store = CronStore::new(&path).unwrap();
        assert_eq!(store.job_count(), 1);
        let job = store.list_jobs(true)[0];
        assert_eq!(job.name, "persistent");
        assert_eq!(job.payload.channel.as_deref(), Some("telegram"));
    }

    #[test]
    fn test_store_load_existing_file() {
        let mut tmp = NamedTempFile::new().unwrap();
        write!(
            tmp,
            r#"{{"version":1,"jobs":[{{"id":"abc","name":"loaded","enabled":true,"schedule":{{"kind":"every","everyMs":1000}},"payload":{{"kind":"agent_turn","message":"hi","deliver":false}},"state":{{}},"createdAtMs":0,"updatedAtMs":0,"deleteAfterRun":false}}]}}"#
        )
        .unwrap();

        let store = CronStore::new(tmp.path()).unwrap();
        assert_eq!(store.job_count(), 1);
        assert_eq!(store.get_job("abc").unwrap().name, "loaded");
    }

    #[test]
    fn test_store_next_wake_ms() {
        let tmp = NamedTempFile::new().unwrap();
        let path = tmp.path().to_path_buf();
        drop(tmp);

        let mut store = CronStore::new(&path).unwrap();
        assert!(store.next_wake_ms().is_none());

        store
            .add_job(
                "j1".into(),
                make_schedule_every(60_000),
                "a".into(),
                false,
                None,
                None,
            )
            .unwrap();
        store
            .add_job(
                "j2".into(),
                make_schedule_every(30_000),
                "b".into(),
                false,
                None,
                None,
            )
            .unwrap();

        let wake = store.next_wake_ms();
        assert!(wake.is_some());
        // j2 的间隔更短，应该先唤醒
        let j2_next = store.list_jobs(true)[1].state.next_run_at_ms.unwrap();
        assert_eq!(wake.unwrap(), j2_next);
    }
}
