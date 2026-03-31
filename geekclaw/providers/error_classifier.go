package providers

import (
	"context"
	"regexp"
	"strings"
)

// Go HTTP 错误消息中的常见模式
var httpStatusPatterns = []*regexp.Regexp{
	regexp.MustCompile(`status[:\s]+(\d{3})`),
	regexp.MustCompile(`http[/\s]+\d*\.?\d*\s+(\d{3})`),
	regexp.MustCompile(`\b([3-5]\d{2})\b`),
}

// errorPattern 定义用于错误分类的单个模式（字符串或正则表达式）。
type errorPattern struct {
	substring string
	regex     *regexp.Regexp
}

func substr(s string) errorPattern { return errorPattern{substring: s} }
func rxp(r string) errorPattern    { return errorPattern{regex: regexp.MustCompile("(?i)" + r)} }

// 按 FailoverReason 组织的错误模式，匹配 OpenClaw 生产环境（约 40 个模式）。
var (
	rateLimitPatterns = []errorPattern{
		rxp(`rate[_ ]limit`),
		substr("too many requests"),
		substr("429"),
		substr("exceeded your current quota"),
		rxp(`exceeded.*quota`),
		rxp(`resource has been exhausted`),
		rxp(`resource.*exhausted`),
		substr("resource_exhausted"),
		substr("quota exceeded"),
		substr("usage limit"),
	}

	overloadedPatterns = []errorPattern{
		rxp(`overloaded_error`),
		rxp(`"type"\s*:\s*"overloaded_error"`),
		substr("overloaded"),
	}

	timeoutPatterns = []errorPattern{
		substr("timeout"),
		substr("timed out"),
		substr("deadline exceeded"),
		substr("context deadline exceeded"),
	}

	billingPatterns = []errorPattern{
		rxp(`\b402\b`),
		substr("payment required"),
		substr("insufficient credits"),
		substr("credit balance"),
		substr("plans & billing"),
		substr("insufficient balance"),
	}

	authPatterns = []errorPattern{
		rxp(`invalid[_ ]?api[_ ]?key`),
		substr("incorrect api key"),
		substr("invalid token"),
		substr("authentication"),
		substr("re-authenticate"),
		substr("oauth token refresh failed"),
		substr("unauthorized"),
		substr("forbidden"),
		substr("access denied"),
		substr("expired"),
		substr("token has expired"),
		rxp(`\b401\b`),
		rxp(`\b403\b`),
		substr("no credentials found"),
		substr("no api key found"),
	}

	formatPatterns = []errorPattern{
		substr("string should match pattern"),
		substr("tool_use.id"),
		substr("tool_use_id"),
		substr("messages.1.content.1.tool_use.id"),
		substr("invalid request format"),
	}

	imageDimensionPatterns = []errorPattern{
		rxp(`image dimensions exceed max`),
	}

	imageSizePatterns = []errorPattern{
		rxp(`image exceeds.*mb`),
	}

	// 映射为超时的临时 HTTP 状态码（服务器端故障）。
	transientStatusCodes = map[int]bool{
		500: true, 502: true, 503: true,
		521: true, 522: true, 523: true, 524: true,
		529: true,
	}
)

// ClassifyError 将错误分类为带有原因的 FailoverError。
// 如果错误不可分类则返回 nil（未知错误不应触发故障转移）。
func ClassifyError(err error, provider, model string) *FailoverError {
	if err == nil {
		return nil
	}

	// 上下文取消：用户中止，不进行故障转移。
	if err == context.Canceled {
		return nil
	}

	// 上下文截止时间超过：视为超时，始终进行故障转移。
	if err == context.DeadlineExceeded {
		return &FailoverError{
			Reason:   FailoverTimeout,
			Provider: provider,
			Model:    model,
			Wrapped:  err,
		}
	}

	msg := strings.ToLower(err.Error())

	// 图像尺寸/大小错误：不可重试，不进行故障转移。
	if IsImageDimensionError(msg) || IsImageSizeError(msg) {
		return &FailoverError{
			Reason:   FailoverFormat,
			Provider: provider,
			Model:    model,
			Wrapped:  err,
		}
	}

	// 首先尝试提取 HTTP 状态码。
	if status := extractHTTPStatus(msg); status > 0 {
		if reason := classifyByStatus(status); reason != "" {
			return &FailoverError{
				Reason:   reason,
				Provider: provider,
				Model:    model,
				Status:   status,
				Wrapped:  err,
			}
		}
	}

	// 消息模式匹配（优先级顺序来自 OpenClaw）。
	if reason := classifyByMessage(msg); reason != "" {
		return &FailoverError{
			Reason:   reason,
			Provider: provider,
			Model:    model,
			Wrapped:  err,
		}
	}

	return nil
}

// classifyByStatus 将 HTTP 状态码映射到 FailoverReason。
func classifyByStatus(status int) FailoverReason {
	switch {
	case status == 401 || status == 403:
		return FailoverAuth
	case status == 402:
		return FailoverBilling
	case status == 408:
		return FailoverTimeout
	case status == 429:
		return FailoverRateLimit
	case status == 400:
		return FailoverFormat
	case transientStatusCodes[status]:
		return FailoverTimeout
	}
	return ""
}

// classifyByMessage 将错误消息与模式进行匹配。
// 优先级顺序很重要（来自 OpenClaw classifyFailoverReason）。
func classifyByMessage(msg string) FailoverReason {
	if matchesAny(msg, rateLimitPatterns) {
		return FailoverRateLimit
	}
	if matchesAny(msg, overloadedPatterns) {
		return FailoverRateLimit // 过载视为速率限制
	}
	if matchesAny(msg, billingPatterns) {
		return FailoverBilling
	}
	if matchesAny(msg, timeoutPatterns) {
		return FailoverTimeout
	}
	if matchesAny(msg, authPatterns) {
		return FailoverAuth
	}
	if matchesAny(msg, formatPatterns) {
		return FailoverFormat
	}
	return ""
}

// extractHTTPStatus 从错误消息中提取 HTTP 状态码。
// 查找如 "status: 429"、"status 429"、"http/1.1 429"、"http 429" 或独立的 "429" 等模式。
func extractHTTPStatus(msg string) int {
	for _, p := range httpStatusPatterns {
		if m := p.FindStringSubmatch(msg); len(m) > 1 {
			return parseDigits(m[1])
		}
	}
	return 0
}

// IsImageDimensionError 返回 true 表示消息指示图像尺寸错误。
func IsImageDimensionError(msg string) bool {
	return matchesAny(msg, imageDimensionPatterns)
}

// IsImageSizeError 返回 true 表示消息指示图像文件大小错误。
func IsImageSizeError(msg string) bool {
	return matchesAny(msg, imageSizePatterns)
}

// matchesAny 检查 msg 是否匹配任何一个模式。
func matchesAny(msg string, patterns []errorPattern) bool {
	for _, p := range patterns {
		if p.regex != nil {
			if p.regex.MatchString(msg) {
				return true
			}
		} else if p.substring != "" {
			if strings.Contains(msg, p.substring) {
				return true
			}
		}
	}
	return false
}

// parseDigits 将数字字符串转换为 int。
func parseDigits(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
