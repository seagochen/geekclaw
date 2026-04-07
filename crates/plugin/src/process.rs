//! 插件进程生命周期管理。
//!
//! `PluginProcess` 负责：
//!   - 启动子进程（过滤危险环境变量）
//!   - 建立 JSON-RPC 传输层
//!   - 执行初始化握手
//!   - 路由通知（日志 / 自定义）
//!   - 优雅关闭（带超时强制终止）

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use serde_json::Value;
use tokio::process::Command;
use tokio::sync::Mutex;
use tracing::{debug, warn};

use crate::config::{is_dangerous_env_var, PluginConfig};
use crate::error::{PluginError, Result};
use crate::transport::{ServiceHandler, Transport};
use crate::wire::JsonRpcNotification;

/// 插件进程启动选项。
pub struct SpawnOpts {
    /// 日志分类（例如 "commands"、"search"）
    pub log_category: String,
    /// 初始化方法名（例如 "command.initialize"）
    pub init_method: String,
    /// 初始化握手的参数
    pub init_params: Option<Value>,
    /// 优雅关闭时调用的方法名（例如 "command.stop"）
    pub stop_method: String,
    /// 日志通知方法名（例如 "command.log"）
    pub log_method: String,
    /// 非日志通知的处理回调
    pub on_notification: Option<Box<dyn Fn(JsonRpcNotification) + Send + Sync>>,
    /// 反向调用服务处理函数
    pub services: HashMap<String, ServiceHandler>,
}

/// 管理通过 stdio JSON-RPC 通信的外部插件进程。
///
/// 典型用法：
/// ```ignore
/// let proc = PluginProcess::new("myplugin", config);
/// let result = proc.spawn(opts).await?;
/// // 使用 proc.call(...) 进行 RPC 调用
/// proc.stop().await;
/// ```
pub struct PluginProcess {
    name: String,
    config: PluginConfig,
    inner: Mutex<Option<ProcessInner>>,
    stop_method: Mutex<String>,
}

/// 进程内部状态（启动后才存在）。
struct ProcessInner {
    child: tokio::process::Child,
    transport: Arc<Transport>,
    /// 后台读取任务句柄
    read_handle: tokio::task::JoinHandle<()>,
    /// 通知处理任务句柄
    notify_handle: tokio::task::JoinHandle<()>,
}

impl PluginProcess {
    /// 创建新的插件进程管理器。
    pub fn new(name: impl Into<String>, config: PluginConfig) -> Self {
        Self {
            name: name.into(),
            config,
            inner: Mutex::new(None),
            stop_method: Mutex::new(String::new()),
        }
    }

    /// 返回插件名称。
    pub fn name(&self) -> &str {
        &self.name
    }

    /// 返回插件配置。
    pub fn config(&self) -> &PluginConfig {
        &self.config
    }

    /// 启动外部进程，建立传输层，执行初始化握手。
    ///
    /// 失败时自动清理资源。返回初始化调用的原始 JSON 结果。
    pub async fn spawn(&self, opts: SpawnOpts) -> Result<Value> {
        let mut inner_guard = self.inner.lock().await;

        if self.config.command.is_empty() {
            return Err(PluginError::Config(format!(
                "插件 {:?}: command 不能为空",
                self.name
            )));
        }

        *self.stop_method.lock().await = opts.stop_method.clone();

        // 构建子进程命令
        let mut cmd = Command::new(&self.config.command);
        cmd.args(&self.config.args);
        cmd.stdin(std::process::Stdio::piped());
        cmd.stdout(std::process::Stdio::piped());
        cmd.stderr(std::process::Stdio::piped());

        if let Some(ref dir) = self.config.working_dir {
            cmd.current_dir(dir);
        }

        // 构建环境变量：继承当前环境，用配置覆盖同名变量，过滤危险变量
        cmd.env_clear();
        let mut env_map: HashMap<String, String> = std::env::vars().collect();

        // 用插件配置覆盖
        for (k, v) in &self.config.env {
            if is_dangerous_env_var(k) {
                warn!(
                    plugin = %self.name,
                    variable = %k,
                    "屏蔽危险环境变量"
                );
                continue;
            }
            env_map.insert(k.clone(), v.clone());
        }

        cmd.envs(&env_map);

        // 启动子进程
        let mut child = cmd.spawn().map_err(|e| {
            PluginError::Io(std::io::Error::new(
                e.kind(),
                format!("启动命令 {:?} 失败: {}", self.config.command, e),
            ))
        })?;

        let stdin = child.stdin.take().ok_or_else(|| {
            PluginError::Io(std::io::Error::other("无法获取子进程 stdin"))
        })?;
        let stdout = child.stdout.take().ok_or_else(|| {
            PluginError::Io(std::io::Error::other("无法获取子进程 stdout"))
        })?;

        // 转发 stderr 到日志
        let stderr = child.stderr.take();
        let plugin_name = self.name.clone();
        let log_cat = opts.log_category.clone();
        if let Some(stderr) = stderr {
            tokio::spawn(async move {
                use tokio::io::AsyncBufReadExt;
                let reader = tokio::io::BufReader::new(stderr);
                let mut lines = reader.lines();
                while let Ok(Some(line)) = lines.next_line().await {
                    debug!(
                        plugin = %plugin_name,
                        stream = "stderr",
                        category = %log_cat,
                        "{}",
                        line
                    );
                }
            });
        }

        // 创建传输层
        let transport = Arc::new(Transport::new(stdin));

        // 注册反向调用服务
        for (method, handler) in opts.services {
            transport.register_service(method, handler).await;
        }

        // 启动后台读取循环
        let transport_clone = Arc::clone(&transport);
        let plugin_name_clone = self.name.clone();
        let log_cat_clone = opts.log_category.clone();
        let read_handle = tokio::spawn(async move {
            if let Err(e) = transport_clone.read_loop(stdout).await {
                warn!(
                    plugin = %plugin_name_clone,
                    category = %log_cat_clone,
                    error = %e,
                    "插件读取循环结束"
                );
            }
        });

        // 启动通知处理
        let transport_clone = Arc::clone(&transport);
        let log_method = opts.log_method.clone();
        let plugin_name_clone = self.name.clone();
        let log_cat_clone = opts.log_category.clone();
        let on_notification = opts.on_notification;
        let notify_handle = tokio::spawn(async move {
            loop {
                match transport_clone.recv_notification().await {
                    Some(notif) => {
                        if notif.method == log_method {
                            handle_log_notification(
                                &notif,
                                &plugin_name_clone,
                                &log_cat_clone,
                            );
                        } else if let Some(ref callback) = on_notification {
                            callback(notif);
                        }
                    }
                    None => break, // 通道关闭
                }
            }
        });

        *inner_guard = Some(ProcessInner {
            child,
            transport: Arc::clone(&transport),
            read_handle,
            notify_handle,
        });

        // 初始化握手
        let result = transport.call(&opts.init_method, opts.init_params).await;
        match result {
            Ok(value) => Ok(value),
            Err(e) => {
                // 握手失败，清理资源
                self.stop_inner(&mut inner_guard).await;
                Err(PluginError::Config(format!(
                    "初始化握手失败: {}",
                    e
                )))
            }
        }
    }

    /// 发送 JSON-RPC 调用。
    pub async fn call(&self, method: &str, params: Option<Value>) -> Result<Value> {
        let inner = self.inner.lock().await;
        match inner.as_ref() {
            Some(inner) => inner.transport.call(method, params).await,
            None => Err(PluginError::ProcessExited),
        }
    }

    /// 检查插件进程是否仍然存活。
    pub async fn is_alive(&self) -> bool {
        let mut inner = self.inner.lock().await;
        match inner.as_mut() {
            Some(pi) => pi.child.try_wait().ok().flatten().is_none(),
            None => false,
        }
    }

    /// 优雅关闭插件进程。
    ///
    /// 先发送 stop RPC 调用（5 秒超时），然后终止进程。
    pub async fn stop(&self) {
        let mut inner = self.inner.lock().await;
        self.stop_inner(&mut inner).await;
    }

    /// 在持有锁的情况下停止进程。
    async fn stop_inner(
        &self,
        inner: &mut Option<ProcessInner>,
    ) {
        if let Some(mut pi) = inner.take() {
            // 尝试优雅关闭：发送 stop 方法
            let stop_method = self.stop_method.lock().await.clone();
            if !stop_method.is_empty() {
                let transport = Arc::clone(&pi.transport);
                let _ = tokio::time::timeout(
                    Duration::from_secs(5),
                    transport.call(&stop_method, None),
                )
                .await;
            }

            // 终止子进程
            let _ = pi.child.kill().await;
            let _ = pi.child.wait().await;

            // 等待后台任务结束
            pi.read_handle.abort();
            pi.notify_handle.abort();
            let _ = pi.read_handle.await;
            let _ = pi.notify_handle.await;
        }
    }
}

/// 将插件日志通知转发到 tracing 日志。
/// 通知参数必须包含 "level" 和 "message" 字符串字段。
fn handle_log_notification(notif: &JsonRpcNotification, plugin_name: &str, category: &str) {
    let params = match &notif.params {
        Some(Value::Object(map)) => map,
        _ => return,
    };

    let msg = params
        .get("message")
        .and_then(|v| v.as_str())
        .unwrap_or("");
    let level = params
        .get("level")
        .and_then(|v| v.as_str())
        .unwrap_or("info");

    match level {
        "debug" => debug!(plugin = %plugin_name, category = %category, "{}", msg),
        "warn" => warn!(plugin = %plugin_name, category = %category, "{}", msg),
        "error" => tracing::error!(plugin = %plugin_name, category = %category, "{}", msg),
        _ => tracing::info!(plugin = %plugin_name, category = %category, "{}", msg),
    }
}

/// 构建环境变量映射，过滤危险变量并合并自定义变量。
/// 返回过滤后的环境变量及被屏蔽的变量名列表。
pub fn build_env(
    custom_env: &HashMap<String, String>,
) -> (HashMap<String, String>, Vec<String>) {
    let mut env_map: HashMap<String, String> = std::env::vars().collect();
    let mut blocked = Vec::new();

    for (k, v) in custom_env {
        if is_dangerous_env_var(k) {
            blocked.push(k.clone());
            continue;
        }
        env_map.insert(k.clone(), v.clone());
    }

    (env_map, blocked)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_env_filters_dangerous() {
        let mut custom = HashMap::new();
        custom.insert("LD_PRELOAD".to_string(), "/evil/lib.so".to_string());
        custom.insert("DYLD_INSERT_LIBRARIES".to_string(), "/evil/dylib".to_string());
        custom.insert("MY_VAR".to_string(), "safe_value".to_string());
        custom.insert("PATH".to_string(), "/custom/path".to_string());

        let (env, blocked) = build_env(&custom);

        // 危险变量应被屏蔽
        assert!(blocked.contains(&"LD_PRELOAD".to_string()));
        assert!(blocked.contains(&"DYLD_INSERT_LIBRARIES".to_string()));

        // 安全变量应保留
        assert_eq!(env.get("MY_VAR").unwrap(), "safe_value");
        assert_eq!(env.get("PATH").unwrap(), "/custom/path");

        // 危险变量不应出现在最终环境中（来自自定义的）
        // 注意：系统本身可能有 LD_PRELOAD 或 LD_LIBRARY_PATH，
        // 但我们只屏蔽用户自定义的值
        assert_eq!(blocked.len(), 2);
    }

    #[test]
    fn test_build_env_case_insensitive() {
        let mut custom = HashMap::new();
        custom.insert("ld_preload".to_string(), "/evil".to_string());
        custom.insert("Dyld_Library_Path".to_string(), "/evil".to_string());

        let (_, blocked) = build_env(&custom);
        assert_eq!(blocked.len(), 2);
    }
}
