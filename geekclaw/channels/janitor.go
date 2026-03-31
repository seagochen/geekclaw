// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package channels

import (
	"context"
	"time"
)

const (
	janitorInterval = 10 * time.Second
	typingStopTTL   = 5 * time.Minute
	placeholderTTL  = 10 * time.Minute
)

// runTTLJanitor 定期扫描 typingStops 和 placeholders 映射，
// 驱逐超过 TTL 的条目。防止在出站路径未触发 preSend 时
// （例如 LLM 错误）导致内存累积。
func (m *Manager) runTTLJanitor(ctx context.Context) {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.typingStops.Range(func(key, value any) bool {
				if entry, ok := value.(typingEntry); ok {
					if now.Sub(entry.createdAt) > typingStopTTL {
						if _, loaded := m.typingStops.LoadAndDelete(key); loaded {
							entry.stop() // 幂等，可安全调用
						}
					}
				}
				return true
			})
			m.reactionUndos.Range(func(key, value any) bool {
				if entry, ok := value.(reactionEntry); ok {
					if now.Sub(entry.createdAt) > typingStopTTL {
						if _, loaded := m.reactionUndos.LoadAndDelete(key); loaded {
							entry.undo() // 幂等，可安全调用
						}
					}
				}
				return true
			})
			m.placeholders.Range(func(key, value any) bool {
				if entry, ok := value.(placeholderEntry); ok {
					if now.Sub(entry.createdAt) > placeholderTTL {
						m.placeholders.Delete(key)
					}
				}
				return true
			})
		}
	}
}
