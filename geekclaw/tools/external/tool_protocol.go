// Package external 实现了通过 stdio 上的 JSON-RPC 通信的
// 外部工具插件的桥接。
package external

import "encoding/json"

// --------------------------------------------------------------------------
// 方法名
// --------------------------------------------------------------------------

const (
	// 生命周期（geekclaw -> 插件）
	MethodToolInitialize = "tool.initialize"
	MethodToolStop       = "tool.stop"

	// 执行（geekclaw -> 插件）
	MethodToolExecute = "tool.execute"

	// 入站（插件 -> geekclaw，通知）
	MethodToolLog = "tool.log"
)

// --------------------------------------------------------------------------
// 生命周期参数 / 结果
// --------------------------------------------------------------------------

// ToolInitializeParams 在生成工具插件进程后发送一次。
type ToolInitializeParams struct {
	Config map[string]any `json:"config,omitempty"` // 来自 YAML 的原始插件配置
}

// ToolDef 描述插件暴露的单个工具。
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolInitializeResult 由插件在初始化后返回。
type ToolInitializeResult struct {
	Tools []ToolDef `json:"tools"`
}

// --------------------------------------------------------------------------
// 执行参数 / 结果
// --------------------------------------------------------------------------

// ToolExecuteParams 在调用插件工具时发送。
type ToolExecuteParams struct {
	Name   string         `json:"name"`             // 要执行的工具名称
	Params map[string]any `json:"params,omitempty"` // 工具参数
}

// ToolExecuteResult 是执行工具的响应。
type ToolExecuteResult struct {
	Content string `json:"content"`          // 结果文本
	Error   bool   `json:"error,omitempty"`  // 执行失败时为 true
}

// --------------------------------------------------------------------------
// 日志通知
// --------------------------------------------------------------------------

// ToolLogParams 映射到 tool.log 通知。
type ToolLogParams struct {
	Level   string `json:"level"`   // "debug"、"info"、"warn"、"error"
	Message string `json:"message"`
}

// --------------------------------------------------------------------------
// 辅助函数
// --------------------------------------------------------------------------

// ParseToolInitializeResult 将原始 JSON-RPC 结果反序列化为 ToolInitializeResult。
func ParseToolInitializeResult(raw json.RawMessage) (*ToolInitializeResult, error) {
	var result ToolInitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ParseToolExecuteResult 将原始 JSON-RPC 结果反序列化为 ToolExecuteResult。
func ParseToolExecuteResult(raw json.RawMessage) (*ToolExecuteResult, error) {
	var result ToolExecuteResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
