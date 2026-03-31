package plugin

import (
	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// HandleLogNotification 将外部插件的日志通知分发到
// geekclaw 的结构化日志器。通知参数必须包含
// "level" 和 "message" 字符串字段。
func HandleLogNotification(notif *Notification, pluginName, category string) {
	if notif.Params == nil {
		return
	}

	params, ok := notif.Params.(map[string]any)
	if !ok {
		return
	}

	msg, _ := params["message"].(string)
	level, _ := params["level"].(string)
	fields := map[string]any{"plugin": pluginName}

	switch level {
	case "debug":
		logger.DebugCF(category, msg, fields)
	case "warn":
		logger.WarnCF(category, msg, fields)
	case "error":
		logger.ErrorCF(category, msg, fields)
	default:
		logger.InfoCF(category, msg, fields)
	}
}

// LogWriter 是一个 io.Writer，将外部进程的 stderr 转发到
// geekclaw 的结构化日志器。将其赋值给 cmd.Stderr。
type LogWriter struct {
	Name     string // 插件名称
	Category string // 日志分类（例如 "commands"、"search"）
}

// Write 实现 io.Writer 接口。
func (w *LogWriter) Write(p []byte) (n int, err error) {
	logger.DebugCF(w.Category, string(p), map[string]any{
		"plugin": w.Name,
		"stream": "stderr",
	})
	return len(p), nil
}
