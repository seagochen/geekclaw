// Package external 实现了通过 stdio 上的 JSON-RPC 通信的外部 LLM 提供者
// 插件的桥接层。
package external

import (
	"encoding/json"

	"github.com/seagosoft/geekclaw/geekclaw/plugin"
	"github.com/seagosoft/geekclaw/geekclaw/providers/protocoltypes"
)

// 复用 pkg/plugin 中的 JSON-RPC 传输类型。
type (
	Request      = plugin.Request
	Response     = plugin.Response
	Notification = plugin.Notification
	RPCError     = plugin.RPCError
	Transport    = plugin.Transport
)

var NewTransport = plugin.NewTransport

// PluginConfig 是通用插件配置的别名。
type PluginConfig = plugin.Config

// --------------------------------------------------------------------------
// 方法名称
// --------------------------------------------------------------------------

const (
	// 生命周期（geekclaw → 插件）
	MethodInitialize = "llm.initialize"
	MethodStop       = "llm.stop"

	// 执行（geekclaw → 插件）
	MethodChat = "llm.chat"

	// 入站（插件 → geekclaw，通知）
	MethodLog   = "llm.log"
	MethodToken = "llm.token" // 可选的流式 token 通知
)

// --------------------------------------------------------------------------
// 生命周期参数 / 结果
// --------------------------------------------------------------------------

// InitializeParams 在生成 LLM 插件进程后发送一次。
type InitializeParams struct {
	Config map[string]any `json:"config,omitempty"` // 来自 YAML 的原始插件配置
}

// InitializeResult 是插件初始化后返回的结果。
type InitializeResult struct {
	Name             string `json:"name"`                        // 提供者显示名称
	DefaultModel     string `json:"default_model,omitempty"`     // 默认模型标识符
	SupportsThinking bool   `json:"supports_thinking,omitempty"` // 提供者是否支持扩展思考
}

// --------------------------------------------------------------------------
// 对话参数 / 结果
// --------------------------------------------------------------------------

// ChatParams 在 geekclaw 需要执行 LLM 对话时发送。
type ChatParams struct {
	Messages []protocoltypes.Message        `json:"messages"`
	Tools    []protocoltypes.ToolDefinition `json:"tools,omitempty"`
	Model    string                         `json:"model"`
	Options  map[string]any                 `json:"options,omitempty"`
}

// ChatResult 是对话请求的响应。
// 直接映射到 protocoltypes.LLMResponse 的字段。
type ChatResult struct {
	Content          string                       `json:"content"`
	ReasoningContent string                       `json:"reasoning_content,omitempty"`
	ToolCalls        []protocoltypes.ToolCall      `json:"tool_calls,omitempty"`
	FinishReason     string                       `json:"finish_reason"`
	Usage            *protocoltypes.UsageInfo      `json:"usage,omitempty"`
	Reasoning        string                       `json:"reasoning,omitempty"`
	ReasoningDetails []protocoltypes.ReasoningDetail `json:"reasoning_details,omitempty"`
}

// TokenNotification 是插件在生成 token 时发送的可选流式通知。
// Go 桥接层会在存在时累积这些通知。
type TokenNotification struct {
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Done             bool   `json:"done,omitempty"` // 流式传输完成时为 true
}

// --------------------------------------------------------------------------
// 日志通知
// --------------------------------------------------------------------------

// LogParams 映射到 llm.log 通知。
type LogParams struct {
	Level   string `json:"level"`   // "debug", "info", "warn", "error"
	Message string `json:"message"`
}

// --------------------------------------------------------------------------
// 辅助函数
// --------------------------------------------------------------------------

// ParseInitializeResult 将原始 JSON-RPC 结果反序列化为 InitializeResult。
func ParseInitializeResult(raw json.RawMessage) (*InitializeResult, error) {
	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ParseChatResult 将原始 JSON-RPC 结果反序列化为 ChatResult。
func ParseChatResult(raw json.RawMessage) (*ChatResult, error) {
	var result ChatResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ToLLMResponse 将 ChatResult 转换为 protocoltypes.LLMResponse。
func (r *ChatResult) ToLLMResponse() *protocoltypes.LLMResponse {
	resp := &protocoltypes.LLMResponse{
		Content:          r.Content,
		ReasoningContent: r.ReasoningContent,
		FinishReason:     r.FinishReason,
		Usage:            r.Usage,
		Reasoning:        r.Reasoning,
		ReasoningDetails: r.ReasoningDetails,
	}
	if len(r.ToolCalls) > 0 {
		resp.ToolCalls = r.ToolCalls
		// 从每个工具调用的 FunctionCall 中解析函数名称/参数
		for i := range resp.ToolCalls {
			tc := &resp.ToolCalls[i]
			if tc.Function != nil {
				tc.Name = tc.Function.Name
				if tc.Function.Arguments != "" {
					var args map[string]any
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
						tc.Arguments = args
					}
				}
			}
		}
	}
	return resp
}
