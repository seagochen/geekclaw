package agent

import "strings"

// ThinkingLevel 控制提供商如何发送思考参数。
//
//   - "adaptive"：发送 {thinking: {type: "adaptive"}} + output_config.effort（Claude 4.6+）
//   - "low"/"medium"/"high"/"xhigh"：发送 {thinking: {type: "enabled", budget_tokens: N}}（所有模型）
//   - "off"：禁用思考
type ThinkingLevel string

const (
	// ThinkingOff 禁用思考功能。
	ThinkingOff ThinkingLevel = "off"
	// ThinkingLow 低级别思考。
	ThinkingLow ThinkingLevel = "low"
	// ThinkingMedium 中级别思考。
	ThinkingMedium ThinkingLevel = "medium"
	// ThinkingHigh 高级别思考。
	ThinkingHigh ThinkingLevel = "high"
	// ThinkingXHigh 超高级别思考。
	ThinkingXHigh ThinkingLevel = "xhigh"
	// ThinkingAdaptive 自适应思考（Claude 4.6+）。
	ThinkingAdaptive ThinkingLevel = "adaptive"
)

// parseThinkingLevel 将配置字符串标准化为 ThinkingLevel。
// 不区分大小写，且对空白字符宽容，适用于面向用户的配置值。
// 对于未知或空值返回 ThinkingOff。
func parseThinkingLevel(level string) ThinkingLevel {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "adaptive":
		return ThinkingAdaptive
	case "low":
		return ThinkingLow
	case "medium":
		return ThinkingMedium
	case "high":
		return ThinkingHigh
	case "xhigh":
		return ThinkingXHigh
	default:
		return ThinkingOff
	}
}
