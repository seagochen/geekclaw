//! 消息总线的核心消息类型。

use serde::{Deserialize, Serialize};

/// 发送者身份信息。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SenderInfo {
    /// 平台标识（如 "telegram"、"discord"）。
    #[serde(default)]
    pub platform: String,
    /// 平台原始 ID。
    #[serde(default)]
    pub platform_id: String,
    /// 规范化 ID（"platform:id" 格式）。
    #[serde(default)]
    pub canonical_id: String,
    /// 用户名。
    #[serde(default)]
    pub username: String,
    /// 显示名称。
    #[serde(default)]
    pub display_name: String,
}

/// 入站消息（从外部渠道进入 Agent）。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct InboundMessage {
    /// 渠道名称。
    pub channel: String,
    /// 发送者 ID。
    #[serde(default)]
    pub sender_id: String,
    /// 结构化发送者信息。
    #[serde(default)]
    pub sender: SenderInfo,
    /// 聊天/会话 ID。
    pub chat_id: String,
    /// 消息内容。
    pub content: String,
    /// 媒体引用列表。
    #[serde(default)]
    pub media: Vec<String>,
    /// 会话标识符。
    #[serde(default)]
    pub session_key: String,
    /// 平台消息 ID。
    #[serde(default)]
    pub message_id: String,
    /// 额外元数据。
    #[serde(default)]
    pub metadata: std::collections::HashMap<String, String>,
}

/// 出站消息（从 Agent 发送到外部渠道）。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct OutboundMessage {
    /// 目标渠道。
    pub channel: String,
    /// 目标聊天 ID。
    pub chat_id: String,
    /// 消息内容。
    pub content: String,
    /// 回复的消息 ID。
    #[serde(default)]
    pub reply_to_message_id: Option<String>,
}
