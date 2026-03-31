package channels

import (
	"sync"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// ChannelFactory 是从配置和消息总线创建 Channel 的构造函数类型。
// 每个频道子包通过 init() 注册一个或多个工厂。
type ChannelFactory func(cfg *config.Config, bus *bus.MessageBus) (Channel, error)

var (
	factoriesMu sync.RWMutex
	factories   = map[string]ChannelFactory{}
)

// RegisterFactory 注册一个命名的频道工厂。由子包的 init() 函数调用。
func RegisterFactory(name string, f ChannelFactory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = f
}

// getFactory 按名称查找频道工厂。
func getFactory(name string) (ChannelFactory, bool) {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	f, ok := factories[name]
	return f, ok
}
