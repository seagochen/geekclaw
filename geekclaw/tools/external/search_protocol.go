// Package external 实现了通过 stdio 上的 JSON-RPC 通信的
// 外部搜索提供者插件的桥接。
package external

import (
	"encoding/json"

	"github.com/seagosoft/geekclaw/geekclaw/plugin"
)

// 复用 pkg/plugin 中的 JSON-RPC 传输类型。
type (
	Request      = plugin.Request
	Response     = plugin.Response
	Notification = plugin.Notification
	RPCError     = plugin.RPCError
	Transport    = plugin.Transport
)

// NewTransport 是 plugin.NewTransport 的别名。
var NewTransport = plugin.NewTransport

// PluginConfig 是通用插件配置的别名。
type PluginConfig = plugin.Config

// --------------------------------------------------------------------------
// 方法名
// --------------------------------------------------------------------------

const (
	// 生命周期（geekclaw -> 插件）
	MethodInitialize = "search.initialize"
	MethodStop       = "search.stop"

	// 执行（geekclaw -> 插件）
	MethodSearch = "search.execute"

	// 入站（插件 -> geekclaw，通知）
	MethodLog = "search.log"
)

// --------------------------------------------------------------------------
// 生命周期参数 / 结果
// --------------------------------------------------------------------------

// InitializeParams 在生成搜索插件进程后发送一次。
type InitializeParams struct {
	Config map[string]any `json:"config,omitempty"` // 来自 YAML 的原始插件配置
}

// InitializeResult 由插件在初始化后返回。
type InitializeResult struct {
	Name string `json:"name"` // 提供者显示名称
}

// --------------------------------------------------------------------------
// 搜索参数 / 结果
// --------------------------------------------------------------------------

// SearchParams 在 geekclaw 需要执行网络搜索时发送。
type SearchParams struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

// SearchResult 是搜索请求的响应。
type SearchResult struct {
	Results string `json:"results"` // 预格式化的文本结果
}

// --------------------------------------------------------------------------
// 日志通知
// --------------------------------------------------------------------------

// LogParams 映射到 search.log 通知。
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

// ParseSearchResult 将原始 JSON-RPC 结果反序列化为 SearchResult。
func ParseSearchResult(raw json.RawMessage) (*SearchResult, error) {
	var result SearchResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
