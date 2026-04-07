//! JSON-RPC 2.0 stdio 传输层。
//!
//! 处理通过子进程 stdin/stdout 管道进行的双向 JSON-RPC 通信。
//! 支持三种消息流：
//!   - 宿主→插件 请求/响应（call）
//!   - 插件→宿主 通知（notifications）
//!   - 插件→宿主 反向请求/响应（register_service + handle_reverse_call）

use std::collections::HashMap;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::Arc;

use serde_json::Value;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::sync::{mpsc, oneshot, Mutex, RwLock};

use crate::error::{PluginError, Result};
use crate::wire::{
    JsonRpcNotification, JsonRpcRequest, JsonRpcResponse, PeekMessage, RpcError,
};

/// 服务处理函数类型：处理来自插件的反向 JSON-RPC 调用。
/// 插件通过 host.* 方法调用宿主服务。
pub type ServiceHandler = Box<dyn Fn(Value) -> std::pin::Pin<Box<dyn std::future::Future<Output = std::result::Result<Value, String>> + Send>> + Send + Sync>;

/// JSON-RPC 2.0 传输层。
///
/// 管理与子进程的双向通信：
///   - 向 stdin 写入请求
///   - 从 stdout 读取响应/通知/反向调用
///   - 原子请求 ID 计数器
///   - 待处理请求映射表
pub struct Transport {
    /// 子进程 stdin 写入端，序列化写入需要互斥
    writer: Arc<Mutex<tokio::process::ChildStdin>>,

    /// 原子请求 ID 计数器
    next_id: AtomicI64,

    /// 待处理请求：id -> 响应发送端
    pending: Arc<RwLock<HashMap<i64, oneshot::Sender<Result<Value>>>>>,

    /// 通知接收通道
    notification_rx: Mutex<mpsc::Receiver<JsonRpcNotification>>,

    /// 通知发送通道（供读取循环使用）
    notification_tx: mpsc::Sender<JsonRpcNotification>,

    /// 反向调用服务处理函数
    services: Arc<RwLock<HashMap<String, ServiceHandler>>>,
}

impl Transport {
    /// 创建新的传输层。
    ///
    /// `stdin` - 子进程 stdin 句柄（用于写入请求）
    ///
    /// 返回 (Transport, stdout_reader) 元组，调用者需要将 stdout_reader
    /// 传递给 `read_loop` 方法启动后台读取。
    pub fn new(stdin: tokio::process::ChildStdin) -> Self {
        let (tx, rx) = mpsc::channel(64);
        Self {
            writer: Arc::new(Mutex::new(stdin)),
            next_id: AtomicI64::new(0),
            pending: Arc::new(RwLock::new(HashMap::new())),
            notification_rx: Mutex::new(rx),
            notification_tx: tx,
            services: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// 注册反向调用服务处理函数。
    /// method 应使用 "host.<service>.<action>" 格式。
    pub async fn register_service(&self, method: impl Into<String>, handler: ServiceHandler) {
        self.services.write().await.insert(method.into(), handler);
    }

    /// 发送 JSON-RPC 请求并等待响应。
    pub async fn call(&self, method: &str, params: Option<Value>) -> Result<Value> {
        let id = self.next_id.fetch_add(1, Ordering::Relaxed) + 1;

        let req = JsonRpcRequest::new(id, method, params);

        let (tx, rx) = oneshot::channel();
        self.pending.write().await.insert(id, tx);

        // 发送请求
        if let Err(e) = self.send_raw(&req).await {
            self.pending.write().await.remove(&id);
            return Err(e);
        }

        // 等待响应
        match rx.await {
            Ok(result) => result,
            Err(_) => Err(PluginError::ChannelClosed),
        }
    }

    /// 发送 JSON-RPC 通知（不期望响应）。
    pub async fn notify(&self, method: &str, params: Option<Value>) -> Result<()> {
        let notif = JsonRpcNotification {
            jsonrpc: "2.0".to_string(),
            method: method.to_string(),
            params,
        };
        self.send_raw(&notif).await
    }

    /// 获取通知接收器。
    /// 注意：同一时刻只能有一个调用者持有此锁。
    pub async fn recv_notification(&self) -> Option<JsonRpcNotification> {
        self.notification_rx.lock().await.recv().await
    }

    /// 从子进程 stdout 持续读取消息并分发。
    /// 阻塞直到 reader 关闭或发生错误。
    ///
    /// 三路分发：
    ///   - id 有值 && method 为空: 响应
    ///   - id 有值 && method 非空: 反向调用
    ///   - id 无值 && method 非空: 通知
    pub async fn read_loop(&self, stdout: tokio::process::ChildStdout) -> Result<()> {
        let reader = BufReader::new(stdout);
        let mut lines = reader.lines();

        while let Ok(Some(line)) = lines.next_line().await {
            if line.is_empty() {
                continue;
            }

            let peek: PeekMessage = match serde_json::from_str(&line) {
                Ok(p) => p,
                Err(_) => continue, // 无法解析的行直接跳过
            };

            if peek.id.is_some() && peek.method.is_empty() {
                // 响应：插件回复宿主发出的请求
                let id = peek.id.unwrap();
                if let Some(sender) = self.pending.write().await.remove(&id) {
                    let result = if let Some(err) = peek.error {
                        Err(PluginError::from(err))
                    } else {
                        Ok(peek.result.unwrap_or(Value::Null))
                    };
                    let _ = sender.send(result);
                }
            } else if peek.id.is_some() && !peek.method.is_empty() {
                // 反向调用：插件调用宿主服务
                let id = peek.id.unwrap();
                let method = peek.method;
                let params = peek.params.unwrap_or(Value::Null);
                self.handle_reverse_call(id, &method, params).await;
            } else if !peek.method.is_empty() {
                // 通知
                let notif: JsonRpcNotification = match serde_json::from_str(&line) {
                    Ok(n) => n,
                    Err(_) => continue,
                };
                // 缓冲区满时丢弃
                let _ = self.notification_tx.try_send(notif);
            }
        }

        Ok(())
    }

    /// 处理来自插件的反向 JSON-RPC 调用。
    async fn handle_reverse_call(&self, id: i64, method: &str, params: Value) {
        let services = self.services.read().await;
        let handler = services.get(method);

        let response = match handler {
            None => {
                JsonRpcResponse::error(id, RpcError::method_not_found(method))
            }
            Some(handler) => {
                match handler(params).await {
                    Ok(result) => JsonRpcResponse::success(id, result),
                    Err(msg) => JsonRpcResponse::error(id, RpcError::internal(msg)),
                }
            }
        };

        let _ = self.send_raw(&response).await;
    }

    /// 序列化并写入单行 JSON。
    async fn send_raw<T: serde::Serialize>(&self, msg: &T) -> Result<()> {
        let mut data = serde_json::to_vec(msg)?;
        data.push(b'\n');

        let mut writer = self.writer.lock().await;
        writer.write_all(&data).await?;
        writer.flush().await?;
        Ok(())
    }
}
