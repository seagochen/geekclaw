// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

// GeekClaw 统一用户身份工具。
// 提供规范的 "platform:id" 格式和匹配逻辑，
// 与所有旧版允许列表格式向后兼容。

package channels

import (
	"strings"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
)

// BuildCanonicalID 构建规范的 "platform:id" 标识符。
// platform 和 platformID 都会被转为小写并去除空白。
func BuildCanonicalID(platform, platformID string) string {
	p := strings.ToLower(strings.TrimSpace(platform))
	id := strings.TrimSpace(platformID)
	if p == "" || id == "" {
		return ""
	}
	return p + ":" + id
}

// ParseCanonicalID 将规范 ID（"platform:id"）拆分为各部分。
// 如果输入不包含冒号分隔符则返回 ok=false。
func ParseCanonicalID(canonical string) (platform, id string, ok bool) {
	canonical = strings.TrimSpace(canonical)
	idx := strings.Index(canonical, ":")
	if idx <= 0 || idx == len(canonical)-1 {
		return "", "", false
	}
	return canonical[:idx], canonical[idx+1:], true
}

// MatchAllowed 检查给定发送者是否匹配单个允许列表条目。
// 与所有旧格式向后兼容：
//
//   - "123456"              → 匹配 sender.PlatformID
//   - "@alice"              → 匹配 sender.Username
//   - "123456|alice"        → 匹配 PlatformID 或 Username
//   - "telegram:123456"     → 精确匹配 sender.CanonicalID
func MatchAllowed(sender bus.SenderInfo, allowed string) bool {
	allowed = strings.TrimSpace(allowed)
	if allowed == "" {
		return false
	}

	// 首先尝试规范匹配："platform:id" 格式
	if platform, id, ok := ParseCanonicalID(allowed); ok {
		// 仅当平台部分看起来像已知平台名称时才作为规范格式处理
		// （纯数字字符串可能是复合 ID）
		if !isNumeric(platform) {
			candidate := BuildCanonicalID(platform, id)
			if candidate != "" && sender.CanonicalID != "" {
				return strings.EqualFold(sender.CanonicalID, candidate)
			}
			// 如果发送者没有规范 ID，尝试匹配 platform + platformID
			return strings.EqualFold(platform, sender.Platform) &&
				sender.PlatformID == id
		}
	}

	// 去除开头的 "@" 用于用户名匹配
	trimmed := strings.TrimPrefix(allowed, "@")

	// 拆分复合 "id|username" 格式
	allowedID := trimmed
	allowedUser := ""
	if idx := strings.Index(trimmed, "|"); idx > 0 {
		allowedID = trimmed[:idx]
		allowedUser = trimmed[idx+1:]
	}

	// 匹配 PlatformID
	if sender.PlatformID != "" && sender.PlatformID == allowedID {
		return true
	}

	// 匹配 Username
	if sender.Username != "" {
		if sender.Username == trimmed || sender.Username == allowedUser {
			return true
		}
	}

	// 将复合发送者格式与允许列表各部分进行匹配
	if allowedUser != "" && sender.PlatformID != "" && sender.PlatformID == allowedID {
		return true
	}
	if allowedUser != "" && sender.Username != "" && sender.Username == allowedUser {
		return true
	}

	return false
}

// isNumeric 返回 s 是否完全由数字组成。
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
