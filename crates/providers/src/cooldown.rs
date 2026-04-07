//! Provider 冷却追踪器。
//!
//! 通过指数退避管理每个 Provider 的冷却状态。

use crate::FailoverReason;
use std::collections::HashMap;
use std::sync::RwLock;
use std::time::{Duration, Instant};

/// 默认故障窗口（24 小时内无故障则重置计数器）。
const DEFAULT_FAILURE_WINDOW: Duration = Duration::from_secs(24 * 3600);

/// Provider 冷却追踪器，线程安全。
pub struct CooldownTracker {
    inner: RwLock<TrackerInner>,
    failure_window: Duration,
}

struct TrackerInner {
    entries: HashMap<String, CooldownEntry>,
}

struct CooldownEntry {
    error_count: u32,
    failure_counts: HashMap<FailoverReason, u32>,
    cooldown_end: Option<Instant>,
    disabled_until: Option<Instant>,
    last_failure: Option<Instant>,
}

impl CooldownTracker {
    /// 创建新的冷却追踪器。
    pub fn new() -> Self {
        Self {
            inner: RwLock::new(TrackerInner {
                entries: HashMap::new(),
            }),
            failure_window: DEFAULT_FAILURE_WINDOW,
        }
    }

    /// 记录 Provider 的一次失败并设置冷却期。
    pub fn mark_failure(&self, provider: &str, reason: FailoverReason) {
        let mut inner = self.inner.write().unwrap();
        let now = Instant::now();
        let entry = inner
            .entries
            .entry(provider.to_string())
            .or_insert_with(CooldownEntry::new);

        // 故障窗口重置。
        if let Some(last) = entry.last_failure {
            if now.duration_since(last) > self.failure_window {
                entry.error_count = 0;
                entry.failure_counts.clear();
            }
        }

        entry.error_count += 1;
        *entry.failure_counts.entry(reason).or_insert(0) += 1;
        entry.last_failure = Some(now);

        if reason == FailoverReason::Billing {
            let billing_count = entry.failure_counts[&FailoverReason::Billing];
            entry.disabled_until = Some(now + calculate_billing_cooldown(billing_count));
        } else {
            entry.cooldown_end = Some(now + calculate_standard_cooldown(entry.error_count));
        }
    }

    /// 标记 Provider 成功，重置所有冷却状态。
    pub fn mark_success(&self, provider: &str) {
        let mut inner = self.inner.write().unwrap();
        if let Some(entry) = inner.entries.get_mut(provider) {
            entry.error_count = 0;
            entry.failure_counts.clear();
            entry.cooldown_end = None;
            entry.disabled_until = None;
        }
    }

    /// 检查 Provider 是否可用（未处于冷却期）。
    pub fn is_available(&self, provider: &str) -> bool {
        let inner = self.inner.read().unwrap();
        let Some(entry) = inner.entries.get(provider) else {
            return true;
        };

        let now = Instant::now();

        // 计费禁用优先。
        if let Some(until) = entry.disabled_until {
            if now < until {
                return false;
            }
        }

        // 标准冷却期。
        if let Some(end) = entry.cooldown_end {
            if now < end {
                return false;
            }
        }

        true
    }

    /// 返回 Provider 变为可用还需要的时间。已可用则返回 Duration::ZERO。
    pub fn cooldown_remaining(&self, provider: &str) -> Duration {
        let inner = self.inner.read().unwrap();
        let Some(entry) = inner.entries.get(provider) else {
            return Duration::ZERO;
        };

        let now = Instant::now();
        let mut remaining = Duration::ZERO;

        if let Some(until) = entry.disabled_until {
            if now < until {
                let d = until - now;
                if d > remaining {
                    remaining = d;
                }
            }
        }

        if let Some(end) = entry.cooldown_end {
            if now < end {
                let d = end - now;
                if d > remaining {
                    remaining = d;
                }
            }
        }

        remaining
    }
}

impl Default for CooldownTracker {
    fn default() -> Self {
        Self::new()
    }
}

impl CooldownEntry {
    fn new() -> Self {
        Self {
            error_count: 0,
            failure_counts: HashMap::new(),
            cooldown_end: None,
            disabled_until: None,
            last_failure: None,
        }
    }
}

/// 标准指数退避：min(1h, 1min * 5^min(n-1, 3))
///   1 次 → 1 分钟
///   2 次 → 5 分钟
///   3 次 → 25 分钟
///   4+ 次 → 1 小时
fn calculate_standard_cooldown(error_count: u32) -> Duration {
    let n = error_count.max(1);
    let exp = (n - 1).min(3);
    let ms = (60_000.0 * 5_f64.powi(exp as i32)) as u64;
    let ms = ms.min(3_600_000);
    Duration::from_millis(ms)
}

/// 计费指数退避：min(24h, 5h * 2^min(n-1, 10))
///   1 次 → 5 小时
///   2 次 → 10 小时
///   3 次 → 20 小时
///   4+ 次 → 24 小时
fn calculate_billing_cooldown(billing_count: u32) -> Duration {
    const BASE_MS: u64 = 5 * 3600 * 1000;
    const MAX_MS: u64 = 24 * 3600 * 1000;

    let n = billing_count.max(1);
    let exp = (n - 1).min(10);
    let raw = BASE_MS as f64 * 2_f64.powi(exp as i32);
    let ms = (raw as u64).min(MAX_MS);
    Duration::from_millis(ms)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_standard_cooldown_values() {
        assert_eq!(calculate_standard_cooldown(1), Duration::from_secs(60));
        assert_eq!(calculate_standard_cooldown(2), Duration::from_secs(300));
        assert_eq!(calculate_standard_cooldown(3), Duration::from_secs(1500));
        assert_eq!(calculate_standard_cooldown(4), Duration::from_secs(3600));
        assert_eq!(calculate_standard_cooldown(10), Duration::from_secs(3600));
    }

    #[test]
    fn test_available_by_default() {
        let tracker = CooldownTracker::new();
        assert!(tracker.is_available("openai"));
    }

    #[test]
    fn test_mark_failure_enters_cooldown() {
        let tracker = CooldownTracker::new();
        tracker.mark_failure("openai", FailoverReason::RateLimit);
        assert!(!tracker.is_available("openai"));
        assert!(tracker.cooldown_remaining("openai") > Duration::ZERO);
    }

    #[test]
    fn test_mark_success_resets() {
        let tracker = CooldownTracker::new();
        tracker.mark_failure("openai", FailoverReason::RateLimit);
        assert!(!tracker.is_available("openai"));

        tracker.mark_success("openai");
        assert!(tracker.is_available("openai"));
        assert_eq!(tracker.cooldown_remaining("openai"), Duration::ZERO);
    }
}
