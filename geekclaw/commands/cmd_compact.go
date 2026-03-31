package commands

import "context"

// compactCommand 返回用于压缩聊天记录以减少上下文大小的命令定义。
func compactCommand() Definition {
	return Definition{
		Name:        "compact",
		Description: "Compress chat history to reduce context size",
		Usage:       "/compact",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.CompactSession == nil {
				return req.Reply(unavailableMsg)
			}
			if err := rt.CompactSession(); err != nil {
				return req.Reply("Failed to compact session: " + err.Error())
			}
			return req.Reply("Chat history compacted.")
		},
	}
}
