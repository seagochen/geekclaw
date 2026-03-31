package external

import (
	"context"
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/plugin"
	"github.com/seagosoft/geekclaw/geekclaw/tools"
)

// ExternalToolPlugin 通过 stdio 上的 JSON-RPC 将 geekclaw 的
// 工具系统桥接到外部进程。Start() 之后，Tools() 返回插件在
// 初始化握手期间声明的工具集。
type ExternalToolPlugin struct {
	proc         *plugin.Process
	declaredTools []tools.Tool
	services      map[string]plugin.ServiceHandler
}

// NewExternalToolPlugin 创建一个新的外部工具插件。
func NewExternalToolPlugin(name string, cfg PluginConfig) *ExternalToolPlugin {
	return &ExternalToolPlugin{
		proc: plugin.NewProcess(name, cfg),
	}
}

// WithServices 设置插件可调用的 Go 服务处理函数。
// 必须在 Start() 之前调用。
func (p *ExternalToolPlugin) WithServices(services map[string]plugin.ServiceHandler) *ExternalToolPlugin {
	p.services = services
	return p
}

// Start 生成外部进程并执行初始化握手。
// 插件用其工具定义进行响应，这些定义被转换为
// 可通过 Tools() 访问的 tools.Tool 实例。
func (p *ExternalToolPlugin) Start(ctx context.Context) error {
	raw, err := p.proc.Spawn(ctx, plugin.SpawnOpts{
		LogCategory: "tools",
		InitMethod:  MethodToolInitialize,
		InitParams:  &ToolInitializeParams{Config: p.proc.PluginConfig().Config},
		StopMethod:  MethodToolStop,
		LogMethod:   MethodToolLog,
		Services:    p.services,
	})
	if err != nil {
		return err
	}

	initResult, err := ParseToolInitializeResult(raw)
	if err != nil {
		p.proc.Stop()
		return fmt.Errorf("parse initialize result: %w", err)
	}

	p.declaredTools = make([]tools.Tool, 0, len(initResult.Tools))
	for _, td := range initResult.Tools {
		def := td // 为闭包捕获
		p.declaredTools = append(p.declaredTools, &externalTool{
			proc:        p.proc,
			name:        def.Name,
			description: def.Description,
			parameters:  def.Parameters,
		})
	}

	logger.InfoCF("tools", "Tool plugin started", map[string]any{
		"plugin":  p.proc.Name(),
		"tools":   len(p.declaredTools),
		"command": p.proc.PluginConfig().Command,
	})

	return nil
}

// Tools 返回插件声明的工具列表。
// 必须在 Start() 之后调用。
func (p *ExternalToolPlugin) Tools() []tools.Tool {
	return p.declaredTools
}

// Stop 优雅地关闭插件进程。
func (p *ExternalToolPlugin) Stop() {
	p.proc.Stop()
}

// externalTool 是将执行转发到插件的单个工具代理。
type externalTool struct {
	proc        *plugin.Process
	name        string
	description string
	parameters  map[string]any
}

// Name 返回工具名称。
func (t *externalTool) Name() string             { return t.name }

// Description 返回工具描述。
func (t *externalTool) Description() string      { return t.description }

// Parameters 返回工具参数。
func (t *externalTool) Parameters() map[string]any { return t.parameters }

// Execute 通过插件执行工具。
func (t *externalTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	raw, err := t.proc.Transport().Call(ctx, MethodToolExecute, &ToolExecuteParams{
		Name:   t.name,
		Params: args,
	})
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("tool plugin %q: %v", t.proc.Name(), err))
	}

	result, err := ParseToolExecuteResult(raw)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("parse tool execute result: %v", err))
	}

	if result.Error {
		return tools.ErrorResult(result.Content)
	}
	return tools.SilentResult(result.Content)
}
