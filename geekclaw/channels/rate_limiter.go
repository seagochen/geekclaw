// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package channels

import (
	"context"
	"errors"
	"math"
	"time"

	"golang.org/x/time/rate"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

const (
	defaultChannelQueueSize = 16
	defaultRateLimit        = 10 // 默认 10 条消息/秒
	maxRetries              = 3
	rateLimitDelay          = 1 * time.Second
	baseBackoff             = 500 * time.Millisecond
	maxBackoff              = 8 * time.Second
)

// channelRateConfig 将频道名称映射到每秒速率限制。
var channelRateConfig = map[string]float64{
	"telegram": 20,
	"discord":  1,
	"slack":    1,
	"matrix":   2,
	"line":     10,
	"qq":       5,
	"irc":      2,
}

// newChannelWorker 创建一个为指定频道名称配置了速率限制器的 channelWorker。
func newChannelWorker(name string, ch Channel) *channelWorker {
	rateVal := float64(defaultRateLimit)
	if r, ok := channelRateConfig[name]; ok {
		rateVal = r
	}
	burst := int(math.Max(1, math.Ceil(rateVal/2)))

	return &channelWorker{
		ch:         ch,
		queue:      make(chan bus.OutboundMessage, defaultChannelQueueSize),
		mediaQueue: make(chan bus.OutboundMediaMessage, defaultChannelQueueSize),
		done:       make(chan struct{}),
		mediaDone:  make(chan struct{}),
		limiter:    rate.NewLimiter(rate.Limit(rateVal), burst),
	}
}

// retryWithBackoff 使用指数退避重试逻辑执行 fn。
// 通过分类错误来确定重试策略：
//   - ErrNotRunning / ErrSendFailed：永久性错误，不重试
//   - ErrRateLimit：固定延迟重试
//   - ErrTemporary / 未知错误：指数退避重试
//
// 成功时返回 nil，或在所有重试耗尽后返回最后一个错误。
func retryWithBackoff(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// 永久性故障——不重试
		if errors.Is(lastErr, ErrNotRunning) || errors.Is(lastErr, ErrSendFailed) {
			return lastErr
		}

		// 最后一次尝试已耗尽——不等待
		if attempt == maxRetries {
			break
		}

		// 速率限制错误——固定延迟
		if errors.Is(lastErr, ErrRateLimit) {
			select {
			case <-time.After(rateLimitDelay):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// ErrTemporary 或未知错误——指数退避
		backoff := min(time.Duration(float64(baseBackoff)*math.Pow(2, float64(attempt))), maxBackoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

// sendWithRetry 通过频道发送消息，附带速率限制和重试逻辑。
func (m *Manager) sendWithRetry(ctx context.Context, name string, w *channelWorker, msg bus.OutboundMessage) {
	// 速率限制：等待令牌
	if err := w.limiter.Wait(ctx); err != nil {
		// ctx 已取消，正在关闭
		return
	}

	// 发送前：停止输入指示器并尝试编辑占位符
	if m.preSend(ctx, name, msg, w.ch) {
		return // 占位符编辑成功，跳过 Send
	}

	if err := retryWithBackoff(ctx, func() error {
		return w.ch.Send(ctx, msg)
	}); err != nil {
		logger.ErrorCF("channels", "Send failed", map[string]any{
			"channel": name,
			"chat_id": msg.ChatID,
			"error":   err.Error(),
			"retries": maxRetries,
		})
	}
}

// sendMediaWithRetry 通过频道发送媒体消息，附带速率限制和重试逻辑。
// 如果频道未实现 MediaSender，则静默跳过。
func (m *Manager) sendMediaWithRetry(ctx context.Context, name string, w *channelWorker, msg bus.OutboundMediaMessage) {
	ms, ok := w.ch.(MediaSender)
	if !ok {
		logger.DebugCF("channels", "Channel does not support MediaSender, skipping media", map[string]any{
			"channel": name,
		})
		return
	}

	// 速率限制：等待令牌
	if err := w.limiter.Wait(ctx); err != nil {
		return
	}

	if err := retryWithBackoff(ctx, func() error {
		return ms.SendMedia(ctx, msg)
	}); err != nil {
		logger.ErrorCF("channels", "SendMedia failed", map[string]any{
			"channel": name,
			"chat_id": msg.ChatID,
			"error":   err.Error(),
			"retries": maxRetries,
		})
	}
}
