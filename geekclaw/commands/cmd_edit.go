package commands

import (
	"context"
	"strings"
)

const editHelp = `Usage:
  /edit <file>                — view file with line numbers
  /edit <file> <N> <text>    — replace line N with text
  /edit <file> +<N> <text>   — insert after line N
  /edit <file> -<N>          — delete line N
  /edit <file> -m """        — write full content (multi-line)
  <content>
  """

Examples:
  /edit main.go              — view main.go
  /edit main.go 5 func foo() — replace line 5
  /edit main.go +10 // note  — insert after line 10
  /edit main.go -3           — delete line 3
  /edit README.md -m """
  # Title
  Hello world
  """                        — overwrite file`

// editCommand 返回用于查看或编辑文件的命令定义（在聊天模式和命令模式下均可使用）。
func editCommand() Definition {
	return Definition{
		Name:        "edit",
		Description: "View or edit files (works in both chat and command mode)",
		Usage:       "/edit <file> [operation] | /edit help",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.Text), "/edit"))
			if arg == "help" || arg == "--help" || arg == "-h" {
				return req.Reply(editHelp)
			}
			if rt == nil || rt.EditFile == nil {
				return req.Reply(unavailableMsg)
			}
			// 传递完整文本；EditFile 在内部会去掉 /edit 前缀
			return req.Reply(rt.EditFile(req.Text))
		},
	}
}
