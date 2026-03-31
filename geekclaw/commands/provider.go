package commands

// CommandProvider 是为插件提供命令定义的接口。
// 内部 Go 提供者和外部 JSON-RPC 提供者均实现此接口。
type CommandProvider interface {
	// Commands 返回此插件提供的命令定义。
	Commands() []Definition

	// Source 返回提供者来源的可读标签（例如 "builtin"、"plugin:hello"）。
	Source() string
}

// PluginCommandDef 描述在初始化握手期间从外部插件接收到的命令定义。
type PluginCommandDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Usage       string   `json:"usage,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}
