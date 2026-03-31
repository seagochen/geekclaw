package external

import (
	"context"
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/commands"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/plugin"
)

// ExternalCommandPlugin 通过 JSON-RPC over stdio 将 geekclaw 的命令系统
// 桥接到外部进程。它实现了 commands.CommandProvider 接口。
type ExternalCommandPlugin struct {
	proc *plugin.Process
	defs []commands.Definition
}

// NewExternalCommandPlugin 创建一个新的外部命令插件。
func NewExternalCommandPlugin(name string, cfg PluginConfig) *ExternalCommandPlugin {
	return &ExternalCommandPlugin{
		proc: plugin.NewProcess(name, cfg),
	}
}

// Start 启动外部进程，执行初始化握手，并填充命令定义。
func (p *ExternalCommandPlugin) Start(ctx context.Context) error {
	raw, err := p.proc.Spawn(ctx, plugin.SpawnOpts{
		LogCategory: "commands",
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

	// 将插件命令定义转换为内部 Definition 结构体
	source := "plugin:" + p.proc.Name()
	p.defs = make([]commands.Definition, 0, len(initResult.Commands))
	for _, cd := range initResult.Commands {
		def := commands.Definition{
			Name:        cd.Name,
			Description: cd.Description,
			Usage:       cd.Usage,
			Aliases:     cd.Aliases,
			Source:      source,
			Handler:     p.makeHandler(cd.Name),
		}
		p.defs = append(p.defs, def)
	}

	logger.InfoCF("commands", "Command plugin started", map[string]any{
		"plugin":   p.proc.Name(),
		"commands": len(p.defs),
		"command":  p.proc.PluginConfig().Command,
	})

	return nil
}

// Stop 优雅地关闭插件进程。
func (p *ExternalCommandPlugin) Stop() {
	p.proc.Stop()
}

// Commands 实现 commands.CommandProvider 接口。
func (p *ExternalCommandPlugin) Commands() []commands.Definition {
	return p.defs
}

// Source 实现 commands.CommandProvider 接口。
func (p *ExternalCommandPlugin) Source() string {
	return "plugin:" + p.proc.Name()
}

// makeHandler 创建一个 Handler 函数，通过 JSON-RPC 将命令执行转发到外部插件进程。
func (p *ExternalCommandPlugin) makeHandler(cmdName string) commands.Handler {
	return func(ctx context.Context, req commands.Request, rt *commands.Runtime) error {
		params := &ExecuteParams{
			Command:  cmdName,
			Text:     req.Text,
			Channel:  req.Channel,
			ChatID:   req.ChatID,
			SenderID: req.SenderID,
		}

		// 向插件提供有限的运行时上下文
		if rt != nil {
			runtimeCtx := make(map[string]any)
			if rt.GetModelInfo != nil {
				name, provider := rt.GetModelInfo()
				runtimeCtx["model"] = name
				runtimeCtx["provider"] = provider
			}
			if rt.GetWorkDir != nil {
				runtimeCtx["work_dir"] = rt.GetWorkDir()
			}
			if rt.GetTokenUsage != nil {
				prompt, completion, requests := rt.GetTokenUsage()
				runtimeCtx["token_usage"] = map[string]int64{
					"prompt_tokens":     prompt,
					"completion_tokens": completion,
					"requests":          requests,
				}
			}
			if len(runtimeCtx) > 0 {
				params.Context = runtimeCtx
			}
		}

		raw, err := p.proc.Transport().Call(ctx, MethodExecute, params)
		if err != nil {
			return req.Reply(fmt.Sprintf("Plugin error: %v", err))
		}

		result, err := ParseExecuteResult(raw)
		if err != nil {
			return req.Reply(fmt.Sprintf("Plugin response error: %v", err))
		}

		if result.Reply != "" {
			return req.Reply(result.Reply)
		}
		return nil
	}
}
