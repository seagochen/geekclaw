package config

import "strings"

// IsAntigravityModel 检查模型字符串是否属于 antigravity 提供者。
// 识别以下格式：
//   - "antigravity"
//   - "google-antigravity"
//   - "antigravity/..."
//   - "google-antigravity/..."
func IsAntigravityModel(model string) bool {
	return model == "antigravity" ||
		model == "google-antigravity" ||
		strings.HasPrefix(model, "antigravity/") ||
		strings.HasPrefix(model, "google-antigravity/")
}

// IsOpenAIModel 检查模型字符串是否属于 openai 提供者。
// 识别以下格式：
//   - "openai"
//   - "openai/..."
func IsOpenAIModel(model string) bool {
	return model == "openai" ||
		strings.HasPrefix(model, "openai/")
}

// IsAnthropicModel 检查模型字符串是否属于 anthropic 提供者。
// 识别以下格式：
//   - "anthropic"
//   - "anthropic/..."
func IsAnthropicModel(model string) bool {
	return model == "anthropic" ||
		strings.HasPrefix(model, "anthropic/")
}

// GetProviderFromModel 根据模型字符串返回提供者名称。
// 如果无法确定提供者，则返回空字符串。
func GetProviderFromModel(model string) string {
	switch {
	case IsAntigravityModel(model):
		return "google-antigravity"
	case IsOpenAIModel(model):
		return "openai"
	case IsAnthropicModel(model):
		return "anthropic"
	default:
		return ""
	}
}
