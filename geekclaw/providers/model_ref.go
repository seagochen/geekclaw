package providers

import "strings"

// ModelRef 表示已解析的模型引用，包含提供者和模型名称。
type ModelRef struct {
	Provider string
	Model    string
}

// ParseModelRef 将 "anthropic/claude-opus" 解析为 {Provider: "anthropic", Model: "claude-opus"}。
// 如果没有斜杠，则使用 defaultProvider。
// 输入为空时返回 nil。
func ParseModelRef(raw string, defaultProvider string) *ModelRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	if idx := strings.Index(raw, "/"); idx > 0 {
		provider := NormalizeProvider(raw[:idx])
		model := strings.TrimSpace(raw[idx+1:])
		if model == "" {
			return nil
		}
		return &ModelRef{Provider: provider, Model: model}
	}

	return &ModelRef{
		Provider: NormalizeProvider(defaultProvider),
		Model:    raw,
	}
}

// NormalizeProvider 将提供者标识符规范化为标准形式。
func NormalizeProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))

	switch p {
	case "z.ai", "z-ai":
		return "zai"
	case "opencode-zen":
		return "opencode"
	case "qwen":
		return "qwen-portal"
	case "kimi-code":
		return "kimi-coding"
	case "gpt":
		return "openai"
	case "claude":
		return "anthropic"
	case "glm":
		return "zhipu"
	case "google":
		return "gemini"
	}

	return p
}

// ModelKey 返回用于去重的标准 "provider/model" 键。
func ModelKey(provider, model string) string {
	return NormalizeProvider(provider) + "/" + strings.ToLower(strings.TrimSpace(model))
}
