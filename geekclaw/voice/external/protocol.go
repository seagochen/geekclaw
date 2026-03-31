// Package external 实现通过 stdio 上的 JSON-RPC 通信的
// 外部语音转录插件桥接。
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

// NewTransport 是 plugin.NewTransport 的别名。
var NewTransport = plugin.NewTransport

// PluginConfig 是通用插件配置的别名。
type PluginConfig = plugin.Config

// --------------------------------------------------------------------------
// 方法名称
// --------------------------------------------------------------------------

const (
	// 生命周期（geekclaw -> 插件）
	MethodInitialize = "transcribe.initialize"
	MethodStop       = "transcribe.stop"

	// 执行（geekclaw -> 插件）
	MethodTranscribe = "transcribe.execute"

	// 入站（插件 -> geekclaw，通知）
	MethodLog = "transcribe.log"
)

// --------------------------------------------------------------------------
// 生命周期参数/结果
// --------------------------------------------------------------------------

// InitializeParams 在启动转录插件进程后发送一次。
type InitializeParams struct {
	Config map[string]any `json:"config,omitempty"` // 来自 YAML 的原始插件配置
}

// InitializeResult 是插件初始化后返回的结果。
type InitializeResult struct {
	Name           string   `json:"name"`                      // 提供者显示名称
	AudioFormats   []string `json:"audio_formats,omitempty"`   // 支持的格式（例如 ["ogg", "wav", "mp3"]）
}

// --------------------------------------------------------------------------
// 转录参数/结果
// --------------------------------------------------------------------------

// TranscribeParams 在 geekclaw 需要转录音频文件时发送。
type TranscribeParams struct {
	AudioFilePath string `json:"audio_file_path"` // 音频文件的绝对路径
}

// TranscribeResult 是转录请求的响应。
type TranscribeResult struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

// --------------------------------------------------------------------------
// 日志通知
// --------------------------------------------------------------------------

// LogParams 对应 transcribe.log 通知。
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

// ParseTranscribeResult 将原始 JSON-RPC 结果反序列化为 TranscribeResult。
func ParseTranscribeResult(raw json.RawMessage) (*TranscribeResult, error) {
	var result TranscribeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
