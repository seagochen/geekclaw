// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package channels

import (
	"context"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
)

// typingEntry 封装输入停止函数及其创建时间戳，用于 TTL 驱逐。
type typingEntry struct {
	stop      func()
	createdAt time.Time
}

// reactionEntry 封装反应撤销函数及其创建时间戳，用于 TTL 驱逐。
type reactionEntry struct {
	undo      func()
	createdAt time.Time
}

// placeholderEntry 封装占位符 ID 及其创建时间戳，用于 TTL 驱逐。
type placeholderEntry struct {
	id        string
	createdAt time.Time
}

// RecordPlaceholder 注册占位符消息以供后续编辑。
// 实现 PlaceholderRecorder 接口。
func (m *Manager) RecordPlaceholder(channel, chatID, placeholderID string) {
	key := channel + ":" + chatID
	m.placeholders.Store(key, placeholderEntry{id: placeholderID, createdAt: time.Now()})
}

// SendPlaceholder 为给定的频道/聊天 ID 发送"思考中…"占位符，
// 并记录以供后续编辑。如果成功发送则返回 true。
func (m *Manager) SendPlaceholder(ctx context.Context, channel, chatID string) bool {
	m.mu.RLock()
	ch, ok := m.channels[channel]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	pc, ok := ch.(PlaceholderCapable)
	if !ok {
		return false
	}
	phID, err := pc.SendPlaceholder(ctx, chatID)
	if err != nil || phID == "" {
		return false
	}
	m.RecordPlaceholder(channel, chatID, phID)
	return true
}

// RecordTypingStop 注册输入停止函数以供后续调用。
// 实现 PlaceholderRecorder 接口。
func (m *Manager) RecordTypingStop(channel, chatID string, stop func()) {
	key := channel + ":" + chatID
	m.typingStops.Store(key, typingEntry{stop: stop, createdAt: time.Now()})
}

// RecordReactionUndo 注册反应撤销函数以供后续调用。
// 实现 PlaceholderRecorder 接口。
func (m *Manager) RecordReactionUndo(channel, chatID string, undo func()) {
	key := channel + ":" + chatID
	m.reactionUndos.Store(key, reactionEntry{undo: undo, createdAt: time.Now()})
}

// preSend 在发送消息前处理停止输入、撤销反应和编辑占位符。
// 如果消息已编辑到占位符中（跳过 Send），则返回 true。
func (m *Manager) preSend(ctx context.Context, name string, msg bus.OutboundMessage, ch Channel) bool {
	key := name + ":" + msg.ChatID

	// 1. 停止输入指示器
	if v, loaded := m.typingStops.LoadAndDelete(key); loaded {
		if entry, ok := v.(typingEntry); ok {
			entry.stop() // 幂等，可安全调用
		}
	}

	// 2. 撤销反应
	if v, loaded := m.reactionUndos.LoadAndDelete(key); loaded {
		if entry, ok := v.(reactionEntry); ok {
			entry.undo() // 幂等，可安全调用
		}
	}

	// 3. 尝试编辑占位符
	if v, loaded := m.placeholders.LoadAndDelete(key); loaded {
		if entry, ok := v.(placeholderEntry); ok && entry.id != "" {
			if editor, ok := ch.(MessageEditor); ok {
				if err := editor.EditMessage(ctx, msg.ChatID, entry.id, msg.Content); err == nil {
					return true // 编辑成功，跳过 Send
				}
				// 编辑失败 → 回退到正常 Send
			}
		}
	}

	return false
}
