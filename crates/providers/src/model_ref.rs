//! 模型引用解析与 Provider 名称规范化。

/// 已解析的模型引用。
#[derive(Debug, Clone, PartialEq)]
pub struct ModelRef {
    pub provider: String,
    pub model: String,
}

/// 将 "anthropic/claude-opus" 解析为 ModelRef。
/// 没有斜杠时使用 default_provider。空输入返回 None。
pub fn parse_model_ref(raw: &str, default_provider: &str) -> Option<ModelRef> {
    let raw = raw.trim();
    if raw.is_empty() {
        return None;
    }

    if let Some(idx) = raw.find('/') {
        if idx == 0 {
            return None;
        }
        let provider = normalize_provider(&raw[..idx]);
        let model = raw[idx + 1..].trim();
        if model.is_empty() {
            return None;
        }
        Some(ModelRef {
            provider,
            model: model.to_string(),
        })
    } else {
        Some(ModelRef {
            provider: normalize_provider(default_provider),
            model: raw.to_string(),
        })
    }
}

/// 规范化 Provider 标识符。
pub fn normalize_provider(provider: &str) -> String {
    let p = provider.trim().to_lowercase();
    match p.as_str() {
        "z.ai" | "z-ai" => "zai".into(),
        "opencode-zen" => "opencode".into(),
        "qwen" => "qwen-portal".into(),
        "kimi-code" => "kimi-coding".into(),
        "gpt" => "openai".into(),
        "claude" => "anthropic".into(),
        "glm" => "zhipu".into(),
        "google" => "gemini".into(),
        _ => p,
    }
}

/// 返回用于去重的标准 "provider/model" 键。
pub fn model_key(provider: &str, model: &str) -> String {
    format!(
        "{}/{}",
        normalize_provider(provider),
        model.trim().to_lowercase()
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_with_provider() {
        let r = parse_model_ref("anthropic/claude-opus", "openai").unwrap();
        assert_eq!(r.provider, "anthropic");
        assert_eq!(r.model, "claude-opus");
    }

    #[test]
    fn test_parse_without_provider() {
        let r = parse_model_ref("gpt-4o", "openai").unwrap();
        assert_eq!(r.provider, "openai");
        assert_eq!(r.model, "gpt-4o");
    }

    #[test]
    fn test_parse_empty() {
        assert!(parse_model_ref("", "openai").is_none());
        assert!(parse_model_ref("  ", "openai").is_none());
    }

    #[test]
    fn test_normalize_aliases() {
        assert_eq!(normalize_provider("claude"), "anthropic");
        assert_eq!(normalize_provider("gpt"), "openai");
        assert_eq!(normalize_provider("google"), "gemini");
        assert_eq!(normalize_provider("GLM"), "zhipu");
    }

    #[test]
    fn test_model_key() {
        assert_eq!(model_key("Claude", "Opus"), "anthropic/opus");
    }
}
