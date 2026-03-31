package memory

import (
	"context"

	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// Store 定义会话持久化存储的接口。
// 每个方法都是原子操作 — 没有单独的 Save() 调用。
type Store interface {
	// AddMessage 向会话追加一条简单文本消息。
	AddMessage(ctx context.Context, sessionKey, role, content string) error

	// AddFullMessage 向会话追加一条完整消息（包含工具调用等）。
	AddFullMessage(ctx context.Context, sessionKey string, msg providers.Message) error

	// GetHistory 返回会话的所有消息，按插入顺序排列。
	// 如果会话不存在，返回空切片（非 nil）。
	GetHistory(ctx context.Context, sessionKey string) ([]providers.Message, error)

	// GetSummary 返回会话的对话摘要。
	// 如果不存在摘要，返回空字符串。
	GetSummary(ctx context.Context, sessionKey string) (string, error)

	// SetSummary 更新会话的对话摘要。
	SetSummary(ctx context.Context, sessionKey, summary string) error

	// TruncateHistory 移除除最后 keepLast 条之外的所有消息。
	// 如果 keepLast <= 0，则移除所有消息。
	TruncateHistory(ctx context.Context, sessionKey string, keepLast int) error

	// SetHistory 用提供的历史记录替换会话中的所有消息。
	SetHistory(ctx context.Context, sessionKey string, history []providers.Message) error

	// Compact 通过物理移除逻辑上已截断的数据来回收存储空间。
	// 不累积死数据的后端可以返回 nil。
	Compact(ctx context.Context, sessionKey string) error

	// Close 释放存储持有的所有资源。
	Close() error
}
