package tools

import "encoding/json"

// ToolResult 表示工具执行的结构化返回值。
// 它为不同类型的结果提供清晰的语义，并支持
// 异步操作、面向用户的消息和错误处理。
type ToolResult struct {
	// ForLLM 是发送给 LLM 的上下文内容。
	// 所有结果都必须包含此字段。
	ForLLM string `json:"for_llm"`

	// ForUser 是直接发送给用户的内容。
	// 如果为空，则不发送用户消息。
	// Silent=true 会覆盖此字段。
	ForUser string `json:"for_user,omitempty"`

	// Silent 抑制向用户发送任何消息。
	// 为 true 时，即使 ForUser 已设置也会被忽略。
	Silent bool `json:"silent"`

	// IsError 指示工具执行是否失败。
	// 为 true 时，结果应被视为错误。
	IsError bool `json:"is_error"`

	// Async 指示工具是否正在异步运行。
	// 为 true 时，工具将稍后完成并通过回调通知。
	Async bool `json:"async"`

	// Err 是底层错误（不进行 JSON 序列化）。
	// 用于内部错误处理和日志记录。
	Err error `json:"-"`

	// Media 包含此工具生成的媒体存储引用。
	// 非空时，代理将以 OutboundMediaMessage 形式发布这些引用。
	Media []string `json:"media,omitempty"`
}

// NewToolResult 创建一个包含 LLM 内容的基本 ToolResult。
// 当你需要一个具有默认行为的简单结果时使用此函数。
//
// 示例：
//
//	result := NewToolResult("File updated successfully")
func NewToolResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM: forLLM,
	}
}

// SilentResult 创建一个静默的 ToolResult（不发送用户消息）。
// 内容仅发送给 LLM 作为上下文。
//
// 用于不应打扰用户的操作，例如：
// - 文件读取/写入
// - 状态更新
// - 后台操作
//
// 示例：
//
//	result := SilentResult("Config file saved")
func SilentResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM:  forLLM,
		Silent:  true,
		IsError: false,
		Async:   false,
	}
}

// AsyncResult 创建用于异步操作的 ToolResult。
// 任务将在后台运行并稍后完成。
//
// 用于长时间运行的操作，例如：
// - 子代理生成
// - 后台处理
// - 带回调的外部 API 调用
//
// 示例：
//
//	result := AsyncResult("Subagent spawned, will report back")
func AsyncResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM:  forLLM,
		Silent:  false,
		IsError: false,
		Async:   true,
	}
}

// ErrorResult 创建一个表示错误的 ToolResult。
// 设置 IsError=true 并包含错误消息。
//
// 示例：
//
//	result := ErrorResult("Failed to connect to database: connection refused")
func ErrorResult(message string) *ToolResult {
	return &ToolResult{
		ForLLM:  message,
		Silent:  false,
		IsError: true,
		Async:   false,
	}
}

// UserErrorResult 创建一个对 LLM 和用户都可见的错误 ToolResult。
// 设置 IsError=true，并用相同内容填充 ForLLM 和 ForUser。
//
// 当用户也必须看到错误时使用此函数（例如，失败时的命令输出）。
//
// 示例：
//
//	result := UserErrorResult("Command failed: permission denied")
func UserErrorResult(content string) *ToolResult {
	return &ToolResult{
		ForLLM:  content,
		ForUser: content,
		IsError: true,
	}
}

// UserResult 创建一个同时包含 LLM 和用户内容的 ToolResult。
// ForLLM 和 ForUser 设置为相同内容。
//
// 当用户需要直接看到结果时使用：
// - 命令执行输出
// - 获取的网页内容
// - 查询结果
//
// 示例：
//
//	result := UserResult("Total files found: 42")
func UserResult(content string) *ToolResult {
	return &ToolResult{
		ForLLM:  content,
		ForUser: content,
		Silent:  false,
		IsError: false,
		Async:   false,
	}
}

// MediaResult 创建一个包含媒体引用的 ToolResult。
// 代理将以 OutboundMediaMessage 形式发布这些引用。
//
// 示例：
//
//	result := MediaResult("Image generated successfully", []string{"media://abc123"})
func MediaResult(forLLM string, mediaRefs []string) *ToolResult {
	return &ToolResult{
		ForLLM: forLLM,
		Media:  mediaRefs,
	}
}

// MarshalJSON 实现自定义 JSON 序列化。
// Err 字段通过 json:"-" 标签从 JSON 输出中排除。
func (tr *ToolResult) MarshalJSON() ([]byte, error) {
	type Alias ToolResult
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(tr),
	})
}

// WithError 设置 Err 字段并返回结果以支持链式调用。
// 这保留了错误用于日志记录，同时将其排除在 JSON 之外。
//
// 示例：
//
//	result := ErrorResult("Operation failed").WithError(err)
func (tr *ToolResult) WithError(err error) *ToolResult {
	tr.Err = err
	return tr
}
