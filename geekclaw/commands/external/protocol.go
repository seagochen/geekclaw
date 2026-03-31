// Package external 实现了通过 JSON-RPC over stdio 通信的外部命令插件桥接。
package external

import (
	"encoding/json"

	"github.com/seagosoft/geekclaw/geekclaw/plugin"
)

// 复用 pkg/plugin 中的 JSON-RPC 线路类型。
type (
	Request      = plugin.Request
	Response     = plugin.Response
	Notification = plugin.Notification
	RPCError     = plugin.RPCError
	Transport    = plugin.Transport
)

// NewTransport 创建新的 JSON-RPC 传输层。
var NewTransport = plugin.NewTransport

// PluginConfig 是通用插件配置的别名。
type PluginConfig = plugin.Config

// --------------------------------------------------------------------------
// 方法名称
// --------------------------------------------------------------------------

const (
	// 生命周期（geekclaw → 插件）
	MethodInitialize = "command.initialize"
	MethodStop       = "command.stop"

	// 执行（geekclaw → 插件）
	MethodExecute = "command.execute"

	// 入站（插件 → geekclaw，通知）
	MethodLog = "command.log"
)

// --------------------------------------------------------------------------
// 生命周期参数/结果
// --------------------------------------------------------------------------

// InitializeParams 在启动命令插件进程后发送一次。
type InitializeParams struct {
	Config map[string]any `json:"config,omitempty"` // 来自 YAML 的原始插件配置
}

// InitializeResult 是插件初始化后返回的结果。
type InitializeResult struct {
	Commands []CommandDef `json:"commands"` // 此插件提供的命令
}

// CommandDef 描述来自插件的命令定义。
type CommandDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Usage       string   `json:"usage,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// --------------------------------------------------------------------------
// 执行参数/结果
// --------------------------------------------------------------------------

// ExecuteParams 在调用插件命令时发送。
type ExecuteParams struct {
	Command  string         `json:"command"`            // 匹配到的命令名称
	Text     string         `json:"text"`               // 完整输入文本
	Channel  string         `json:"channel,omitempty"`  // 频道名称
	ChatID   string         `json:"chat_id,omitempty"`  // 聊天/房间 ID
	SenderID string         `json:"sender_id,omitempty"`
	Context  map[string]any `json:"context,omitempty"`  // 可选的运行时上下文
}

// ExecuteResult 是执行命令后的响应。
type ExecuteResult struct {
	Reply string `json:"reply"` // 回复给用户的文本
}

// --------------------------------------------------------------------------
// 日志通知
// --------------------------------------------------------------------------

// LogParams 对应 command.log 通知。
type LogParams struct {
	Level   string `json:"level"`   // "debug"、"info"、"warn"、"error"
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

// ParseExecuteResult 将原始 JSON-RPC 结果反序列化为 ExecuteResult。
func ParseExecuteResult(raw json.RawMessage) (*ExecuteResult, error) {
	var result ExecuteResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
