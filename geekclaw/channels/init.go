// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package channels

import (
	"sync/atomic"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/media"
)

// ExternalChannelCreator 是创建外部频道的函数类型。
// 由 external 包在 init 时注册，以避免循环导入。
type ExternalChannelCreator func(name string, cfg config.ExternalChannelConfig, bus *bus.MessageBus) (Channel, error)

// externalCreator 保存已注册的 ExternalChannelCreator 函数。
var externalCreator atomic.Value

// RegisterExternalCreator 注册创建外部频道的工厂函数。
// 由 external 包的 init() 调用。
func RegisterExternalCreator(creator ExternalChannelCreator) {
	externalCreator.Store(creator)
}

// initChannel 是一个辅助方法，按名称查找工厂并创建频道。
func (m *Manager) initChannel(name, displayName string) {
	f, ok := getFactory(name)
	if !ok {
		logger.WarnCF("channels", "Factory not registered", map[string]any{
			"channel": displayName,
		})
		return
	}
	logger.DebugCF("channels", "Attempting to initialize channel", map[string]any{
		"channel": displayName,
	})
	ch, err := f(m.config, m.bus)
	if err != nil {
		logger.ErrorCF("channels", "Failed to initialize channel", map[string]any{
			"channel": displayName,
			"error":   err.Error(),
		})
	} else {
		injectDependencies(ch, m.mediaStore, m)
		m.channels[name] = ch
		logger.InfoCF("channels", "Channel enabled successfully", map[string]any{
			"channel": displayName,
		})
	}
}

// initChannels 初始化所有频道。
func (m *Manager) initChannels() error {
	logger.InfoC("channels", "Initializing channel manager")

	// 所有频道现在都是外部频道（通过 stdio 的 JSON-RPC）。
	// 旧版内置频道工厂不再注册。
	m.initExternalChannels()

	logger.InfoCF("channels", "Channel initialization completed", map[string]any{
		"enabled_channels": len(m.channels),
	})

	return nil
}

// initExternalChannels 创建并注册配置中定义的外部频道。
// 外部频道作为独立进程运行，通过 stdio 上的 JSON-RPC 通信。
func (m *Manager) initExternalChannels() {
	if m.config.Channels.External == nil {
		return
	}

	for name, extCfg := range m.config.Channels.External {
		if !extCfg.Enabled {
			continue
		}

		f, ok := getFactory("external:" + name)
		if ok {
			// 已注册（不应发生，但保持安全）
			ch, err := f(m.config, m.bus)
			if err != nil {
				logger.ErrorCF("channels", "Failed to initialize external channel", map[string]any{
					"channel": name,
					"error":   err.Error(),
				})
				continue
			}
			m.registerChannel(name, ch)
			continue
		}

		// 通过 external 包的构造函数直接创建。
		// 我们使用延迟导入方式：工厂由 external 包的 init() 注册。
		// 由于外部频道具有动态名称，因此在此直接创建。
		if creator, ok := externalCreator.Load().(ExternalChannelCreator); ok {
			ch, err := creator(name, extCfg, m.bus)
			if err != nil {
				logger.ErrorCF("channels", "Failed to create external channel", map[string]any{
					"channel": name,
					"error":   err.Error(),
				})
				continue
			}
			m.registerChannel(name, ch)
		}
	}
}

// injectDependencies 为频道设置 MediaStore、PlaceholderRecorder 和 Owner，
// 前提是频道实现了相应的能力接口。
func injectDependencies(ch Channel, store media.MediaStore, recorder PlaceholderRecorder) {
	if store != nil {
		if setter, ok := ch.(MediaStoreAware); ok {
			setter.SetMediaStore(store)
		}
	}
	if setter, ok := ch.(PlaceholderRecorderAware); ok {
		setter.SetPlaceholderRecorder(recorder)
	}
	if setter, ok := ch.(OwnerAware); ok {
		setter.SetOwner(ch)
	}
}

// registerChannel 注入依赖项并将频道存储到管理器中。
func (m *Manager) registerChannel(name string, ch Channel) {
	injectDependencies(ch, m.mediaStore, m)
	m.channels[name] = ch
	logger.InfoCF("channels", "External channel enabled", map[string]any{
		"channel": name,
	})
}
