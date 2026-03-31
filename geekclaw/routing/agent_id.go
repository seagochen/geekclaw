package routing

import (
	"regexp"
	"strings"
)

// 默认常量，用于智能体 ID 和账户 ID 的规范化处理。
const (
	DefaultAgentID   = "main"    // 默认智能体 ID
	DefaultMainKey   = "main"    // 默认主会话键
	DefaultAccountID = "default" // 默认账户 ID
	MaxAgentIDLength = 64        // 智能体 ID 最大长度
)

// ID 验证和清理用的正则表达式。
var (
	validIDRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`) // 合法 ID 格式
	invalidCharsRe = regexp.MustCompile(`[^a-z0-9_-]+`)               // 非法字符匹配
	leadingDashRe  = regexp.MustCompile(`^-+`)                        // 前导连字符
	trailingDashRe = regexp.MustCompile(`-+$`)                        // 尾部连字符
)

// NormalizeAgentID 将智能体 ID 规范化为 [a-z0-9][a-z0-9_-]{0,63} 格式。
// 非法字符会被折叠为 "-"，前导和尾部的连字符会被去除。
// 空输入返回 DefaultAgentID（"main"）。
func NormalizeAgentID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return DefaultAgentID
	}
	lower := strings.ToLower(trimmed)
	if validIDRe.MatchString(lower) {
		return lower
	}
	result := invalidCharsRe.ReplaceAllString(lower, "-")
	result = leadingDashRe.ReplaceAllString(result, "")
	result = trailingDashRe.ReplaceAllString(result, "")
	if len(result) > MaxAgentIDLength {
		result = result[:MaxAgentIDLength]
	}
	if result == "" {
		return DefaultAgentID
	}
	return result
}

// NormalizeAccountID 规范化账户 ID。空输入返回 DefaultAccountID。
func NormalizeAccountID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return DefaultAccountID
	}
	lower := strings.ToLower(trimmed)
	if validIDRe.MatchString(lower) {
		return lower
	}
	result := invalidCharsRe.ReplaceAllString(lower, "-")
	result = leadingDashRe.ReplaceAllString(result, "")
	result = trailingDashRe.ReplaceAllString(result, "")
	if len(result) > MaxAgentIDLength {
		result = result[:MaxAgentIDLength]
	}
	if result == "" {
		return DefaultAccountID
	}
	return result
}
