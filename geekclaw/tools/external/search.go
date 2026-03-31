package external

import (
	"context"
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/plugin"
)

// ExternalSearchProvider 通过 stdio 上的 JSON-RPC 将 geekclaw 的
// 网络搜索系统桥接到外部进程。它实现了 tools.SearchProvider。
type ExternalSearchProvider struct {
	proc         *plugin.Process
	providerName string // 在初始化握手后设置
}

// NewExternalSearchProvider 创建一个新的外部搜索插件。
func NewExternalSearchProvider(name string, cfg PluginConfig) *ExternalSearchProvider {
	return &ExternalSearchProvider{
		proc: plugin.NewProcess(name, cfg),
	}
}

// Start 生成外部进程并执行初始化握手。
func (p *ExternalSearchProvider) Start(ctx context.Context) error {
	raw, err := p.proc.Spawn(ctx, plugin.SpawnOpts{
		LogCategory: "search",
		InitMethod:  MethodInitialize,
		InitParams:  &InitializeParams{Config: p.proc.PluginConfig().Config},
		StopMethod:  MethodStop,
		LogMethod:   MethodLog,
	})
	if err != nil {
		return err
	}

	initResult, err := ParseInitializeResult(raw)
	if err != nil {
		p.proc.Stop()
		return fmt.Errorf("parse initialize result: %w", err)
	}

	p.providerName = initResult.Name
	if p.providerName == "" {
		p.providerName = p.proc.Name()
	}

	logger.InfoCF("search", "Search plugin started", map[string]any{
		"plugin":   p.proc.Name(),
		"provider": p.providerName,
		"command":  p.proc.PluginConfig().Command,
	})

	return nil
}

// Name 返回提供者的显示名称。
func (p *ExternalSearchProvider) Name() string {
	if p.providerName != "" {
		return p.providerName
	}
	return "plugin:" + p.proc.Name()
}

// Search 实现 tools.SearchProvider。
func (p *ExternalSearchProvider) Search(ctx context.Context, query string, count int) (string, error) {
	raw, err := p.proc.Transport().Call(ctx, MethodSearch, &SearchParams{
		Query: query,
		Count: count,
	})
	if err != nil {
		return "", fmt.Errorf("search plugin %q: %w", p.proc.Name(), err)
	}

	result, err := ParseSearchResult(raw)
	if err != nil {
		return "", fmt.Errorf("parse search result: %w", err)
	}

	return result.Results, nil
}

// Stop 优雅地关闭插件进程。
func (p *ExternalSearchProvider) Stop() {
	p.proc.Stop()
}
