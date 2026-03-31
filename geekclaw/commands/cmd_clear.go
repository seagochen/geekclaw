package commands

import "context"

// clearCommand 返回用于清除当前会话所有聊天记录的命令定义。
func clearCommand() Definition {
	return Definition{
		Name:        "clear",
		Description: "Clear all chat history for this session",
		Usage:       "/clear",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.ClearSession == nil {
				return req.Reply(unavailableMsg)
			}
			if err := rt.ClearSession(); err != nil {
				return req.Reply("Failed to clear session: " + err.Error())
			}
			return req.Reply("Chat history cleared. Starting fresh.")
		},
	}
}
