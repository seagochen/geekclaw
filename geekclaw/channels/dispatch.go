// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package channels

import (
	"context"

	"golang.org/x/time/rate"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// channelWorker 封装单个频道的出站消息队列和速率限制器。
type channelWorker struct {
	ch         Channel
	queue      chan bus.OutboundMessage
	mediaQueue chan bus.OutboundMediaMessage
	done       chan struct{}
	mediaDone  chan struct{}
	limiter    *rate.Limiter
}

// asyncTask 封装一个可取消的异步任务。
type asyncTask struct {
	cancel context.CancelFunc
}

// runWorker 处理单个频道的出站消息，自动拆分超过频道最大消息长度的消息。
func (m *Manager) runWorker(ctx context.Context, name string, w *channelWorker) {
	defer close(w.done)
	for {
		select {
		case msg, ok := <-w.queue:
			if !ok {
				return
			}
			maxLen := 0
			if mlp, ok := w.ch.(MessageLengthProvider); ok {
				maxLen = mlp.MaxMessageLength()
			}
			if maxLen > 0 && len([]rune(msg.Content)) > maxLen {
				chunks := SplitMessage(msg.Content, maxLen)
				for _, chunk := range chunks {
					chunkMsg := msg
					chunkMsg.Content = chunk
					m.sendWithRetry(ctx, name, w, chunkMsg)
				}
			} else {
				m.sendWithRetry(ctx, name, w, msg)
			}
		case <-ctx.Done():
			return
		}
	}
}

// runMediaWorker 处理单个频道的出站媒体消息。
func (m *Manager) runMediaWorker(ctx context.Context, name string, w *channelWorker) {
	defer close(w.mediaDone)
	for {
		select {
		case msg, ok := <-w.mediaQueue:
			if !ok {
				return
			}
			m.sendMediaWithRetry(ctx, name, w, msg)
		case <-ctx.Done():
			return
		}
	}
}

// dispatchLoop 是通用的消息分发循环，从消息总线订阅消息并路由到对应的频道 worker。
func dispatchLoop[M any](
	ctx context.Context,
	m *Manager,
	subscribe func(context.Context) (M, bool),
	getChannel func(M) string,
	enqueue func(context.Context, *channelWorker, M) bool,
	startMsg, stopMsg, unknownMsg, noWorkerMsg string,
) {
	logger.InfoC("channels", startMsg)

	for {
		msg, ok := subscribe(ctx)
		if !ok {
			logger.InfoC("channels", stopMsg)
			return
		}

		channel := getChannel(msg)

		// 静默跳过内部频道
		if IsInternalChannel(channel) {
			continue
		}

		m.mu.RLock()
		_, exists := m.channels[channel]
		w, wExists := m.workers[channel]
		m.mu.RUnlock()

		if !exists {
			logger.WarnCF("channels", unknownMsg, map[string]any{"channel": channel})
			continue
		}

		if wExists && w != nil {
			if !enqueue(ctx, w, msg) {
				return
			}
		} else if exists {
			logger.WarnCF("channels", noWorkerMsg, map[string]any{"channel": channel})
		}
	}
}

// dispatchOutbound 分发出站文本消息到对应的频道 worker。
func (m *Manager) dispatchOutbound(ctx context.Context) {
	dispatchLoop(
		ctx, m,
		m.bus.SubscribeOutbound,
		func(msg bus.OutboundMessage) string { return msg.Channel },
		func(ctx context.Context, w *channelWorker, msg bus.OutboundMessage) bool {
			select {
			case w.queue <- msg:
				return true
			case <-ctx.Done():
				return false
			default:
				// 队列已满，记录告警后阻塞等待
				logger.WarnCF("channels", "Outbound queue full, applying backpressure", map[string]any{
					"channel":    msg.Channel,
					"queue_cap":  cap(w.queue),
					"queue_len":  len(w.queue),
				})
				select {
				case w.queue <- msg:
					return true
				case <-ctx.Done():
					return false
				}
			}
		},
		"Outbound dispatcher started",
		"Outbound dispatcher stopped",
		"Unknown channel for outbound message",
		"Channel has no active worker, skipping message",
	)
}

// dispatchOutboundMedia 分发出站媒体消息到对应的频道 worker。
func (m *Manager) dispatchOutboundMedia(ctx context.Context) {
	dispatchLoop(
		ctx, m,
		m.bus.SubscribeOutboundMedia,
		func(msg bus.OutboundMediaMessage) string { return msg.Channel },
		func(ctx context.Context, w *channelWorker, msg bus.OutboundMediaMessage) bool {
			select {
			case w.mediaQueue <- msg:
				return true
			case <-ctx.Done():
				return false
			default:
				logger.WarnCF("channels", "Outbound media queue full, applying backpressure", map[string]any{
					"channel":   msg.Channel,
					"queue_cap": cap(w.mediaQueue),
					"queue_len": len(w.mediaQueue),
				})
				select {
				case w.mediaQueue <- msg:
					return true
				case <-ctx.Done():
					return false
				}
			}
		},
		"Outbound media dispatcher started",
		"Outbound media dispatcher stopped",
		"Unknown channel for outbound media message",
		"Channel has no active worker, skipping media message",
	)
}
