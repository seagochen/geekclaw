package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/seagosoft/geekclaw/geekclaw/interactive"
)

// interactiveCommand 创建用于管理交互模式的 /interactive 命令。
func interactiveCommand() Definition {
	return Definition{
		Name:        "interactive",
		Description: "Manage interactive mode for step-by-step confirmations",
		Usage:       "/interactive [on|off|status|auto]",
		SubCommands: []SubCommand{
			{
				Name:        "on",
				Description: "Enable confirmation mode (always ask before executing)",
				Handler:     interactiveOnHandler,
			},
			{
				Name:        "off",
				Description: "Disable interactive mode (direct execution)",
				Handler:     interactiveOffHandler,
			},
			{
				Name:        "auto",
				Description: "Enable auto mode (AI decides when to confirm)",
				Handler:     interactiveAutoHandler,
			},
			{
				Name:        "status",
				Description: "Show current interactive mode status",
				Handler:     interactiveStatusHandler,
			},
			{
				Name:        "confirm",
				Description: "Confirm a pending action",
				ArgsUsage:   "<option-id>",
				Handler:     interactiveConfirmHandler,
			},
			{
				Name:        "cancel",
				Description: "Cancel a pending confirmation",
				Handler:     interactiveCancelHandler,
			},
		},
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			// 未指定子命令——显示状态
			return interactiveStatusHandler(ctx, req, rt)
		},
	}
}

// interactiveOnHandler 处理启用确认模式的请求。
func interactiveOnHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.SetInteractiveMode == nil {
		return req.Reply(unavailableMsg)
	}

	oldMode := rt.SetInteractiveMode(interactive.ModeConfirm)
	return req.Reply(fmt.Sprintf("✅ Interactive mode enabled (was: %s)\n\nI will now ask for confirmation before executing actions.", oldMode.String()))
}

// interactiveOffHandler 处理禁用交互模式（切换为直接执行模式）的请求。
func interactiveOffHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.SetInteractiveMode == nil {
		return req.Reply(unavailableMsg)
	}

	oldMode := rt.SetInteractiveMode(interactive.ModeDirect)
	return req.Reply(fmt.Sprintf("✅ Direct mode enabled (was: %s)\n\nI will execute actions immediately without confirmation.", oldMode.String()))
}

// interactiveAutoHandler 处理启用自动模式（由 AI 决定是否需要确认）的请求。
func interactiveAutoHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.SetInteractiveMode == nil {
		return req.Reply(unavailableMsg)
	}

	oldMode := rt.SetInteractiveMode(interactive.ModeAuto)
	return req.Reply(fmt.Sprintf("✅ Auto mode enabled (was: %s)\n\nI will decide when to ask for confirmation based on the action.", oldMode.String()))
}

// interactiveStatusHandler 处理显示当前交互模式状态的请求。
func interactiveStatusHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.GetInteractiveMode == nil {
		return req.Reply(unavailableMsg)
	}

	mode := rt.GetInteractiveMode()
	pending := rt.GetPendingConfirmation != nil

	var modeDesc string
	switch mode {
	case interactive.ModeConfirm:
		modeDesc = "🔔 Confirmation mode - I will always ask before executing"
	case interactive.ModeDirect:
		modeDesc = "⚡ Direct mode - I execute immediately"
	case interactive.ModeAuto:
		modeDesc = "🤖 Auto mode - I decide when to confirm"
	default:
		modeDesc = fmt.Sprintf("Unknown mode: %s", mode.String())
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Interactive Mode: %s\n\n", modeDesc))

	if pending {
		result.WriteString("📋 You have a pending confirmation.\n")
		result.WriteString("Use `/interactive confirm <option>` to respond or `/interactive cancel` to cancel.\n")
	} else {
		result.WriteString("No pending confirmations.\n")
	}

	result.WriteString("\nCommands:\n")
	result.WriteString("  /interactive on     - Always ask for confirmation\n")
	result.WriteString("  /interactive off    - Execute immediately\n")
	result.WriteString("  /interactive auto   - Let AI decide\n")
	result.WriteString("  /interactive status - Show this status\n")

	return req.Reply(result.String())
}

// interactiveConfirmHandler 处理确认待执行操作的请求。
func interactiveConfirmHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.RespondToConfirmation == nil {
		return req.Reply(unavailableMsg)
	}

	// 从命令参数中获取响应内容（命令之后的所有内容）
	response := extractCommandArgs(req.Text, "confirm")
	if response == "" {
		// 检查是否有待确认的操作
		if rt.GetPendingConfirmation != nil {
			conf := rt.GetPendingConfirmation()
			if conf != nil {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📋 Pending Confirmation:\n\n%s\n\n", conf.Message))
				if len(conf.Options) > 0 {
					sb.WriteString("Options:\n")
					for i, opt := range conf.Options {
						sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, opt.ID))
					}
					sb.WriteString("\nUse `/interactive confirm <number>` or `/interactive confirm <option-id>`\n")
				} else {
					sb.WriteString("Use `/interactive confirm yes` or `/interactive confirm no`\n")
				}
				return req.Reply(sb.String())
			}
		}
		return req.Reply("⚠️ No pending confirmation. Use `/interactive status` to check.")
	}

	// 发送响应
	err := rt.RespondToConfirmation(response)
	if err != nil {
		return req.Reply(fmt.Sprintf("❌ Error: %v", err))
	}

	return req.Reply(fmt.Sprintf("✅ Response recorded: %s", response))
}

// extractCommandArgs 从完整命令文本中提取子命令之后的参数。
// 例如，"/interactive confirm option1" 中子命令为 "confirm"，返回 "option1"。
func extractCommandArgs(text, subcommand string) string {
	// 去掉主命令前缀（例如 "/interactive"）
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return ""
	}

	// 查找子命令
	for i, part := range parts {
		name, ok := trimCommandPrefix(part)
		if ok && strings.EqualFold(name, subcommand) {
			if i+1 < len(parts) {
				return strings.Join(parts[i+1:], " ")
			}
			return ""
		}
	}
	return ""
}

// interactiveCancelHandler 处理取消待确认操作的请求。
func interactiveCancelHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.CancelConfirmation == nil {
		return req.Reply(unavailableMsg)
	}

	err := rt.CancelConfirmation()
	if err != nil {
		return req.Reply(fmt.Sprintf("❌ Error: %v", err))
	}

	return req.Reply("✅ Confirmation cancelled.")
}
