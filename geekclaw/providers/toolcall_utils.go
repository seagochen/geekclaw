// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package providers

import (
	"encoding/json"
)

// NormalizeToolCall 规范化 ToolCall 以确保所有字段都被正确填充。
// 处理 Name/Arguments 可能位于不同位置（顶层 vs Function）的情况，
// 并确保两者一致填充。
func NormalizeToolCall(tc ToolCall) ToolCall {
	normalized := tc

	// 如果未设置，从 Function 填充 Name
	if normalized.Name == "" && normalized.Function != nil {
		normalized.Name = normalized.Function.Name
	}

	// 确保 Arguments 不为 nil
	if normalized.Arguments == nil {
		normalized.Arguments = map[string]any{}
	}

	// 如果尚未设置，从 Function.Arguments 解析 Arguments
	if len(normalized.Arguments) == 0 && normalized.Function != nil && normalized.Function.Arguments != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(normalized.Function.Arguments), &parsed); err == nil && parsed != nil {
			normalized.Arguments = parsed
		}
	}

	// 确保 Function 以一致的值填充
	argsJSON, _ := json.Marshal(normalized.Arguments)
	if normalized.Function == nil {
		normalized.Function = &FunctionCall{
			Name:      normalized.Name,
			Arguments: string(argsJSON),
		}
	} else {
		if normalized.Function.Name == "" {
			normalized.Function.Name = normalized.Name
		}
		if normalized.Name == "" {
			normalized.Name = normalized.Function.Name
		}
		if normalized.Function.Arguments == "" {
			normalized.Function.Arguments = string(argsJSON)
		}
	}

	return normalized
}
