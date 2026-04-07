//! LLM 错误分类器。
//!
//! 通过 HTTP 状态码和错误消息模式匹配，将错误分类为 FailoverReason。

use crate::{FailoverError, FailoverReason};
use regex::Regex;
use std::sync::LazyLock;

/// 将错误分类为 FailoverError。不可分类时返回 None。
pub fn classify_error(
    err: &(dyn std::error::Error + Send + Sync),
    provider: &str,
    model: &str,
) -> Option<FailoverError> {
    let msg = err.to_string().to_lowercase();

    // 图像尺寸/大小错误：不可重试。
    if is_image_dimension_error(&msg) || is_image_size_error(&msg) {
        return Some(FailoverError {
            reason: FailoverReason::Format,
            provider: provider.into(),
            model: model.into(),
            status: 0,
            source: msg.into(),
        });
    }

    // HTTP 状态码匹配。
    if let Some(status) = extract_http_status(&msg) {
        if let Some(reason) = classify_by_status(status) {
            return Some(FailoverError {
                reason,
                provider: provider.into(),
                model: model.into(),
                status,
                source: msg.into(),
            });
        }
    }

    // 消息模式匹配。
    if let Some(reason) = classify_by_message(&msg) {
        return Some(FailoverError {
            reason,
            provider: provider.into(),
            model: model.into(),
            status: 0,
            source: msg.into(),
        });
    }

    None
}

/// 将 HTTP 状态码映射到 FailoverReason。
fn classify_by_status(status: u16) -> Option<FailoverReason> {
    match status {
        401 | 403 => Some(FailoverReason::Auth),
        402 => Some(FailoverReason::Billing),
        408 => Some(FailoverReason::Timeout),
        429 => Some(FailoverReason::RateLimit),
        400 => Some(FailoverReason::Format),
        500 | 502 | 503 | 521 | 522 | 523 | 524 | 529 => Some(FailoverReason::Timeout),
        _ => None,
    }
}

/// 按模式匹配错误消息。优先级顺序来自 OpenClaw。
fn classify_by_message(msg: &str) -> Option<FailoverReason> {
    if matches_any(msg, &RATE_LIMIT_PATTERNS) {
        return Some(FailoverReason::RateLimit);
    }
    if matches_any(msg, &OVERLOADED_PATTERNS) {
        return Some(FailoverReason::RateLimit);
    }
    if matches_any(msg, &BILLING_PATTERNS) {
        return Some(FailoverReason::Billing);
    }
    if matches_any(msg, &TIMEOUT_PATTERNS) {
        return Some(FailoverReason::Timeout);
    }
    if matches_any(msg, &AUTH_PATTERNS) {
        return Some(FailoverReason::Auth);
    }
    if matches_any(msg, &FORMAT_PATTERNS) {
        return Some(FailoverReason::Format);
    }
    None
}

/// 从错误消息中提取 HTTP 状态码。
fn extract_http_status(msg: &str) -> Option<u16> {
    static PATTERNS: LazyLock<Vec<Regex>> = LazyLock::new(|| {
        vec![
            Regex::new(r"status[:\s]+(\d{3})").unwrap(),
            Regex::new(r"http[/\s]+\d*\.?\d*\s+(\d{3})").unwrap(),
            Regex::new(r"\b([3-5]\d{2})\b").unwrap(),
        ]
    });

    for re in PATTERNS.iter() {
        if let Some(caps) = re.captures(msg) {
            if let Some(m) = caps.get(1) {
                if let Ok(n) = m.as_str().parse::<u16>() {
                    return Some(n);
                }
            }
        }
    }
    None
}

fn is_image_dimension_error(msg: &str) -> bool {
    msg.contains("image dimensions exceed max")
}

fn is_image_size_error(msg: &str) -> bool {
    static RE: LazyLock<Regex> = LazyLock::new(|| Regex::new(r"(?i)image exceeds.*mb").unwrap());
    RE.is_match(msg)
}

// --- 错误模式定义 ---

enum Pattern {
    Substring(&'static str),
    Regex(Regex),
}

fn matches_any(msg: &str, patterns: &[Pattern]) -> bool {
    patterns.iter().any(|p| match p {
        Pattern::Substring(s) => msg.contains(s),
        Pattern::Regex(re) => re.is_match(msg),
    })
}

macro_rules! patterns {
    ($($item:expr),* $(,)?) => {{
        vec![$($item),*]
    }};
}

fn sub(s: &'static str) -> Pattern {
    Pattern::Substring(s)
}

fn rxp(s: &str) -> Pattern {
    Pattern::Regex(Regex::new(&format!("(?i){s}")).unwrap())
}

static RATE_LIMIT_PATTERNS: LazyLock<Vec<Pattern>> = LazyLock::new(|| {
    patterns![
        rxp(r"rate[_ ]limit"),
        sub("too many requests"),
        sub("429"),
        sub("exceeded your current quota"),
        rxp(r"exceeded.*quota"),
        rxp(r"resource has been exhausted"),
        rxp(r"resource.*exhausted"),
        sub("resource_exhausted"),
        sub("quota exceeded"),
        sub("usage limit"),
    ]
});

static OVERLOADED_PATTERNS: LazyLock<Vec<Pattern>> = LazyLock::new(|| {
    patterns![
        rxp(r"overloaded_error"),
        sub("overloaded"),
    ]
});

static TIMEOUT_PATTERNS: LazyLock<Vec<Pattern>> = LazyLock::new(|| {
    patterns![
        sub("timeout"),
        sub("timed out"),
        sub("deadline exceeded"),
        sub("context deadline exceeded"),
    ]
});

static BILLING_PATTERNS: LazyLock<Vec<Pattern>> = LazyLock::new(|| {
    patterns![
        rxp(r"\b402\b"),
        sub("payment required"),
        sub("insufficient credits"),
        sub("credit balance"),
        sub("plans & billing"),
        sub("insufficient balance"),
    ]
});

static AUTH_PATTERNS: LazyLock<Vec<Pattern>> = LazyLock::new(|| {
    patterns![
        rxp(r"invalid[_ ]?api[_ ]?key"),
        sub("incorrect api key"),
        sub("invalid token"),
        sub("authentication"),
        sub("unauthorized"),
        sub("forbidden"),
        sub("access denied"),
        sub("expired"),
        rxp(r"\b401\b"),
        rxp(r"\b403\b"),
        sub("no credentials found"),
        sub("no api key found"),
    ]
});

static FORMAT_PATTERNS: LazyLock<Vec<Pattern>> = LazyLock::new(|| {
    patterns![
        sub("string should match pattern"),
        sub("tool_use.id"),
        sub("tool_use_id"),
        sub("invalid request format"),
    ]
});

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_classify_rate_limit() {
        let err: Box<dyn std::error::Error + Send + Sync> =
            "rate limit exceeded".into();
        let fe = classify_error(err.as_ref(), "openai", "gpt-4o").unwrap();
        assert_eq!(fe.reason, FailoverReason::RateLimit);
    }

    #[test]
    fn test_classify_by_status_429() {
        let err: Box<dyn std::error::Error + Send + Sync> =
            "status: 429 too many requests".into();
        let fe = classify_error(err.as_ref(), "openai", "gpt-4o").unwrap();
        assert_eq!(fe.reason, FailoverReason::RateLimit);
    }

    #[test]
    fn test_classify_auth() {
        let err: Box<dyn std::error::Error + Send + Sync> =
            "invalid api key provided".into();
        let fe = classify_error(err.as_ref(), "openai", "gpt-4o").unwrap();
        assert_eq!(fe.reason, FailoverReason::Auth);
    }

    #[test]
    fn test_unclassifiable_returns_none() {
        let err: Box<dyn std::error::Error + Send + Sync> =
            "some random error".into();
        assert!(classify_error(err.as_ref(), "openai", "gpt-4o").is_none());
    }
}
