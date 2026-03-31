package external

import (
	"context"
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/plugin"
	"github.com/seagosoft/geekclaw/geekclaw/providers/protocoltypes"
)

// ExternalLLMProvider 通过 stdio 上的 JSON-RPC 将 geekclaw 的 LLM 提供者系统
// 桥接到外部进程。实现了 providers.LLMProvider 和 providers.StatefulProvider 接口。
type ExternalLLMProvider struct {
	proc             *plugin.Process
	providerName     string // 初始化握手后设置
	defaultModel     string // 初始化握手后设置
	supportsThinking bool   // 初始化握手后设置
}

// NewExternalLLMProvider 创建新的外部 LLM 插件。
func NewExternalLLMProvider(name string, cfg PluginConfig) *ExternalLLMProvider {
	return &ExternalLLMProvider{
		proc: plugin.NewProcess(name, cfg),
	}
}

// Start 启动外部进程并执行初始化握手。
func (p *ExternalLLMProvider) Start(ctx context.Context) error {
	raw, err := p.proc.Spawn(ctx, plugin.SpawnOpts{
		LogCategory: "llm",
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
	p.defaultModel = initResult.DefaultModel
	p.supportsThinking = initResult.SupportsThinking

	logger.InfoCF("llm", "LLM plugin started", map[string]any{
		"plugin":            p.proc.Name(),
		"provider":          p.providerName,
		"default_model":     p.defaultModel,
		"supports_thinking": p.supportsThinking,
		"command":           p.proc.PluginConfig().Command,
	})

	return nil
}

// Chat 实现 providers.LLMProvider 接口。
func (p *ExternalLLMProvider) Chat(
	ctx context.Context,
	messages []protocoltypes.Message,
	tools []protocoltypes.ToolDefinition,
	model string,
	options map[string]any,
) (*protocoltypes.LLMResponse, error) {
	raw, err := p.proc.Transport().Call(ctx, MethodChat, &ChatParams{
		Messages: messages,
		Tools:    tools,
		Model:    model,
		Options:  options,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM plugin %q chat: %w", p.proc.Name(), err)
	}

	result, err := ParseChatResult(raw)
	if err != nil {
		return nil, fmt.Errorf("parse chat result: %w", err)
	}

	return result.ToLLMResponse(), nil
}

// GetDefaultModel 实现 providers.LLMProvider 接口。
func (p *ExternalLLMProvider) GetDefaultModel() string {
	if p.defaultModel != "" {
		return p.defaultModel
	}
	return "plugin:" + p.proc.Name()
}

// SupportsThinking 实现 providers.ThinkingCapable 接口。
func (p *ExternalLLMProvider) SupportsThinking() bool {
	return p.supportsThinking
}

// Name 返回提供者的显示名称。
func (p *ExternalLLMProvider) Name() string {
	if p.providerName != "" {
		return p.providerName
	}
	return "plugin:" + p.proc.Name()
}

// Close 实现 providers.StatefulProvider 接口。
func (p *ExternalLLMProvider) Close() {
	p.Stop()
}

// Stop 优雅地关闭插件进程。
func (p *ExternalLLMProvider) Stop() {
	p.proc.Stop()
}
