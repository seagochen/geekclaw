// Package protocoltypes 定义了 LLM 提供者之间共享的协议类型。
package protocoltypes

// ToolCall 表示 LLM 响应中的一个工具调用。
type ToolCall struct {
	ID               string         `json:"id"`
	Type             string         `json:"type,omitempty"`
	Function         *FunctionCall  `json:"function,omitempty"`
	Name             string         `json:"-"`
	Arguments        map[string]any `json:"-"`
	ThoughtSignature string         `json:"-"` // 仅供内部使用
	ExtraContent     *ExtraContent  `json:"extra_content,omitempty"`
}

// ExtraContent 包含提供者特有的额外内容。
type ExtraContent struct {
	Google *GoogleExtra `json:"google,omitempty"`
}

// GoogleExtra 包含 Google/Gemini 特有的额外数据。
type GoogleExtra struct {
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

// FunctionCall 表示工具调用中的函数调用详情。
type FunctionCall struct {
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

// LLMResponse 表示 LLM 提供者的通用响应结构。
type LLMResponse struct {
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	FinishReason     string            `json:"finish_reason"`
	Usage            *UsageInfo        `json:"usage,omitempty"`
	Reasoning        string            `json:"reasoning"`
	ReasoningDetails []ReasoningDetail `json:"reasoning_details"`
}

// ReasoningDetail 表示推理过程中的一个详情块。
type ReasoningDetail struct {
	Format string `json:"format"`
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Text   string `json:"text"`
}

// UsageInfo 包含 token 使用统计信息。
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CacheControl 标记内容块以启用 LLM 端的前缀缓存。
// 目前仅支持 "ephemeral"（由 Anthropic 使用）。
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ContentBlock 表示系统消息的结构化片段。
// 理解 SystemParts 的适配器可以使用这些块来设置
// 每个块的缓存控制（如 Anthropic 的 cache_control: ephemeral）。
type ContentBlock struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// Message 表示对话中的一条消息。
type Message struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	Media            []string       `json:"media,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	SystemParts      []ContentBlock `json:"system_parts,omitempty"` // 用于缓存感知适配器的结构化系统块
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
}

// ToolDefinition 定义一个可供 LLM 使用的工具。
type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

// ToolFunctionDefinition 定义工具的函数签名，包括名称、描述和参数。
type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}
