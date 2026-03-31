package session

import "github.com/seagosoft/geekclaw/geekclaw/providers"

// SessionStore 定义智能体循环使用的持久化操作接口。
// SessionManager（旧版 JSON 后端）和 JSONLBackend 都实现了此接口，
// 允许在不修改智能体循环代码的情况下替换存储层。
//
// 写入方法（Add*、Set*、Truncate*）是即发即忘的：不返回错误。
// 实现应在内部记录失败日志。这与智能体循环所依赖的
// 原始 SessionManager 约定一致。
type SessionStore interface {
	// AddMessage 向会话追加一条简单的角色/内容消息。
	AddMessage(sessionKey, role, content string)
	// AddFullMessage 向会话追加一条包含工具调用的完整消息。
	AddFullMessage(sessionKey string, msg providers.Message)
	// GetHistory 返回会话的完整消息历史记录。
	GetHistory(key string) []providers.Message
	// GetSummary 返回对话摘要，如果不存在则返回空字符串。
	GetSummary(key string) string
	// SetSummary 替换对话摘要。
	SetSummary(key, summary string)
	// SetHistory 替换完整的消息历史记录。
	SetHistory(key string, history []providers.Message)
	// TruncateHistory 仅保留最后 keepLast 条消息。
	TruncateHistory(key string, keepLast int)
	// Save 将待处理状态持久化到持久存储。
	Save(key string) error
	// Close 释放存储持有的资源。
	Close() error
}
