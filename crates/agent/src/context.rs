//! 上下文构建器：构建系统提示词和管理 token 预算。

use geekclaw_providers::Message;

/// 上下文构建器，负责组装系统提示词和管理 token 预算。
pub struct ContextBuilder {
    /// 基础系统提示词。
    system_prompt: String,
    /// 最大上下文 token 数。
    max_context_tokens: usize,
}

impl ContextBuilder {
    pub fn new(system_prompt: String, max_context_tokens: usize) -> Self {
        Self {
            system_prompt,
            max_context_tokens,
        }
    }

    /// 构建完整的消息列表（系统提示词 + 历史 + 当前消息）。
    pub fn build_messages(
        &self,
        history: Vec<Message>,
        user_message: &str,
    ) -> Vec<Message> {
        let mut messages = Vec::new();

        // 系统提示词。
        if !self.system_prompt.is_empty() {
            messages.push(Message::system(&self.system_prompt));
        }

        // 历史消息。
        messages.extend(history);

        // 当前用户消息。
        if !user_message.is_empty() {
            messages.push(Message::user(user_message));
        }

        messages
    }

    /// 估算消息列表的 token 数（粗略估算：每 4 个字符约 1 token）。
    pub fn estimate_tokens(messages: &[Message]) -> usize {
        messages
            .iter()
            .map(|m| m.content.len() / 4 + 1)
            .sum()
    }

    /// 裁剪历史消息以适应 token 预算。
    /// 保留系统提示词和最近的消息，从最早的历史开始裁剪。
    pub fn trim_to_budget(&self, messages: &mut Vec<Message>) {
        while Self::estimate_tokens(messages) > self.max_context_tokens && messages.len() > 2 {
            // 保留第一条（系统提示词）和最后一条（用户消息），删除最早的历史。
            messages.remove(1);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_messages() {
        let builder = ContextBuilder::new("You are helpful.".into(), 128_000);
        let history = vec![
            Message::user("hi"),
            Message::assistant("hello!"),
        ];
        let msgs = builder.build_messages(history, "what's up?");

        assert_eq!(msgs.len(), 4);
        assert_eq!(msgs[0].role, "system");
        assert_eq!(msgs[1].role, "user");
        assert_eq!(msgs[2].role, "assistant");
        assert_eq!(msgs[3].role, "user");
        assert_eq!(msgs[3].content, "what's up?");
    }

    #[test]
    fn test_estimate_tokens() {
        let msgs = vec![Message::user("hello world")]; // 11 chars → ~3 tokens
        let tokens = ContextBuilder::estimate_tokens(&msgs);
        assert!(tokens > 0);
    }

    #[test]
    fn test_trim_to_budget() {
        // 每条消息 ~1 token（content.len()/4+1），预算设为 3 token
        let builder = ContextBuilder::new("s".into(), 3);
        let mut msgs = vec![
            Message::system("s"),
            Message::user("a long enough message to exceed budget"),
            Message::assistant("another long response that uses tokens"),
            Message::user("more history content here"),
            Message::assistant("yet another response"),
            Message::user("current"),
        ];
        let original_len = msgs.len();
        builder.trim_to_budget(&mut msgs);
        // 应该裁剪掉一些历史消息
        assert!(msgs.len() < original_len);
        assert_eq!(msgs[0].role, "system");
    }
}
