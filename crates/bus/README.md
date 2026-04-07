# geekclaw-bus — 进程内消息总线

## 模块概述

基于 `tokio::sync::mpsc` 的异步消息总线，是所有模块间通信的桥梁。提供入站（外部 → Agent）和出站（Agent → 外部）两个独立通道，缓冲区大小 64。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/lib.rs` | `MessageBus` 实现：创建、发布、消费、关闭。包含 `AtomicBool` 的关闭状态检测和 3 个单元测试 |
| `src/types.rs` | 消息类型定义：`InboundMessage`（入站）、`OutboundMessage`（出站）、`SenderInfo`（发送者身份） |

## 核心类型

```rust
pub struct MessageBus {
    inbound_tx/rx: mpsc::Sender/Receiver<InboundMessage>,
    outbound_tx/rx: mpsc::Sender/Receiver<OutboundMessage>,
    closed: AtomicBool,
}
```

**API:**
- `publish_inbound(msg)` / `consume_inbound()` — 入站通道读写
- `publish_outbound(msg)` / `consume_outbound()` — 出站通道读写
- `inbound_sender()` / `outbound_sender()` — 获取 Sender 克隆，供外部模块使用
- `close()` — 标记关闭，后续 publish 返回 `BusError::Closed`

## 设计决策

- 使用 `AtomicBool` 而非 drop channel 来标记关闭，允许在 close 后仍能 drain 剩余消息
- 缓冲区 64 是经验值，足够应对突发消息而不会占用过多内存
- 没有使用 broadcast，因为当前架构是单消费者（AgentLoop）

## 不完善之处

- **无媒体通道**：Go 版本有独立的 `OutboundMediaMessage` 通道，Rust 版暂未实现
- **无背压机制**：channel 满时 send 会等待，没有丢弃策略或告警
- **单消费者**：不支持多个消费者竞争同一消息，如需扇出需要额外实现
