// 定时任务调度服务，负责周期性检查到期任务并执行回调。

use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;
use std::time::Duration;

use tokio::sync::{Mutex, Notify};
use tracing::{error, info};

use crate::error::Result;
use crate::store::{compute_next_run, CronStore};
use crate::types::CronJob;

/// 任务执行回调类型。
pub type JobCallback =
    Arc<dyn Fn(CronJob) -> Pin<Box<dyn Future<Output = ()> + Send>> + Send + Sync>;

/// 定时任务调度服务。
///
/// 通过 tokio 任务运行一个 tick 循环，定期检查到期任务并调用回调函数。
/// 使用 `CancellationToken` 实现优雅关闭。
pub struct CronService {
    store: Arc<Mutex<CronStore>>,
    callback: Arc<Mutex<Option<JobCallback>>>,
    /// 用于通知调度循环重新计算唤醒时间
    wake_notify: Arc<Notify>,
    /// 用于通知调度循环关闭
    shutdown: tokio::sync::watch::Sender<bool>,
    shutdown_rx: Mutex<Option<tokio::sync::watch::Receiver<bool>>>,
}

impl CronService {
    /// 创建新的调度服务实例。
    pub fn new(store_path: impl Into<std::path::PathBuf>) -> Result<Self> {
        let store = CronStore::new(store_path)?;
        let (shutdown_tx, shutdown_rx) = tokio::sync::watch::channel(false);

        Ok(Self {
            store: Arc::new(Mutex::new(store)),
            callback: Arc::new(Mutex::new(None)),
            wake_notify: Arc::new(Notify::new()),
            shutdown: shutdown_tx,
            shutdown_rx: Mutex::new(Some(shutdown_rx)),
        })
    }

    /// 设置任务执行回调。
    pub async fn set_callback(&self, cb: JobCallback) {
        let mut guard = self.callback.lock().await;
        *guard = Some(cb);
    }

    /// 获取底层存储的引用，用于外部 CRUD 操作。
    pub fn store(&self) -> &Arc<Mutex<CronStore>> {
        &self.store
    }

    /// 通知调度循环重新计算唤醒时间（非阻塞）。
    pub fn notify_wake(&self) {
        self.wake_notify.notify_one();
    }

    /// 启动调度服务，返回 JoinHandle。
    ///
    /// 在 tokio 任务中运行 tick 循环，根据下一个任务的执行时间动态计算唤醒间隔。
    /// 最长休眠 60 秒（保底轮询）。
    pub async fn run(&self) -> Result<tokio::task::JoinHandle<()>> {
        // 初始化：重新计算所有任务的下次执行时间
        {
            let mut store = self.store.lock().await;
            store.recompute_next_runs();
            store.save()?;
        }

        let mut shutdown_rx = {
            let mut guard = self.shutdown_rx.lock().await;
            guard.take().expect("run() 只能调用一次")
        };

        let store = Arc::clone(&self.store);
        let callback = Arc::clone(&self.callback);
        let wake_notify = Arc::clone(&self.wake_notify);

        let handle = tokio::spawn(async move {
            const MAX_SLEEP: Duration = Duration::from_secs(60);

            loop {
                // 检查到期任务
                let due_jobs = {
                    let mut store = store.lock().await;
                    collect_and_clear_due_jobs(&mut store)
                };

                // 在锁外执行回调
                if !due_jobs.is_empty() {
                    let cb_guard = callback.lock().await;
                    if let Some(ref cb) = *cb_guard {
                        for job in &due_jobs {
                            info!(
                                "[cron] 执行任务 '{}' (id: {}, schedule: {})",
                                job.name, job.id, job.schedule.kind
                            );
                            cb(job.clone()).await;
                        }
                    }
                    drop(cb_guard);

                    // 更新已执行任务的状态
                    let mut store = store.lock().await;
                    update_after_execution(&mut store, &due_jobs);
                    if let Err(e) = store.save() {
                        error!("[cron] 持久化失败: {}", e);
                    }
                }

                // 计算下次唤醒时间
                let sleep_dur = {
                    let store = store.lock().await;
                    match store.next_wake_ms() {
                        Some(next_ms) => {
                            let now_ms = chrono::Utc::now().timestamp_millis();
                            let until = Duration::from_millis(
                                (next_ms - now_ms).max(100) as u64,
                            );
                            until.min(MAX_SLEEP)
                        }
                        None => MAX_SLEEP,
                    }
                };

                // 等待：shutdown / wake 通知 / 超时
                tokio::select! {
                    _ = shutdown_rx.changed() => {
                        info!("[cron] 调度服务关闭");
                        return;
                    }
                    _ = wake_notify.notified() => {
                        // 有新任务或任务变更，立即重新检查
                        continue;
                    }
                    _ = tokio::time::sleep(sleep_dur) => {
                        // 定时唤醒
                    }
                }
            }
        });

        Ok(handle)
    }

    /// 停止调度服务。
    pub fn stop(&self) {
        let _ = self.shutdown.send(true);
    }

    /// 返回服务状态信息。
    pub async fn status(&self) -> ServiceStatus {
        let store = self.store.lock().await;
        let total = store.job_count();
        let enabled = store.list_jobs(false).len();
        let next_wake = store.next_wake_ms();
        ServiceStatus {
            total_jobs: total,
            enabled_jobs: enabled,
            next_wake_at_ms: next_wake,
        }
    }
}

/// 服务状态信息。
#[derive(Debug, Clone)]
pub struct ServiceStatus {
    pub total_jobs: usize,
    pub enabled_jobs: usize,
    pub next_wake_at_ms: Option<i64>,
}

/// 收集到期任务并清除其 next_run_at_ms（避免重复执行）。
fn collect_and_clear_due_jobs(store: &mut CronStore) -> Vec<CronJob> {
    let now_ms = chrono::Utc::now().timestamp_millis();
    let mut due = Vec::new();

    for job in store.jobs_mut() {
        if job.enabled {
            if let Some(next) = job.state.next_run_at_ms {
                if next <= now_ms {
                    due.push(job.clone());
                    job.state.next_run_at_ms = None;
                }
            }
        }
    }

    due
}

/// 执行完成后更新任务状态。
fn update_after_execution(store: &mut CronStore, executed_jobs: &[CronJob]) {
    let now_ms = chrono::Utc::now().timestamp_millis();

    for executed in executed_jobs {
        // 一次性任务：执行后删除或禁用
        if executed.schedule.kind == "at" {
            if executed.delete_after_run {
                let _ = store.remove_job(&executed.id);
                continue;
            }
            // 不删除则禁用
            if let Some(job) = store.get_job_mut(&executed.id) {
                job.enabled = false;
                job.state.next_run_at_ms = None;
                job.state.last_run_at_ms = Some(now_ms);
                job.state.last_status = Some("ok".to_string());
                job.state.last_error = None;
                job.updated_at_ms = now_ms;
            }
            continue;
        }

        // 周期任务：计算下次执行时间
        if let Some(job) = store.get_job_mut(&executed.id) {
            job.state.last_run_at_ms = Some(now_ms);
            job.state.last_status = Some("ok".to_string());
            job.state.last_error = None;
            job.state.next_run_at_ms = compute_next_run(&job.schedule, now_ms);
            job.updated_at_ms = now_ms;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::CronSchedule;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tempfile::NamedTempFile;

    fn make_schedule_every(every_ms: i64) -> CronSchedule {
        CronSchedule {
            kind: "every".to_string(),
            at_ms: None,
            every_ms: Some(every_ms),
            expr: None,
            tz: None,
        }
    }

    fn make_schedule_at(at_ms: i64) -> CronSchedule {
        CronSchedule {
            kind: "at".to_string(),
            at_ms: Some(at_ms),
            every_ms: None,
            expr: None,
            tz: None,
        }
    }

    #[tokio::test]
    async fn test_service_lifecycle() {
        let tmp = NamedTempFile::new().unwrap();
        let path = tmp.path().to_path_buf();
        drop(tmp);

        let service = CronService::new(&path).unwrap();
        let handle = service.run().await.unwrap();

        let status = service.status().await;
        assert_eq!(status.total_jobs, 0);

        service.stop();
        handle.await.unwrap();
    }

    #[tokio::test]
    async fn test_service_fires_callback() {
        let tmp = NamedTempFile::new().unwrap();
        let path = tmp.path().to_path_buf();
        drop(tmp);

        let service = CronService::new(&path).unwrap();

        // 添加一个即将到期的间隔任务
        {
            let mut store = service.store().lock().await;
            store
                .add_job(
                    "fire-test".into(),
                    make_schedule_every(100), // 100ms 间隔
                    "test msg".into(),
                    false,
                    None,
                    None,
                )
                .unwrap();
        }

        let counter = Arc::new(AtomicUsize::new(0));
        let counter_clone = Arc::clone(&counter);

        service
            .set_callback(Arc::new(move |_job| {
                let c = Arc::clone(&counter_clone);
                Box::pin(async move {
                    c.fetch_add(1, Ordering::SeqCst);
                })
            }))
            .await;

        let handle = service.run().await.unwrap();

        // 等待足够时间让 tick 触发
        tokio::time::sleep(Duration::from_millis(500)).await;

        service.stop();
        handle.await.unwrap();

        // 回调应该至少被调用了一次
        assert!(counter.load(Ordering::SeqCst) >= 1);
    }

    #[tokio::test]
    async fn test_at_job_deleted_after_run() {
        let tmp = NamedTempFile::new().unwrap();
        let path = tmp.path().to_path_buf();
        drop(tmp);

        let service = CronService::new(&path).unwrap();

        // 添加一个即将到期的一次性任务
        let now = chrono::Utc::now().timestamp_millis();
        let job_id;
        {
            let mut store = service.store().lock().await;
            let job = store
                .add_job(
                    "once-job".into(),
                    make_schedule_at(now + 50), // 50ms 后执行
                    "once msg".into(),
                    false,
                    None,
                    None,
                )
                .unwrap();
            job_id = job.id.clone();
        }

        let fired = Arc::new(AtomicUsize::new(0));
        let fired_clone = Arc::clone(&fired);

        service
            .set_callback(Arc::new(move |_| {
                let f = Arc::clone(&fired_clone);
                Box::pin(async move {
                    f.fetch_add(1, Ordering::SeqCst);
                })
            }))
            .await;

        let handle = service.run().await.unwrap();
        tokio::time::sleep(Duration::from_millis(300)).await;

        // 验证任务已被删除
        {
            let store = service.store().lock().await;
            assert!(store.get_job(&job_id).is_none(), "一次性任务执行后应被删除");
        }

        service.stop();
        handle.await.unwrap();

        assert!(fired.load(Ordering::SeqCst) >= 1);
    }

    #[tokio::test]
    async fn test_collect_due_jobs() {
        let tmp = NamedTempFile::new().unwrap();
        let path = tmp.path().to_path_buf();
        drop(tmp);

        let mut store = CronStore::new(&path).unwrap();
        let now = chrono::Utc::now().timestamp_millis();

        // 添加一个已过期的任务
        store
            .add_job(
                "due".into(),
                make_schedule_every(1), // 1ms 间隔，马上到期
                "msg".into(),
                false,
                None,
                None,
            )
            .unwrap();

        // 添加一个未到期的任务
        store
            .add_job(
                "not-due".into(),
                make_schedule_at(now + 999_999),
                "msg".into(),
                false,
                None,
                None,
            )
            .unwrap();

        // 等一下确保第一个任务到期
        tokio::time::sleep(Duration::from_millis(10)).await;

        let due = collect_and_clear_due_jobs(&mut store);
        assert_eq!(due.len(), 1);
        assert_eq!(due[0].name, "due");
    }
}
