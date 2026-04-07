//! 故障转移链：在多个 LLM Provider 候选者之间协调故障转移。

use crate::model_ref::{model_key, parse_model_ref};
use crate::{CooldownTracker, FailoverReason, LlmResponse, ModelConfig};
use std::collections::HashMap;
use std::fmt;
use std::future::Future;
use std::sync::RwLock;
use std::time::{Duration, Instant};
use tracing::warn;

/// 最近成功候选者的缓存 TTL。
const LAST_SUCCESS_TTL: Duration = Duration::from_secs(300);

/// 故障转移候选者。
#[derive(Debug, Clone)]
pub struct FallbackCandidate {
    pub provider: String,
    pub model: String,
}

/// 故障转移结果。
pub struct FallbackResult {
    pub response: LlmResponse,
    pub provider: String,
    pub model: String,
    pub attempts: Vec<FallbackAttempt>,
}

/// 故障转移链中的一次尝试记录。
pub struct FallbackAttempt {
    pub provider: String,
    pub model: String,
    pub error: Option<Box<dyn std::error::Error + Send + Sync>>,
    pub reason: Option<FailoverReason>,
    pub duration: Duration,
    pub skipped: bool,
}

/// 所有候选者都已耗尽的错误。
#[derive(Debug)]
pub struct FallbackExhaustedError {
    pub attempts: Vec<FallbackAttemptSummary>,
}

/// 用于错误显示的尝试摘要。
#[derive(Debug)]
pub struct FallbackAttemptSummary {
    pub provider: String,
    pub model: String,
    pub error_msg: String,
    pub reason: Option<FailoverReason>,
    pub skipped: bool,
}

impl fmt::Display for FallbackExhaustedError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "fallback: all {} candidates failed:",
            self.attempts.len()
        )?;
        for (i, a) in self.attempts.iter().enumerate() {
            if a.skipped {
                write!(
                    f,
                    "\n  [{}] {}/{}: skipped (cooldown)",
                    i + 1,
                    a.provider,
                    a.model
                )?;
            } else {
                write!(
                    f,
                    "\n  [{}] {}/{}: {} (reason={:?})",
                    i + 1,
                    a.provider,
                    a.model,
                    a.error_msg,
                    a.reason
                )?;
            }
        }
        Ok(())
    }
}

impl std::error::Error for FallbackExhaustedError {}

struct LastSuccessEntry {
    provider: String,
    model: String,
    at: Instant,
}

/// 故障转移链，在多个候选者之间协调模型故障转移。
pub struct FallbackChain {
    cooldown: CooldownTracker,
    last_success: RwLock<HashMap<String, LastSuccessEntry>>,
}

impl FallbackChain {
    /// 创建新的故障转移链。
    pub fn new(cooldown: CooldownTracker) -> Self {
        Self {
            cooldown,
            last_success: RwLock::new(HashMap::new()),
        }
    }

    /// 将模型配置解析为去重后的候选者列表。
    pub fn resolve_candidates(
        cfg: &ModelConfig,
        default_provider: &str,
    ) -> Vec<FallbackCandidate> {
        let mut seen = std::collections::HashSet::new();
        let mut candidates = Vec::new();

        let mut add = |raw: &str| {
            let raw = raw.trim();
            if let Some(r) = parse_model_ref(raw, default_provider) {
                let key = model_key(&r.provider, &r.model);
                if seen.insert(key) {
                    candidates.push(FallbackCandidate {
                        provider: r.provider,
                        model: r.model,
                    });
                }
            }
        };

        add(&cfg.primary);
        for fb in &cfg.fallbacks {
            add(fb);
        }

        candidates
    }

    /// 执行故障转移链。按顺序尝试每个候选者，遵循冷却期和错误分类。
    pub async fn execute<F, Fut>(
        &self,
        candidates: &[FallbackCandidate],
        run: F,
    ) -> Result<FallbackResult, Box<dyn std::error::Error + Send + Sync>>
    where
        F: Fn(&str, &str) -> Fut,
        Fut: Future<Output = Result<LlmResponse, Box<dyn std::error::Error + Send + Sync>>>,
    {
        if candidates.is_empty() {
            return Err("fallback: no candidates configured".into());
        }

        let mut attempts = Vec::with_capacity(candidates.len());
        let cache_key = candidates_key(candidates);

        // 尝试缓存的最近成功候选者。
        if let Some(cached) = self.get_cached_success(&cache_key) {
            if self.cooldown.is_available(&cached.provider) {
                match run(&cached.provider, &cached.model).await {
                    Ok(response) => {
                        self.cooldown.mark_success(&cached.provider);
                        self.cache_success(&cache_key, &cached.provider, &cached.model);
                        return Ok(FallbackResult {
                            response,
                            provider: cached.provider,
                            model: cached.model,
                            attempts,
                        });
                    }
                    Err(_) => {
                        // 缓存候选失败，继续正常流程。
                    }
                }
            }
        }

        let mut summaries = Vec::new();

        for (i, candidate) in candidates.iter().enumerate() {
            // 检查冷却期。
            if !self.cooldown.is_available(&candidate.provider) {
                let remaining = self.cooldown.cooldown_remaining(&candidate.provider);
                warn!(
                    provider = %candidate.provider,
                    model = %candidate.model,
                    remaining_secs = remaining.as_secs(),
                    "跳过处于冷却期的 provider"
                );
                attempts.push(FallbackAttempt {
                    provider: candidate.provider.clone(),
                    model: candidate.model.clone(),
                    error: Some(
                        format!(
                            "provider {} in cooldown ({:?} remaining)",
                            candidate.provider, remaining
                        )
                        .into(),
                    ),
                    reason: Some(FailoverReason::RateLimit),
                    duration: Duration::ZERO,
                    skipped: true,
                });
                summaries.push(FallbackAttemptSummary {
                    provider: candidate.provider.clone(),
                    model: candidate.model.clone(),
                    error_msg: "cooldown".into(),
                    reason: Some(FailoverReason::RateLimit),
                    skipped: true,
                });
                continue;
            }

            let start = Instant::now();
            match run(&candidate.provider, &candidate.model).await {
                Ok(response) => {
                    self.cooldown.mark_success(&candidate.provider);
                    self.cache_success(&cache_key, &candidate.provider, &candidate.model);
                    return Ok(FallbackResult {
                        response,
                        provider: candidate.provider.clone(),
                        model: candidate.model.clone(),
                        attempts,
                    });
                }
                Err(err) => {
                    let elapsed = start.elapsed();

                    // 错误分类。
                    if let Some(fail_err) =
                        crate::classify_error(err.as_ref(), &candidate.provider, &candidate.model)
                    {
                        if !fail_err.is_retriable() {
                            // 不可重试，立即中止。
                            return Err(Box::new(fail_err));
                        }

                        self.cooldown
                            .mark_failure(&candidate.provider, fail_err.reason);

                        summaries.push(FallbackAttemptSummary {
                            provider: candidate.provider.clone(),
                            model: candidate.model.clone(),
                            error_msg: err.to_string(),
                            reason: Some(fail_err.reason),
                            skipped: false,
                        });

                        attempts.push(FallbackAttempt {
                            provider: candidate.provider.clone(),
                            model: candidate.model.clone(),
                            error: Some(err),
                            reason: Some(fail_err.reason),
                            duration: elapsed,
                            skipped: false,
                        });
                    } else {
                        // 无法分类的错误，不触发故障转移。
                        return Err(err);
                    }

                    // 最后一个候选者，返回聚合错误。
                    if i == candidates.len() - 1 {
                        return Err(Box::new(FallbackExhaustedError {
                            attempts: summaries,
                        }));
                    }
                }
            }
        }

        // 所有候选者都被跳过。
        Err(Box::new(FallbackExhaustedError {
            attempts: summaries,
        }))
    }

    fn get_cached_success(&self, cache_key: &str) -> Option<FallbackCandidate> {
        let map = self.last_success.read().unwrap();
        let entry = map.get(cache_key)?;
        if entry.at.elapsed() < LAST_SUCCESS_TTL {
            Some(FallbackCandidate {
                provider: entry.provider.clone(),
                model: entry.model.clone(),
            })
        } else {
            None
        }
    }

    fn cache_success(&self, cache_key: &str, provider: &str, model: &str) {
        let mut map = self.last_success.write().unwrap();
        map.insert(
            cache_key.to_string(),
            LastSuccessEntry {
                provider: provider.into(),
                model: model.into(),
                at: Instant::now(),
            },
        );
    }
}

/// 为候选者列表生成缓存键。
fn candidates_key(candidates: &[FallbackCandidate]) -> String {
    candidates
        .iter()
        .map(|c| format!("{}/{}", c.provider, c.model))
        .collect::<Vec<_>>()
        .join("|")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_resolve_candidates_dedup() {
        let cfg = ModelConfig {
            primary: "openai/gpt-4o".into(),
            fallbacks: vec!["anthropic/claude-opus".into(), "openai/gpt-4o".into()],
        };
        let candidates = FallbackChain::resolve_candidates(&cfg, "openai");
        assert_eq!(candidates.len(), 2);
        assert_eq!(candidates[0].provider, "openai");
        assert_eq!(candidates[1].provider, "anthropic");
    }

    #[tokio::test]
    async fn test_execute_first_success() {
        let chain = FallbackChain::new(CooldownTracker::new());
        let candidates = vec![FallbackCandidate {
            provider: "openai".into(),
            model: "gpt-4o".into(),
        }];

        let result = chain
            .execute(&candidates, |_provider, _model| async {
                Ok(LlmResponse {
                    content: "hello".into(),
                    reasoning_content: None,
                    tool_calls: vec![],
                    finish_reason: "stop".into(),
                    usage: None,
                })
            })
            .await
            .unwrap();

        assert_eq!(result.provider, "openai");
        assert_eq!(result.response.content, "hello");
    }

    #[tokio::test]
    async fn test_execute_fallback_on_retriable_error() {
        let chain = FallbackChain::new(CooldownTracker::new());
        let candidates = vec![
            FallbackCandidate {
                provider: "openai".into(),
                model: "gpt-4o".into(),
            },
            FallbackCandidate {
                provider: "anthropic".into(),
                model: "claude-opus".into(),
            },
        ];

        let result = chain
            .execute(&candidates, |provider, _model| {
                let provider = provider.to_string();
                async move {
                    if provider == "openai" {
                        let err: Box<dyn std::error::Error + Send + Sync> =
                            "status: 429 rate limit".into();
                        Err(err)
                    } else {
                        Ok(LlmResponse {
                            content: "from anthropic".into(),
                            reasoning_content: None,
                            tool_calls: vec![],
                            finish_reason: "stop".into(),
                            usage: None,
                        })
                    }
                }
            })
            .await
            .unwrap();

        assert_eq!(result.provider, "anthropic");
        assert_eq!(result.response.content, "from anthropic");
    }
}
