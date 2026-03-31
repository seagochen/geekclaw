package commands

// BuiltinDefinitions 返回所有内置命令定义。
// 每个命令组在各自的 cmd_*.go 文件中定义。
// 定义是无状态的——运行时依赖通过执行时传递给处理器的 Runtime 参数提供。
func BuiltinDefinitions() []Definition {
	return []Definition{
		startCommand(),
		helpCommand(),
		showCommand(),
		listCommand(),
		switchCommand(),
		checkCommand(),
		// 会话模式命令（原 : 前缀）
		cmdModeCommand(),
		picoModeCommand(),
		hipicoCmnd(),
		execCommand(),
		editCommand(),
		// 信息与会话管理
		usageCommand(),
		clearCommand(),
		compactCommand(),
		// 任务管理
		stopCommand(),
		// 交互模式
		interactiveCommand(),
	}
}
