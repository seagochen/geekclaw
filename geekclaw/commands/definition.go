package commands

import (
	"fmt"
	"strings"
)

// SubCommand 定义父命令中的单个子命令。
type SubCommand struct {
	Name        string
	Description string
	ArgsUsage   string // 可选，例如 "<session-id>"
	Handler     Handler
}

// Definition 是斜杠命令的唯一元数据和行为契约。
//
// 设计说明（第一阶段）：
//   - 每个频道从此类型读取命令结构，而不是维护本地副本。
//   - 可见性是全局的：所有定义对所有频道均可用。
//   - 平台菜单注册（例如 Telegram BotCommand）也从此定义派生，
//     以确保 UI 标签和运行时行为保持一致。
type Definition struct {
	Name        string
	Description string
	Usage       string // 用于简单命令；设置了 SubCommands 时忽略
	Aliases     []string
	SubCommands []SubCommand // 可选；设置后，Executor 会路由到子命令处理器
	Handler     Handler      // 用于没有子命令的简单命令
	Source      string       // 来源标签："" 或 "builtin" 表示内置，"plugin:<name>" 表示插件
}

// EffectiveUsage 返回用法字符串。当存在 SubCommands 时，
// 会根据子命令名称自动生成，以避免元数据和行为不一致。
func (d Definition) EffectiveUsage() string {
	if len(d.SubCommands) == 0 {
		return d.Usage
	}
	names := make([]string, 0, len(d.SubCommands))
	for _, sc := range d.SubCommands {
		name := sc.Name
		if sc.ArgsUsage != "" {
			name += " " + sc.ArgsUsage
		}
		names = append(names, name)
	}
	return fmt.Sprintf("/%s [%s]", d.Name, strings.Join(names, "|"))
}
