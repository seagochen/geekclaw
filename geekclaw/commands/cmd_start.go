package commands

import "context"

// startCommand 返回用于启动机器人的命令定义。
func startCommand() Definition {
	return Definition{
		Name:        "start",
		Description: "Start the bot",
		Usage:       "/start",
		Handler: func(_ context.Context, req Request, _ *Runtime) error {
			return req.Reply("Hello! I am GeekClaw 🦞")
		},
	}
}
