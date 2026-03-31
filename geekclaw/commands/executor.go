package commands

import (
	"context"
	"fmt"
	"strings"
)

// Outcome 表示命令执行的结果类型。
type Outcome int

const (
	// OutcomePassthrough 表示此输入应继续通过正常的代理流程处理。
	OutcomePassthrough Outcome = iota
	// OutcomeHandled 表示命令处理器已执行（无论处理器是否返回错误）。
	OutcomeHandled
)

// ExecuteResult 包含命令执行的结果信息。
type ExecuteResult struct {
	Outcome Outcome
	Command string
	Err     error
}

// Executor 负责将请求分发到已注册的命令处理器。
type Executor struct {
	reg *Registry
	rt  *Runtime
}

// NewExecutor 创建一个新的命令执行器。
func NewExecutor(reg *Registry, rt *Runtime) *Executor {
	return &Executor{reg: reg, rt: rt}
}

// Execute 实现两种状态的命令决策：
// 1) 已处理：立即执行命令；
// 2) 透传：不是命令或有意推迟到代理逻辑处理。
func (e *Executor) Execute(ctx context.Context, req Request) ExecuteResult {
	cmdName, ok := parseCommandName(req.Text)
	if !ok {
		return ExecuteResult{Outcome: OutcomePassthrough}
	}

	if e == nil || e.reg == nil {
		return ExecuteResult{Outcome: OutcomePassthrough, Command: cmdName}
	}

	def, found := e.reg.Lookup(cmdName)
	if !found {
		// "!foo" 中 foo 不是已注册的命令 → 回退到 /exec。
		// 这使得 ! 成为通用的 shell 执行前缀：!ls、!git status 等。
		if strings.HasPrefix(strings.TrimSpace(req.Text), "!") {
			if execDef, ok := e.reg.Lookup("exec"); ok {
				req.Text = "/exec " + strings.TrimSpace(req.Text)[1:]
				return e.executeDefinition(ctx, req, execDef)
			}
		}
		return ExecuteResult{Outcome: OutcomePassthrough, Command: cmdName}
	}

	return e.executeDefinition(ctx, req, def)
}

// executeDefinition 执行指定的命令定义，处理子命令路由。
func (e *Executor) executeDefinition(ctx context.Context, req Request, def Definition) ExecuteResult {
	// 确保 Reply 始终非 nil，这样处理器无需检查。
	if req.Reply == nil {
		req.Reply = func(string) error { return nil }
	}

	// 简单命令——无子命令
	if len(def.SubCommands) == 0 {
		if def.Handler == nil {
			return ExecuteResult{Outcome: OutcomePassthrough, Command: def.Name}
		}
		err := def.Handler(ctx, req, e.rt)
		return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name, Err: err}
	}

	// 子命令路由
	subName := nthToken(req.Text, 1)
	if subName == "" {
		err := req.Reply("Usage: " + def.EffectiveUsage())
		return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name, Err: err}
	}

	normalized := normalizeCommandName(subName)
	for _, sc := range def.SubCommands {
		if normalizeCommandName(sc.Name) == normalized {
			if sc.Handler == nil {
				return ExecuteResult{Outcome: OutcomePassthrough, Command: def.Name}
			}
			err := sc.Handler(ctx, req, e.rt)
			return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name, Err: err}
		}
	}

	// 未知子命令
	err := req.Reply(fmt.Sprintf("Unknown option: %s. Usage: %s", subName, def.EffectiveUsage()))
	return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name, Err: err}
}
