//! GeekClaw LLM Provider 抽象层。
//!
//! 提供统一的 LLM 调用接口、故障转移链和冷却追踪。

mod cooldown;
mod error_classifier;
mod external;
mod fallback;
mod model_ref;
mod openai_compat;
mod types;

pub use cooldown::CooldownTracker;
pub use error_classifier::classify_error;
pub use external::ExternalProvider;
pub use fallback::{FallbackCandidate, FallbackChain, FallbackResult};
pub use model_ref::{normalize_provider, parse_model_ref, ModelRef};
pub use openai_compat::OpenAICompatProvider;
pub use types::*;
