//! JSON-RPC 2.0 线路类型定义。
//!
//! 支持三种消息类型：请求（Request）、响应（Response）、通知（Notification）。

use serde::{Deserialize, Serialize};
use serde_json::Value;

/// JSON-RPC 2.0 请求，由宿主发送给插件。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JsonRpcRequest {
    pub jsonrpc: String,
    pub id: i64,
    pub method: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub params: Option<Value>,
}

impl JsonRpcRequest {
    /// 创建一个新的 JSON-RPC 2.0 请求。
    pub fn new(id: i64, method: impl Into<String>, params: Option<Value>) -> Self {
        Self {
            jsonrpc: "2.0".to_string(),
            id,
            method: method.into(),
            params,
        }
    }
}

/// JSON-RPC 2.0 响应，由插件返回给宿主。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JsonRpcResponse {
    pub jsonrpc: String,
    pub id: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub result: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<RpcError>,
}

impl JsonRpcResponse {
    /// 创建一个成功响应。
    pub fn success(id: i64, result: Value) -> Self {
        Self {
            jsonrpc: "2.0".to_string(),
            id,
            result: Some(result),
            error: None,
        }
    }

    /// 创建一个错误响应。
    pub fn error(id: i64, error: RpcError) -> Self {
        Self {
            jsonrpc: "2.0".to_string(),
            id,
            result: None,
            error: Some(error),
        }
    }
}

/// JSON-RPC 2.0 通知（无 ID），由插件发送给宿主的单向消息。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JsonRpcNotification {
    pub jsonrpc: String,
    pub method: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub params: Option<Value>,
}

/// JSON-RPC 2.0 错误对象。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RpcError {
    pub code: i32,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<Value>,
}

impl std::fmt::Display for RpcError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "RPC 错误 (code={}): {}", self.code, self.message)
    }
}

impl std::error::Error for RpcError {}

impl RpcError {
    /// 方法未找到错误（-32601）。
    pub fn method_not_found(method: &str) -> Self {
        Self {
            code: -32601,
            message: format!("method not found: {}", method),
            data: None,
        }
    }

    /// 服务端内部错误（-32000）。
    pub fn internal(message: impl Into<String>) -> Self {
        Self {
            code: -32000,
            message: message.into(),
            data: None,
        }
    }
}

/// 用于从 stdout 读取消息时的"窥探"结构体。
/// 根据字段存在与否判断消息类型：
///   - id 有值 && method 为空: 响应
///   - id 有值 && method 非空: 反向调用
///   - id 无值 && method 非空: 通知
#[derive(Debug, Deserialize)]
pub(crate) struct PeekMessage {
    pub id: Option<i64>,
    #[serde(default)]
    pub method: String,
    pub result: Option<Value>,
    pub error: Option<RpcError>,
    pub params: Option<Value>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_request_serialization() {
        let req = JsonRpcRequest::new(1, "test.method", Some(json!({"key": "value"})));
        let json_str = serde_json::to_string(&req).unwrap();
        let parsed: JsonRpcRequest = serde_json::from_str(&json_str).unwrap();
        assert_eq!(parsed.jsonrpc, "2.0");
        assert_eq!(parsed.id, 1);
        assert_eq!(parsed.method, "test.method");
        assert_eq!(parsed.params, Some(json!({"key": "value"})));
    }

    #[test]
    fn test_request_no_params() {
        let req = JsonRpcRequest::new(2, "test.nop", None);
        let json_str = serde_json::to_string(&req).unwrap();
        // params 为 None 时不应序列化
        assert!(!json_str.contains("params"));
    }

    #[test]
    fn test_response_success() {
        let resp = JsonRpcResponse::success(1, json!({"status": "ok"}));
        let json_str = serde_json::to_string(&resp).unwrap();
        let parsed: JsonRpcResponse = serde_json::from_str(&json_str).unwrap();
        assert_eq!(parsed.id, 1);
        assert!(parsed.result.is_some());
        assert!(parsed.error.is_none());
    }

    #[test]
    fn test_response_error() {
        let resp = JsonRpcResponse::error(1, RpcError::internal("something failed"));
        let json_str = serde_json::to_string(&resp).unwrap();
        let parsed: JsonRpcResponse = serde_json::from_str(&json_str).unwrap();
        assert_eq!(parsed.id, 1);
        assert!(parsed.result.is_none());
        let err = parsed.error.unwrap();
        assert_eq!(err.code, -32000);
        assert_eq!(err.message, "something failed");
    }

    #[test]
    fn test_notification_deserialization() {
        let json_str = r#"{"jsonrpc":"2.0","method":"log","params":{"level":"info","message":"hello"}}"#;
        let notif: JsonRpcNotification = serde_json::from_str(json_str).unwrap();
        assert_eq!(notif.method, "log");
        assert!(notif.params.is_some());
    }

    #[test]
    fn test_peek_message_response() {
        let json_str = r#"{"jsonrpc":"2.0","id":1,"result":{"ok":true}}"#;
        let peek: PeekMessage = serde_json::from_str(json_str).unwrap();
        assert_eq!(peek.id, Some(1));
        assert!(peek.method.is_empty());
        assert!(peek.result.is_some());
    }

    #[test]
    fn test_peek_message_notification() {
        let json_str = r#"{"jsonrpc":"2.0","method":"notify","params":{}}"#;
        let peek: PeekMessage = serde_json::from_str(json_str).unwrap();
        assert!(peek.id.is_none());
        assert_eq!(peek.method, "notify");
    }

    #[test]
    fn test_peek_message_reverse_call() {
        let json_str = r#"{"jsonrpc":"2.0","id":5,"method":"host.bus.publish","params":{}}"#;
        let peek: PeekMessage = serde_json::from_str(json_str).unwrap();
        assert_eq!(peek.id, Some(5));
        assert_eq!(peek.method, "host.bus.publish");
    }
}
