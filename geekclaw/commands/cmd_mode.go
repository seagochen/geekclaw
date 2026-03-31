package commands

import (
	"context"
	"strings"
)

// cmdModeCommand 返回用于切换到命令模式（执行 shell 命令）的命令定义。
func cmdModeCommand() Definition {
	return Definition{
		Name:        "cmd",
		Description: "Switch to command mode (execute shell commands)",
		Usage:       "/cmd",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.SetModeCmd == nil {
				return req.Reply(unavailableMsg)
			}
			return req.Reply(rt.SetModeCmd())
		},
	}
}

// picoModeCommand 返回用于切换到聊天模式（AI 对话）的命令定义。
func picoModeCommand() Definition {
	return Definition{
		Name:        "pico",
		Description: "Switch to chat mode (AI conversation)",
		Usage:       "/pico",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.SetModePico == nil {
				return req.Reply(unavailableMsg)
			}
			return req.Reply(rt.SetModePico())
		},
	}
}

// hipicoCmnd 返回用于从命令模式发起一次性 AI 帮助请求的命令定义。
func hipicoCmnd() Definition {
	return Definition{
		Name:        "hipico",
		Description: "Ask AI for one-shot help (works from command mode)",
		Usage:       "/hipico <message>",
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.RunOneShot == nil {
				return req.Reply(unavailableMsg)
			}
			// 去掉命令前缀以获取消息正文
			msg := strings.TrimSpace(req.Text)
			for _, prefix := range []string{"/hipico", "!hipico"} {
				if strings.HasPrefix(msg, prefix) {
					msg = strings.TrimSpace(msg[len(prefix):])
					break
				}
			}
			if msg == "" {
				return req.Reply(
					"👋 Hi~ What can I help you with?\n\n💡 Examples:\n  /hipico what files are in the working directory?\n  /hipico what time is it now?\n  /hipico what's the progress on the previous task?",
				)
			}
			reply, err := rt.RunOneShot(ctx, msg)
			if err != nil {
				return req.Reply("Error: " + err.Error())
			}
			return req.Reply(reply)
		},
	}
}
