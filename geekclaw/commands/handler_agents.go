package commands

import (
	"context"
	"fmt"
	"strings"
)

// agentsHandler 返回一个共享的处理器，供 /show agents 和 /list agents 使用。
func agentsHandler() Handler {
	return func(_ context.Context, req Request, rt *Runtime) error {
		if rt == nil || rt.ListAgentIDs == nil {
			return req.Reply(unavailableMsg)
		}
		ids := rt.ListAgentIDs()
		if len(ids) == 0 {
			return req.Reply("No agents registered")
		}
		return req.Reply(fmt.Sprintf("Registered agents: %s", strings.Join(ids, ", ")))
	}
}
