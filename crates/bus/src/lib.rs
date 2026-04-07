//! GeekClaw 进程内消息总线。
//!
//! 基于 `tokio::sync::mpsc`，提供入站/出站消息路由。

mod types;
pub use types::*;

use std::sync::atomic::{AtomicBool, Ordering};
use tokio::sync::mpsc;
use tracing::warn;

/// 消息总线通道默认缓冲区大小。
const DEFAULT_BUFFER_SIZE: usize = 64;

/// 消息总线错误。
#[derive(Debug, thiserror::Error)]
pub enum BusError {
    #[error("消息总线已关闭")]
    Closed,
}

/// 核心消息总线，在入站和出站通道之间路由消息。
pub struct MessageBus {
    inbound_tx: mpsc::Sender<InboundMessage>,
    inbound_rx: mpsc::Receiver<InboundMessage>,
    outbound_tx: mpsc::Sender<OutboundMessage>,
    outbound_rx: mpsc::Receiver<OutboundMessage>,
    closed: AtomicBool,
}

impl MessageBus {
    /// 创建新的消息总线。
    pub fn new() -> Self {
        let (inbound_tx, inbound_rx) = mpsc::channel(DEFAULT_BUFFER_SIZE);
        let (outbound_tx, outbound_rx) = mpsc::channel(DEFAULT_BUFFER_SIZE);
        Self {
            inbound_tx,
            inbound_rx,
            outbound_tx,
            outbound_rx,
            closed: AtomicBool::new(false),
        }
    }

    /// 发布入站消息。
    pub async fn publish_inbound(&self, msg: InboundMessage) -> Result<(), BusError> {
        if self.closed.load(Ordering::Acquire) {
            return Err(BusError::Closed);
        }
        self.inbound_tx.send(msg).await.map_err(|_| {
            warn!("发布入站消息失败：接收端已关闭");
            BusError::Closed
        })
    }

    /// 消费一条入站消息。
    pub async fn consume_inbound(&mut self) -> Option<InboundMessage> {
        self.inbound_rx.recv().await
    }

    /// 发布出站消息。
    pub async fn publish_outbound(&self, msg: OutboundMessage) -> Result<(), BusError> {
        if self.closed.load(Ordering::Acquire) {
            return Err(BusError::Closed);
        }
        self.outbound_tx.send(msg).await.map_err(|_| {
            warn!("发布出站消息失败：接收端已关闭");
            BusError::Closed
        })
    }

    /// 消费一条出站消息。
    pub async fn consume_outbound(&mut self) -> Option<OutboundMessage> {
        self.outbound_rx.recv().await
    }

    /// 获取入站发送端的克隆（用于外部模块发布消息）。
    pub fn inbound_sender(&self) -> mpsc::Sender<InboundMessage> {
        self.inbound_tx.clone()
    }

    /// 获取出站发送端的克隆（用于 agent 发送响应）。
    pub fn outbound_sender(&self) -> mpsc::Sender<OutboundMessage> {
        self.outbound_tx.clone()
    }

    /// 关闭消息总线。
    pub fn close(&self) {
        self.closed.store(true, Ordering::Release);
    }

    /// 检查总线是否已关闭。
    pub fn is_closed(&self) -> bool {
        self.closed.load(Ordering::Acquire)
    }
}

impl Default for MessageBus {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_inbound_roundtrip() {
        let mut bus = MessageBus::new();
        let msg = InboundMessage {
            channel: "test".into(),
            sender_id: "user1".into(),
            chat_id: "chat1".into(),
            content: "hello".into(),
            session_key: "test:user1".into(),
            ..Default::default()
        };

        bus.publish_inbound(msg.clone()).await.unwrap();
        let received = bus.consume_inbound().await.unwrap();
        assert_eq!(received.content, "hello");
        assert_eq!(received.channel, "test");
    }

    #[tokio::test]
    async fn test_outbound_roundtrip() {
        let mut bus = MessageBus::new();
        let msg = OutboundMessage {
            channel: "test".into(),
            chat_id: "chat1".into(),
            content: "response".into(),
            reply_to_message_id: None,
        };

        bus.publish_outbound(msg).await.unwrap();
        let received = bus.consume_outbound().await.unwrap();
        assert_eq!(received.content, "response");
    }

    #[tokio::test]
    async fn test_close_rejects_publish() {
        let bus = MessageBus::new();
        bus.close();

        let msg = InboundMessage {
            content: "should fail".into(),
            ..Default::default()
        };
        assert!(bus.publish_inbound(msg).await.is_err());
    }
}
