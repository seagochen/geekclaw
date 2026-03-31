package commands

import (
	"context"
	"strings"
)

// Handler 是命令处理函数的类型定义。
type Handler func(ctx context.Context, req Request, rt *Runtime) error

// Request 封装了命令请求的上下文信息。
type Request struct {
	Channel  string
	ChatID   string
	SenderID string
	Text     string
	Reply    func(text string) error
}

// unavailableMsg 是命令在当前上下文中不可用时的默认提示信息。
const unavailableMsg = "Command unavailable in current context."

// commandPrefixes 定义了可识别的命令前缀列表。
var commandPrefixes = []string{"/", "!"}

// parseCommandName 接受 "/name"、"!name" 和 Telegram 的 "/name@bot" 格式，
// 然后规范化为小写命令名称。
func parseCommandName(input string) (string, bool) {
	token := nthToken(input, 0)
	if token == "" {
		return "", false
	}

	name, ok := trimCommandPrefix(token)
	if !ok {
		return "", false
	}
	if i := strings.Index(name, "@"); i >= 0 {
		name = name[:i]
	}
	name = normalizeCommandName(name)
	if name == "" {
		return "", false
	}
	return name, true
}

// trimCommandPrefix 去掉命令标记的前缀，并返回去掉前缀后的名称和是否成功。
func trimCommandPrefix(token string) (string, bool) {
	for _, prefix := range commandPrefixes {
		if strings.HasPrefix(token, prefix) {
			return strings.TrimPrefix(token, prefix), true
		}
	}
	return "", false
}

// HasCommandPrefix 判断输入是否以可识别的命令前缀（如 "/" 或 "!"）开头。
func HasCommandPrefix(input string) bool {
	token := nthToken(input, 0)
	if token == "" {
		return false
	}
	_, ok := trimCommandPrefix(token)
	return ok
}

// nthToken 从按空白字符分割的输入中返回第 n 个标记（从 0 开始索引）。
func nthToken(input string, n int) string {
	parts := strings.Fields(strings.TrimSpace(input))
	if n >= len(parts) {
		return ""
	}
	return parts[n]
}

// normalizeCommandName 将命令名称规范化为小写并去除首尾空白。
func normalizeCommandName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
