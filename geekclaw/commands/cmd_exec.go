package commands

import (
	"context"
	"strings"
)

// execCommand 注册 /exec（别名：!）用于执行 shell 命令。
// 在 pico 和 cmd 模式下均可使用；在 cmd 模式下，裸文本会被重写为 /exec，
// 因此所有 shell 执行都通过此单一处理器完成。
func execCommand() Definition {
	return Definition{
		Name:        "exec",
		Description: "Execute a shell command",
		Usage:       "/exec <command>",
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.ExecCmd == nil {
				return req.Reply(unavailableMsg)
			}
			// 去掉前导命令标记（"/exec"、"!exec" 或 "!" 回退），
			// 将剩余部分作为要执行的 shell 命令。
			args := strings.TrimSpace(req.Text)
			if idx := strings.IndexAny(args, " \t"); idx >= 0 {
				args = strings.TrimSpace(args[idx:])
			} else {
				args = ""
			}
			if args == "" {
				return req.Reply("Usage: /exec <command>\nExample: /exec ls -la")
			}
			result, err := rt.ExecCmd(ctx, args)
			if err != nil {
				return req.Reply("Error: " + err.Error())
			}
			return req.Reply(result)
		},
	}
}
