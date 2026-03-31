// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

// Package external 实现了 ExternalChannel 桥接，允许频道实现作为
// 独立进程运行，通过 stdio 上的 JSON-RPC 通信。这使得频道可以用
// 任何语言（Python、JS 等）编写，无需编译到主二进制文件中。
package external

import (
	"github.com/seagosoft/geekclaw/geekclaw/plugin"
)

// --------------------------------------------------------------------------
// JSON-RPC 2.0 线路类型——从 pkg/plugin 重新导出
// --------------------------------------------------------------------------

type (
	Request      = plugin.Request
	Response     = plugin.Response
	Notification = plugin.Notification
	RPCError     = plugin.RPCError
	Transport    = plugin.Transport
)

var NewTransport = plugin.NewTransport

// --------------------------------------------------------------------------
// 方法名称
// --------------------------------------------------------------------------

const (
	// 生命周期（geekclaw → 频道）
	MethodInitialize = "channel.initialize"
	MethodStart      = "channel.start"
	MethodStop       = "channel.stop"

	// 出站（geekclaw → 频道）
	MethodSend             = "channel.send"
	MethodSendMedia        = "channel.send_media"
	MethodStartTyping      = "channel.start_typing"
	MethodStopTyping       = "channel.stop_typing"
	MethodEditMessage      = "channel.edit_message"
	MethodReact            = "channel.react"
	MethodUndoReact        = "channel.undo_react"
	MethodSendPlaceholder  = "channel.send_placeholder"
	MethodRegisterCommands = "channel.register_commands"

	// 入站（频道 → geekclaw，通知）
	MethodMessage = "channel.message"
	MethodLog     = "channel.log"
)

// --------------------------------------------------------------------------
// 生命周期参数 / 结果
// --------------------------------------------------------------------------

// InitializeParams 在启动频道进程后发送一次。
type InitializeParams struct {
	Name   string         `json:"name"`   // 配置中注册的频道名称
	Config map[string]any `json:"config"` // 来自 YAML 的原始频道配置
}

// InitializeResult 由频道在初始化后返回。
type InitializeResult struct {
	Capabilities []string `json:"capabilities"` // 例如 ["typing", "edit", "reaction", "placeholder", "media", "commands"]
	MaxMessageLength int  `json:"max_message_length,omitempty"` // 0 = 无限制
}

// --------------------------------------------------------------------------
// 出站参数（geekclaw → 频道）
// --------------------------------------------------------------------------

// SendParams 映射到 channel.send。
type SendParams struct {
	ChatID           string `json:"chat_id"`
	Content          string `json:"content"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}

// SendMediaParams 映射到 channel.send_media。
type SendMediaParams struct {
	ChatID string      `json:"chat_id"`
	Parts  []MediaPart `json:"parts"`
}

// MediaPart 描述单个媒体附件。
type MediaPart struct {
	Type        string `json:"type"`                   // "image" | "audio" | "video" | "file"
	Data        string `json:"data"`                   // base64 编码的内容
	Caption     string `json:"caption,omitempty"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"` // MIME 类型
}

// TypingParams 映射到 channel.start_typing / channel.stop_typing。
type TypingParams struct {
	ChatID string `json:"chat_id"`
	StopID string `json:"stop_id,omitempty"` // 仅用于 stop_typing
}

// TypingResult 由 channel.start_typing 返回。
type TypingResult struct {
	StopID string `json:"stop_id"`
}

// EditMessageParams 映射到 channel.edit_message。
type EditMessageParams struct {
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}

// ReactParams 映射到 channel.react / channel.undo_react。
type ReactParams struct {
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id,omitempty"` // 仅用于 react
	UndoID    string `json:"undo_id,omitempty"`    // 仅用于 undo_react
}

// ReactResult 由 channel.react 返回。
type ReactResult struct {
	UndoID string `json:"undo_id"`
}

// PlaceholderParams 映射到 channel.send_placeholder。
type PlaceholderParams struct {
	ChatID string `json:"chat_id"`
}

// PlaceholderResult 由 channel.send_placeholder 返回。
type PlaceholderResult struct {
	MessageID string `json:"message_id"`
}

// CommandDef 是 channel.register_commands 的单个命令定义。
type CommandDef struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// RegisterCommandsParams 映射到 channel.register_commands。
type RegisterCommandsParams struct {
	Commands []CommandDef `json:"commands"`
}

// --------------------------------------------------------------------------
// 入站参数（频道 → geekclaw，通知）
// --------------------------------------------------------------------------

// InboundMessageParams 映射到 channel.message 通知。
type InboundMessageParams struct {
	SenderID    string            `json:"sender_id"`
	ChatID      string            `json:"chat_id"`
	Content     string            `json:"content"`
	MessageID   string            `json:"message_id,omitempty"`
	Media       []string          `json:"media,omitempty"` // data URL 或文件路径
	PeerKind    string            `json:"peer_kind,omitempty"` // "direct" | "group"
	PeerID      string            `json:"peer_id,omitempty"`
	Platform    string            `json:"platform,omitempty"`
	PlatformID  string            `json:"platform_id,omitempty"`
	Username    string            `json:"username,omitempty"`
	DisplayName string            `json:"display_name,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// LogParams 映射到 channel.log 通知。
type LogParams struct {
	Level   string `json:"level"` // "debug", "info", "warn", "error"
	Message string `json:"message"`
}

// --------------------------------------------------------------------------
// 能力常量
// --------------------------------------------------------------------------

const (
	CapTyping      = "typing"
	CapEdit        = "edit"
	CapReaction    = "reaction"
	CapPlaceholder = "placeholder"
	CapMedia       = "media"
	CapCommands    = "commands"
)
