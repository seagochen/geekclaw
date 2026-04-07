// 消息类型，直接复用 providers 的 Message 定义。
//
// 这避免了 agent 层在 memory::Message 和 providers::Message 之间进行转换。

pub use geekclaw_providers::{FunctionCall, Message, ToolCall};
