package utils

import (
	"strings"
	"sync/atomic"
	"unicode"
)

// disableTruncation 是全局变量，用于禁用截断功能。
var disableTruncation atomic.Bool

// SetDisableTruncation 全局启用或禁用字符串截断。
func SetDisableTruncation(enabled bool) {
	disableTruncation.Store(enabled)
}

// SanitizeMessageContent 移除 Unicode 控制字符、格式字符（RTL 覆盖、
// 零宽字符）和其他非图形字符，这些字符可能混淆 LLM
// 或导致代理 UI 的显示问题。
func SanitizeMessageContent(input string) string {
	var sb strings.Builder
	// 预分配内存以避免多次分配
	sb.Grow(len(input))

	for _, r := range input {
		// unicode.IsGraphic 对 Unicode 图形字符返回 true。
		// 包括字母、标记、数字、标点和符号。
		// 排除控制字符（Cc）、格式字符（Cf）、
		// 代理项（Cs）和私用区（Co）。
		if unicode.IsGraphic(r) || r == '\n' || r == '\r' || r == '\t' {
			sb.WriteRune(r)
		}
	}

	return sb.String()
}

// Truncate 返回最多 maxLen 个 rune 的截断版本。
// 正确处理多字节 Unicode 字符。
// 如果字符串被截断，会追加 "..." 以指示截断。
func Truncate(s string, maxLen int) string {
	// 如果未截断标志激活，返回完整字符串
	if disableTruncation.Load() {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	// 为 "..." 保留 3 个字符
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// DerefStr 解引用字符串指针，
// 如果指针为 nil 则返回回退值。
func DerefStr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}
