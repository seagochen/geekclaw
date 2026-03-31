package commands

import (
	"context"
	"strings"
)

// stopCommand 创建用于中断 AI 任务的 /stop 命令。
func stopCommand() Definition {
	return Definition{
		Name:        "stop",
		Description: "Stop the current AI processing task",
		Usage:       "/stop",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil {
				return req.Reply(unavailableMsg)
			}

			// 优先尝试停止当前会话中最近的任务
			if rt.StopLatestTaskInSession != nil {
				stopped, taskInfo := rt.StopLatestTaskInSession()
				if stopped {
					return req.Reply("✓ Stopped: " + taskInfo)
				}
			}

			// 回退到停止任意最近的任务
			if rt.StopLatestTask != nil {
				stopped, taskInfo := rt.StopLatestTask()
				if stopped {
					return req.Reply("✓ Stopped: " + taskInfo)
				}
			}

			// 检查是否有正在运行的任务
			if rt.ListActiveTasks != nil {
				tasks := rt.ListActiveTasks()
				if len(tasks) > 0 {
					return req.Reply("No cancellable tasks found.\n\nActive tasks:\n" + strings.Join(tasks, "\n"))
				}
			}

			return req.Reply("No active tasks to stop.")
		},
	}
}
