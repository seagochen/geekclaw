// Package plugin 提供外部插件进程的管理功能。
package plugin

// Config 是外部插件进程的通用配置。
// 用于命令、搜索、语音和 LLM 提供者插件。
type Config struct {
	Enabled bool              `json:"enabled"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Config  map[string]any    `json:"config,omitempty"` // 初始化时传递给插件
}
