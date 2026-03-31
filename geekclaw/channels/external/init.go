package external

import (
	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// init 注册外部频道创建器，以便频道管理器可以创建外部频道实例。
func init() {
	channels.RegisterExternalCreator(func(name string, cfg config.ExternalChannelConfig, messageBus *bus.MessageBus) (channels.Channel, error) {
		return NewExternalChannel(name, cfg, messageBus)
	})
}
