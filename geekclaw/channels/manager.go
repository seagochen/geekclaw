// GeekClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package channels

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/health"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/media"
)

// Manager 管理所有消息频道的生命周期、消息分发和速率限制。
type Manager struct {
	channels      map[string]Channel
	workers       map[string]*channelWorker
	bus           *bus.MessageBus
	config        *config.Config
	mediaStore    media.MediaStore
	dispatchTask  *asyncTask
	mux           *http.ServeMux
	httpServer    *http.Server
	mu            sync.RWMutex
	placeholders  sync.Map // "channel:chatID" → placeholderEntry
	typingStops   sync.Map // "channel:chatID" → typingEntry
	reactionUndos sync.Map // "channel:chatID" → reactionEntry
}

// NewManager 创建一个新的频道管理器实例。
func NewManager(cfg *config.Config, messageBus *bus.MessageBus, store media.MediaStore) (*Manager, error) {
	m := &Manager{
		channels:   make(map[string]Channel),
		workers:    make(map[string]*channelWorker),
		bus:        messageBus,
		config:     cfg,
		mediaStore: store,
	}

	if err := m.initChannels(); err != nil {
		return nil, err
	}

	return m, nil
}

// SetupHTTPServer 使用给定的监听地址创建共享 HTTP 服务器。
// 它注册健康检查端点，并发现实现了 WebhookHandler 和/或 HealthChecker 的频道
// 来注册其处理器。
func (m *Manager) SetupHTTPServer(addr string, healthServer *health.Server) {
	m.mux = http.NewServeMux()

	// 注册健康检查端点
	if healthServer != nil {
		healthServer.RegisterOnMux(m.mux)
	}

	// 发现并注册 webhook 处理器和健康检查器
	for name, ch := range m.channels {
		if wh, ok := ch.(WebhookHandler); ok {
			m.mux.Handle(wh.WebhookPath(), wh)
			logger.InfoCF("channels", "Webhook handler registered", map[string]any{
				"channel": name,
				"path":    wh.WebhookPath(),
			})
		}
		if hc, ok := ch.(HealthChecker); ok {
			m.mux.HandleFunc(hc.HealthPath(), hc.HealthHandler)
			logger.InfoCF("channels", "Health endpoint registered", map[string]any{
				"channel": name,
				"path":    hc.HealthPath(),
			})
		}
	}

	m.httpServer = &http.Server{
		Addr:         addr,
		Handler:      m.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// StartAll 启动所有已注册的频道。
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.channels) == 0 {
		logger.WarnC("channels", "No channels enabled")
		return errors.New("no channels enabled")
	}

	logger.InfoC("channels", "Starting all channels")

	dispatchCtx, cancel := context.WithCancel(ctx)
	m.dispatchTask = &asyncTask{cancel: cancel}

	for name, channel := range m.channels {
		logger.InfoCF("channels", "Starting channel", map[string]any{
			"channel": name,
		})
		if err := channel.Start(ctx); err != nil {
			logger.ErrorCF("channels", "Failed to start channel", map[string]any{
				"channel": name,
				"error":   err.Error(),
			})
			continue
		}
		// 仅在频道成功启动后才延迟创建 worker
		w := newChannelWorker(name, channel)
		m.workers[name] = w
		go m.runWorker(dispatchCtx, name, w)
		go m.runMediaWorker(dispatchCtx, name, w)
	}

	// 启动从消息总线读取并路由到 worker 的调度器
	go m.dispatchOutbound(dispatchCtx)
	go m.dispatchOutboundMedia(dispatchCtx)

	// 启动 TTL 清理器，清理过期的输入/占位符条目
	go m.runTTLJanitor(dispatchCtx)

	// 如果已配置则启动共享 HTTP 服务器
	if m.httpServer != nil {
		go func() {
			logger.InfoCF("channels", "Shared HTTP server listening", map[string]any{
				"addr": m.httpServer.Addr,
			})
			if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.ErrorCF("channels", "Shared HTTP server error", map[string]any{
					"error": err.Error(),
				})
			}
		}()
	}

	logger.InfoC("channels", "All channels started")
	return nil
}

// StopAll 停止所有频道并关闭 HTTP 服务器。
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logger.InfoC("channels", "Stopping all channels")

	// 首先关闭共享 HTTP 服务器
	if m.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := m.httpServer.Shutdown(shutdownCtx); err != nil {
			logger.ErrorCF("channels", "Shared HTTP server shutdown error", map[string]any{
				"error": err.Error(),
			})
		}
		m.httpServer = nil
	}

	// 取消调度器
	if m.dispatchTask != nil {
		m.dispatchTask.cancel()
		m.dispatchTask = nil
	}

	// 关闭所有 worker 队列并等待排空
	for _, w := range m.workers {
		if w != nil {
			close(w.queue)
		}
	}
	for _, w := range m.workers {
		if w != nil {
			<-w.done
		}
	}
	// 关闭所有媒体 worker 队列并等待排空
	for _, w := range m.workers {
		if w != nil {
			close(w.mediaQueue)
		}
	}
	for _, w := range m.workers {
		if w != nil {
			<-w.mediaDone
		}
	}

	// 停止所有频道
	for name, channel := range m.channels {
		logger.InfoCF("channels", "Stopping channel", map[string]any{
			"channel": name,
		})
		if err := channel.Stop(ctx); err != nil {
			logger.ErrorCF("channels", "Error stopping channel", map[string]any{
				"channel": name,
				"error":   err.Error(),
			})
		}
	}

	logger.InfoC("channels", "All channels stopped")
	return nil
}

// GetChannel 按名称获取频道。
func (m *Manager) GetChannel(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	channel, ok := m.channels[name]
	return channel, ok
}

// GetStatus 返回所有频道的状态信息。
func (m *Manager) GetStatus() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]any)
	for name, channel := range m.channels {
		status[name] = map[string]any{
			"enabled": true,
			"running": channel.IsRunning(),
		}
	}
	return status
}

// GetEnabledChannels 返回所有已启用的频道名称。
func (m *Manager) GetEnabledChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// RegisterChannel 注册一个新频道。
func (m *Manager) RegisterChannel(name string, channel Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[name] = channel
}

// UnregisterChannel 注销指定频道并清理其 worker。
func (m *Manager) UnregisterChannel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workers[name]; ok && w != nil {
		close(w.queue)
		<-w.done
		close(w.mediaQueue)
		<-w.mediaDone
	}
	delete(m.workers, name)
	delete(m.channels, name)
}

// SendMessage 通过频道 worker 的速率限制器和重试逻辑同步发送出站消息。
// 它会阻塞直到消息送达（或所有重试耗尽），从而在后续操作依赖于消息
// 已发送时保持顺序。
func (m *Manager) SendMessage(ctx context.Context, msg bus.OutboundMessage) error {
	m.mu.RLock()
	_, exists := m.channels[msg.Channel]
	w, wExists := m.workers[msg.Channel]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %s not found", msg.Channel)
	}
	if !wExists || w == nil {
		return fmt.Errorf("channel %s has no active worker", msg.Channel)
	}

	maxLen := 0
	if mlp, ok := w.ch.(MessageLengthProvider); ok {
		maxLen = mlp.MaxMessageLength()
	}
	if maxLen > 0 && len([]rune(msg.Content)) > maxLen {
		for _, chunk := range SplitMessage(msg.Content, maxLen) {
			chunkMsg := msg
			chunkMsg.Content = chunk
			m.sendWithRetry(ctx, msg.Channel, w, chunkMsg)
		}
	} else {
		m.sendWithRetry(ctx, msg.Channel, w, msg)
	}
	return nil
}

// SendToChannel 向指定频道的指定聊天发送消息。
func (m *Manager) SendToChannel(ctx context.Context, channelName, chatID, content string) error {
	m.mu.RLock()
	_, exists := m.channels[channelName]
	w, wExists := m.workers[channelName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %s not found", channelName)
	}

	msg := bus.OutboundMessage{
		Channel: channelName,
		ChatID:  chatID,
		Content: content,
	}

	if wExists && w != nil {
		select {
		case w.queue <- msg:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// 回退：直接发送（正常情况下不应发生）
	channel, _ := m.channels[channelName]
	return channel.Send(ctx, msg)
}
