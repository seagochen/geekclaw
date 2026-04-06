package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// FallbackChain 在多个候选者之间协调模型故障转移。
type FallbackChain struct {
	cooldown *CooldownTracker

	// 缓存最近成功的候选者，下次优先尝试
	lastSuccessMu sync.RWMutex
	lastSuccess   map[string]*lastSuccessEntry // key = 候选列表指纹
}

// lastSuccessEntry 记录最近成功的候选者及其时间。
type lastSuccessEntry struct {
	provider string
	model    string
	at       time.Time
}

const lastSuccessTTL = 5 * time.Minute

// FallbackCandidate 表示一个待尝试的模型/提供者。
type FallbackCandidate struct {
	Provider string
	Model    string
}

// FallbackResult 包含成功的响应以及所有尝试的元数据。
type FallbackResult struct {
	Response *LLMResponse
	Provider string
	Model    string
	Attempts []FallbackAttempt
}

// FallbackAttempt 记录故障转移链中的一次尝试。
type FallbackAttempt struct {
	Provider string
	Model    string
	Error    error
	Reason   FailoverReason
	Duration time.Duration
	Skipped  bool // 为 true 表示因冷却期而被跳过
}

// NewFallbackChain 使用给定的冷却追踪器创建新的故障转移链。
func NewFallbackChain(cooldown *CooldownTracker) *FallbackChain {
	return &FallbackChain{
		cooldown:    cooldown,
		lastSuccess: make(map[string]*lastSuccessEntry),
	}
}

// candidatesKey 为候选者列表生成缓存键。
func candidatesKey(candidates []FallbackCandidate) string {
	if len(candidates) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range candidates {
		if i > 0 {
			sb.WriteByte('|')
		}
		sb.WriteString(c.Provider)
		sb.WriteByte('/')
		sb.WriteString(c.Model)
	}
	return sb.String()
}

// ResolveCandidates 将模型配置解析为去重后的候选者列表。
func ResolveCandidates(cfg ModelConfig, defaultProvider string) []FallbackCandidate {
	return ResolveCandidatesWithLookup(cfg, defaultProvider, nil)
}

// ResolveCandidatesWithLookup 将模型配置解析为去重后的候选者列表，支持自定义查找函数。
func ResolveCandidatesWithLookup(
	cfg ModelConfig,
	defaultProvider string,
	lookup func(raw string) (resolved string, ok bool),
) []FallbackCandidate {
	seen := make(map[string]bool)
	var candidates []FallbackCandidate

	addCandidate := func(raw string) {
		candidateRaw := strings.TrimSpace(raw)
		if lookup != nil {
			if resolved, ok := lookup(candidateRaw); ok {
				candidateRaw = resolved
			}
		}

		ref := ParseModelRef(candidateRaw, defaultProvider)
		if ref == nil {
			return
		}
		key := ModelKey(ref.Provider, ref.Model)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, FallbackCandidate{
			Provider: ref.Provider,
			Model:    ref.Model,
		})
	}

	// 主模型优先。
	addCandidate(cfg.Primary)

	// 然后是备选模型。
	for _, fb := range cfg.Fallbacks {
		addCandidate(fb)
	}

	return candidates
}

// Execute 执行文本/对话请求的故障转移链。
// 按顺序尝试每个候选者，遵循冷却期和错误分类规则。
//
// 行为：
//   - 处于冷却期的候选者将被跳过（记录为跳过的尝试）。
//   - context.Canceled 立即中止（用户取消，不进行故障转移）。
//   - 不可重试的错误（格式错误）立即中止。
//   - 可重试的错误触发故障转移到下一个候选者。
//   - 成功后标记提供者为可用（重置冷却期）。
//   - 如果全部失败，返回包含所有尝试的聚合错误。
func (fc *FallbackChain) Execute(
	ctx context.Context,
	candidates []FallbackCandidate,
	run func(ctx context.Context, provider, model string) (*LLMResponse, error),
) (*FallbackResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("fallback: no candidates configured")
	}

	result := &FallbackResult{
		Attempts: make([]FallbackAttempt, 0, len(candidates)),
	}

	// 尝试使用缓存的最近成功候选者，避免每次都从头遍历
	cacheKey := candidatesKey(candidates)
	fc.lastSuccessMu.RLock()
	cached := fc.lastSuccess[cacheKey]
	fc.lastSuccessMu.RUnlock()

	if cached != nil && time.Since(cached.at) < lastSuccessTTL {
		if fc.cooldown.IsAvailable(cached.provider) {
			resp, err := run(ctx, cached.provider, cached.model)
			if err == nil {
				fc.cooldown.MarkSuccess(cached.provider)
				fc.lastSuccessMu.Lock()
				fc.lastSuccess[cacheKey] = &lastSuccessEntry{
					provider: cached.provider,
					model:    cached.model,
					at:       time.Now(),
				}
				fc.lastSuccessMu.Unlock()
				result.Response = resp
				result.Provider = cached.provider
				result.Model = cached.model
				return result, nil
			}
			// 缓存候选失败，继续正常故障转移流程
		}
	}

	for i, candidate := range candidates {
		// 每次尝试前检查上下文。
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		// 检查冷却期。
		if !fc.cooldown.IsAvailable(candidate.Provider) {
			remaining := fc.cooldown.CooldownRemaining(candidate.Provider)
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Skipped:  true,
				Reason:   FailoverRateLimit,
				Error: fmt.Errorf(
					"provider %s in cooldown (%s remaining)",
					candidate.Provider,
					remaining.Round(time.Second),
				),
			})
			continue
		}

		// 执行运行函数。
		start := time.Now()
		resp, err := run(ctx, candidate.Provider, candidate.Model)
		elapsed := time.Since(start)

		if err == nil {
			// 成功 — 缓存此候选者供下次优先使用。
			fc.cooldown.MarkSuccess(candidate.Provider)
			fc.lastSuccessMu.Lock()
			fc.lastSuccess[cacheKey] = &lastSuccessEntry{
				provider: candidate.Provider,
				model:    candidate.Model,
				at:       time.Now(),
			}
			fc.lastSuccessMu.Unlock()
			result.Response = resp
			result.Provider = candidate.Provider
			result.Model = candidate.Model
			return result, nil
		}

		// 上下文取消：立即中止，不进行故障转移。
		if ctx.Err() == context.Canceled {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Duration: elapsed,
			})
			return nil, context.Canceled
		}

		// 对错误进行分类。
		failErr := ClassifyError(err, candidate.Provider, candidate.Model)

		if failErr == nil {
			// 无法分类的错误：不进行故障转移，立即返回。
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Duration: elapsed,
			})
			return nil, fmt.Errorf("fallback: unclassified error from %s/%s: %w",
				candidate.Provider, candidate.Model, err)
		}

		// 不可重试的错误：立即中止。
		if !failErr.IsRetriable() {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    failErr,
				Reason:   failErr.Reason,
				Duration: elapsed,
			})
			return nil, failErr
		}

		// 可重试的错误：标记失败并继续尝试下一个候选者。
		fc.cooldown.MarkFailure(candidate.Provider, failErr.Reason)
		result.Attempts = append(result.Attempts, FallbackAttempt{
			Provider: candidate.Provider,
			Model:    candidate.Model,
			Error:    failErr,
			Reason:   failErr.Reason,
			Duration: elapsed,
		})

		// 如果这是最后一个候选者，返回聚合错误。
		if i == len(candidates)-1 {
			return nil, &FallbackExhaustedError{Attempts: result.Attempts}
		}
	}

	// 所有候选者都被跳过（全部处于冷却期）。
	return nil, &FallbackExhaustedError{Attempts: result.Attempts}
}

// ExecuteImage 执行图像/视觉请求的故障转移链。
// 比 Execute 简单：不检查冷却期（图像端点有不同的速率限制）。
// 图像尺寸/大小错误立即中止（不可重试）。
func (fc *FallbackChain) ExecuteImage(
	ctx context.Context,
	candidates []FallbackCandidate,
	run func(ctx context.Context, provider, model string) (*LLMResponse, error),
) (*FallbackResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("image fallback: no candidates configured")
	}

	result := &FallbackResult{
		Attempts: make([]FallbackAttempt, 0, len(candidates)),
	}

	for i, candidate := range candidates {
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		start := time.Now()
		resp, err := run(ctx, candidate.Provider, candidate.Model)
		elapsed := time.Since(start)

		if err == nil {
			result.Response = resp
			result.Provider = candidate.Provider
			result.Model = candidate.Model
			return result, nil
		}

		if ctx.Err() == context.Canceled {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Duration: elapsed,
			})
			return nil, context.Canceled
		}

		// 图像尺寸/大小错误不可重试。
		errMsg := strings.ToLower(err.Error())
		if IsImageDimensionError(errMsg) || IsImageSizeError(errMsg) {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Reason:   FailoverFormat,
				Duration: elapsed,
			})
			return nil, &FailoverError{
				Reason:   FailoverFormat,
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Wrapped:  err,
			}
		}

		// 其他错误：记录并尝试下一个。
		result.Attempts = append(result.Attempts, FallbackAttempt{
			Provider: candidate.Provider,
			Model:    candidate.Model,
			Error:    err,
			Duration: elapsed,
		})

		if i == len(candidates)-1 {
			return nil, &FallbackExhaustedError{Attempts: result.Attempts}
		}
	}

	return nil, &FallbackExhaustedError{Attempts: result.Attempts}
}

// FallbackExhaustedError 表示所有故障转移候选者都已尝试并失败。
type FallbackExhaustedError struct {
	Attempts []FallbackAttempt
}

func (e *FallbackExhaustedError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("fallback: all %d candidates failed:", len(e.Attempts)))
	for i, a := range e.Attempts {
		if a.Skipped {
			sb.WriteString(fmt.Sprintf("\n  [%d] %s/%s: skipped (cooldown)", i+1, a.Provider, a.Model))
		} else {
			sb.WriteString(fmt.Sprintf("\n  [%d] %s/%s: %v (reason=%s, %s)",
				i+1, a.Provider, a.Model, a.Error, a.Reason, a.Duration.Round(time.Millisecond)))
		}
	}
	return sb.String()
}
