// GeekClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
)

// ToolLoopConfig 配置工具执行循环。
type ToolLoopConfig struct {
	Provider      providers.LLMProvider
	Model         string
	Tools         *ToolRegistry
	MaxIterations int
	LLMOptions    map[string]any
}

// ToolLoopResult 包含运行工具循环的结果。
type ToolLoopResult struct {
	Content    string
	Iterations int
}

// RunToolLoop 执行 LLM + 工具调用迭代循环。
// 这是核心代理逻辑，可被主代理和子代理复用。
func RunToolLoop(
	ctx context.Context,
	config ToolLoopConfig,
	messages []providers.Message,
	channel, chatID string,
) (*ToolLoopResult, error) {
	iteration := 0
	var finalContent string

	for iteration < config.MaxIterations {
		iteration++

		logger.DebugCF("toolloop", "LLM iteration",
			map[string]any{
				"iteration": iteration,
				"max":       config.MaxIterations,
			})

		// 1. 构建工具定义
		var providerToolDefs []providers.ToolDefinition
		if config.Tools != nil {
			providerToolDefs = config.Tools.ToProviderDefs()
		}

		// 2. 设置默认 LLM 选项
		llmOpts := config.LLMOptions
		if llmOpts == nil {
			llmOpts = map[string]any{}
		}
		// 3. 调用 LLM
		response, err := config.Provider.Chat(ctx, messages, providerToolDefs, config.Model, llmOpts)
		if err != nil {
			logger.ErrorCF("toolloop", "LLM call failed",
				map[string]any{
					"iteration": iteration,
					"error":     err.Error(),
				})
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		// 4. 如果没有工具调用，循环结束
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF("toolloop", "LLM response without tool calls (direct answer)",
				map[string]any{
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, providers.NormalizeToolCall(tc))
		}

		// 5. 记录工具调用
		toolNames := make([]string, 0, len(normalizedToolCalls))
		for _, tc := range normalizedToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("toolloop", "LLM requested tool calls",
			map[string]any{
				"tools":     toolNames,
				"count":     len(normalizedToolCalls),
				"iteration": iteration,
			})

		// 6. 构建包含工具调用的助手消息
		assistantMsg := providers.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:        tc.ID,
				Type:      "function",
				Name:      tc.Name,
				Arguments: tc.Arguments,
				Function: &providers.FunctionCall{
					Name:      tc.Name,
					Arguments: string(argumentsJSON),
				},
			})
		}
		messages = append(messages, assistantMsg)

		// 7. 并行执行工具调用
		type indexedResult struct {
			result *ToolResult
			tc     providers.ToolCall
		}

		results := make([]indexedResult, len(normalizedToolCalls))
		var wg sync.WaitGroup

		for i, tc := range normalizedToolCalls {
			results[i].tc = tc

			wg.Add(1)
			go func(idx int, tc providers.ToolCall) {
				defer wg.Done()

				argsJSON, _ := json.Marshal(tc.Arguments)
				argsPreview := utils.Truncate(string(argsJSON), 200)
				logger.InfoCF("toolloop", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
					map[string]any{
						"tool":      tc.Name,
						"iteration": iteration,
					})

				var toolResult *ToolResult
				if config.Tools != nil {
					toolResult = config.Tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, channel, chatID, nil)
				} else {
					toolResult = ErrorResult("No tools available")
				}
				results[idx].result = toolResult
			}(i, tc)
		}
		wg.Wait()

		// 按原始顺序追加结果
		for _, r := range results {
			contentForLLM := r.result.ForLLM
			if contentForLLM == "" && r.result.Err != nil {
				contentForLLM = r.result.Err.Error()
			}

			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: r.tc.ID,
			})
		}
	}

	return &ToolLoopResult{
		Content:    finalContent,
		Iterations: iteration,
	}, nil
}
