package session

import (
	"context"
	"log"

	"github.com/seagosoft/geekclaw/geekclaw/memory"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// JSONLBackend 将 memory.Store 适配为 SessionStore 接口。
// 写入错误会被记录日志而非返回，符合智能体循环所依赖的
// SessionManager 的即发即忘约定。
type JSONLBackend struct {
	store memory.Store
}

// NewJSONLBackend 将 memory.Store 包装为 SessionStore 使用。
func NewJSONLBackend(store memory.Store) *JSONLBackend {
	return &JSONLBackend{store: store}
}

// AddMessage 向会话添加一条简单的角色/内容消息。
func (b *JSONLBackend) AddMessage(sessionKey, role, content string) {
	if err := b.store.AddMessage(context.Background(), sessionKey, role, content); err != nil {
		log.Printf("session: add message: %v", err)
	}
}

// AddFullMessage 向会话添加一条包含工具调用信息的完整消息。
func (b *JSONLBackend) AddFullMessage(sessionKey string, msg providers.Message) {
	if err := b.store.AddFullMessage(context.Background(), sessionKey, msg); err != nil {
		log.Printf("session: add full message: %v", err)
	}
}

// GetHistory 返回会话的完整消息历史记录。
func (b *JSONLBackend) GetHistory(key string) []providers.Message {
	msgs, err := b.store.GetHistory(context.Background(), key)
	if err != nil {
		log.Printf("session: get history: %v", err)
		return []providers.Message{}
	}
	return msgs
}

// GetSummary 返回会话的对话摘要，如果不存在则返回空字符串。
func (b *JSONLBackend) GetSummary(key string) string {
	summary, err := b.store.GetSummary(context.Background(), key)
	if err != nil {
		log.Printf("session: get summary: %v", err)
		return ""
	}
	return summary
}

// SetSummary 替换会话的对话摘要。
func (b *JSONLBackend) SetSummary(key, summary string) {
	if err := b.store.SetSummary(context.Background(), key, summary); err != nil {
		log.Printf("session: set summary: %v", err)
	}
}

// SetHistory 替换会话的完整消息历史记录。
func (b *JSONLBackend) SetHistory(key string, history []providers.Message) {
	if err := b.store.SetHistory(context.Background(), key, history); err != nil {
		log.Printf("session: set history: %v", err)
	}
}

// TruncateHistory 仅保留最后 keepLast 条消息。
func (b *JSONLBackend) TruncateHistory(key string, keepLast int) {
	if err := b.store.TruncateHistory(context.Background(), key, keepLast); err != nil {
		log.Printf("session: truncate history: %v", err)
	}
}

// Save 持久化会话状态。由于 JSONL 存储在每次写入时都会立即 fsync，
// 数据已经是持久的。Save 执行压缩以回收被逻辑截断的消息所占空间
// （当没有需要压缩的内容时为空操作）。
func (b *JSONLBackend) Save(key string) error {
	return b.store.Compact(context.Background(), key)
}

// Close 释放底层存储持有的资源。
func (b *JSONLBackend) Close() error {
	return b.store.Close()
}
