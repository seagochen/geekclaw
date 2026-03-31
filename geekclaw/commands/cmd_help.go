package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// helpCommand 返回用于显示帮助信息的命令定义。
func helpCommand() Definition {
	return Definition{
		Name:        "help",
		Description: "Show this help message",
		Usage:       "/help",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			var defs []Definition
			if rt != nil && rt.ListDefinitions != nil {
				defs = rt.ListDefinitions()
			} else {
				defs = BuiltinDefinitions()
			}
			return req.Reply(formatHelpMessage(defs))
		},
	}
}

// formatHelpMessage 将命令定义列表格式化为帮助信息字符串。
func formatHelpMessage(defs []Definition) string {
	if len(defs) == 0 {
		return "No commands available."
	}

	// 将命令分为内置命令和插件命令
	var builtin, plugin []Definition
	for _, def := range defs {
		if strings.HasPrefix(def.Source, "plugin:") {
			plugin = append(plugin, def)
		} else {
			builtin = append(builtin, def)
		}
	}

	sortDefs := func(ds []Definition) {
		sort.Slice(ds, func(i, j int) bool { return ds[i].Name < ds[j].Name })
	}
	sortDefs(builtin)
	sortDefs(plugin)

	lines := make([]string, 0, len(defs)+2)
	for _, def := range builtin {
		lines = append(lines, formatDefLine(def))
	}

	if len(plugin) > 0 {
		lines = append(lines, "", "Plugin commands:")
		for _, def := range plugin {
			lines = append(lines, formatDefLine(def))
		}
	}

	return strings.Join(lines, "\n")
}

// formatDefLine 将单个命令定义格式化为一行帮助文本。
func formatDefLine(def Definition) string {
	usage := def.EffectiveUsage()
	if usage == "" {
		usage = "/" + def.Name
	}
	desc := def.Description
	if desc == "" {
		desc = "No description"
	}
	return fmt.Sprintf("%s - %s", usage, desc)
}
