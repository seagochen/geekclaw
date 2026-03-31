package providers

import (
	"context"
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/providers/protocoltypes"
)

type (
	ToolCall               = protocoltypes.ToolCall
	FunctionCall           = protocoltypes.FunctionCall
	LLMResponse            = protocoltypes.LLMResponse
	UsageInfo              = protocoltypes.UsageInfo
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	ExtraContent           = protocoltypes.ExtraContent
	GoogleExtra            = protocoltypes.GoogleExtra
	ContentBlock           = protocoltypes.ContentBlock
	CacheControl           = protocoltypes.CacheControl
)

// LLMProvider 定义了大语言模型提供者的通用接口。
type LLMProvider interface {
	Chat(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
	) (*LLMResponse, error)
	GetDefaultModel() string
}

// StatefulProvider 是带有状态的提供者接口，需要在使用完毕后关闭。
type StatefulProvider interface {
	LLMProvider
	Close()
}

// ThinkingCapable 是一个可选接口，用于支持扩展思考功能的提供者（如 Anthropic）。
// 当配置了 thinking_level 但当前提供者不支持时，代理循环会发出警告。
type ThinkingCapable interface {
	SupportsThinking() bool
}

// FailoverReason 对 LLM 请求失败的原因进行分类，用于故障转移决策。
type FailoverReason string

const (
	FailoverAuth       FailoverReason = "auth"
	FailoverRateLimit  FailoverReason = "rate_limit"
	FailoverBilling    FailoverReason = "billing"
	FailoverTimeout    FailoverReason = "timeout"
	FailoverFormat     FailoverReason = "format"
	FailoverOverloaded FailoverReason = "overloaded"
	FailoverUnknown    FailoverReason = "unknown"
)

// FailoverError 将 LLM 提供者的错误封装并附加分类元数据。
type FailoverError struct {
	Reason   FailoverReason
	Provider string
	Model    string
	Status   int
	Wrapped  error
}

func (e *FailoverError) Error() string {
	return fmt.Sprintf("failover(%s): provider=%s model=%s status=%d: %v",
		e.Reason, e.Provider, e.Model, e.Status, e.Wrapped)
}

func (e *FailoverError) Unwrap() error {
	return e.Wrapped
}

// IsRetriable 返回 true 表示该错误应触发故障转移到下一个候选者。
// 不可重试的：格式错误（请求结构错误、图片尺寸/大小问题）。
func (e *FailoverError) IsRetriable() bool {
	return e.Reason != FailoverFormat
}

// ModelConfig 保存主模型和备选模型列表。
type ModelConfig struct {
	Primary   string
	Fallbacks []string
}
