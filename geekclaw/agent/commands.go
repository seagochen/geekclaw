// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/commands"
	"github.com/seagosoft/geekclaw/geekclaw/interactive"
	"github.com/seagosoft/geekclaw/geekclaw/tools"
)

// handleCommand 检查消息是否为命令前缀，如果是则执行该命令。
// 返回命令的回复内容和是否已处理的标志。
func (al *AgentLoop) handleCommand(
	ctx context.Context,
	msg bus.InboundMessage,
	agent *AgentInstance,
	sessionKey string,
) (string, bool) {
	if !commands.HasCommandPrefix(msg.Content) {
		return "", false
	}

	if al.cmdRegistry == nil {
		return "", false
	}

	rt := al.buildCommandsRuntime(agent, sessionKey, msg)
	executor := commands.NewExecutor(al.cmdRegistry, rt)

	var commandReply string
	result := executor.Execute(ctx, commands.Request{
		Channel:  msg.Channel,
		ChatID:   msg.ChatID,
		SenderID: msg.SenderID,
		Text:     msg.Content,
		Reply: func(text string) error {
			commandReply = text
			return nil
		},
	})

	switch result.Outcome {
	case commands.OutcomeHandled:
		if result.Err != nil {
			return mapCommandError(result), true
		}
		if commandReply != "" {
			return commandReply, true
		}
		return "", true
	default: // OutcomePassthrough — 让消息穿透传递给 LLM
		return "", false
	}
}

// buildCommandsRuntime 构建命令执行运行时环境，包含会话管理、文件编辑、任务管理等回调。
func (al *AgentLoop) buildCommandsRuntime(agent *AgentInstance, sessionKey string, msg bus.InboundMessage) *commands.Runtime {
	rt := &commands.Runtime{
		Config:          al.cfg,
		ListAgentIDs:    al.registry.ListAgentIDs,
		ListDefinitions: al.cmdRegistry.Definitions,
		GetEnabledChannels: func() []string {
			if al.channelManager == nil {
				return nil
			}
			return al.channelManager.GetEnabledChannels()
		},
		SwitchChannel: func(value string) error {
			if al.channelManager == nil {
				return fmt.Errorf("channel manager not initialized")
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Errorf("channel '%s' not found or not enabled", value)
			}
			return nil
		},

		// 会话模式（会话作用域）
		GetSessionMode: func() string {
			if al.getSessionMode(sessionKey) == modeCmd {
				return "cmd"
			}
			return "pico"
		},
		SetModeCmd: func() string {
			if agent == nil {
				return "Command unavailable in current context."
			}
			if !al.isExecAdmin(msg.Sender) {
				return "Permission denied: you are not authorized to use command mode."
			}
			al.setSessionMode(sessionKey, modeCmd)
			workDir := al.getSessionWorkDir(sessionKey)
			if workDir == "" {
				workDir = agent.Workspace
				al.setSessionWorkDir(sessionKey, workDir)
			}
			return fmt.Sprintf("```\n%s$\n```\nType `/pico` to return to chat mode.", shortenHomePath(workDir))
		},
		SetModePico: func() string {
			al.setSessionMode(sessionKey, modePico)
			return "Switched to chat mode. Type /cmd to enter command mode."
		},
		GetWorkDir: func() string {
			if agent == nil {
				return ""
			}
			workDir := al.getSessionWorkDir(sessionKey)
			if workDir == "" {
				return agent.Workspace
			}
			return workDir
		},
		GetWorkspace: func() string {
			if agent == nil {
				return ""
			}
			return agent.Workspace
		},

		// 文件编辑 — 委托给 handleEditCommand 并进行正确的路径解析
		EditFile: func(content string) string {
			if agent == nil {
				return "Command unavailable in current context."
			}
			workDir := al.getSessionWorkDir(sessionKey)
			if workDir == "" {
				workDir = agent.Workspace
			}
			return al.handleEditCommand(content, workDir, agent.Workspace)
		},

		// Token 使用统计
		GetTokenUsage: func() (int64, int64, int64) {
			if agent == nil {
				return 0, 0, 0
			}
			return agent.TotalPromptTokens.Load(), agent.TotalCompletionTokens.Load(), agent.TotalRequests.Load()
		},

		// Shell 命令执行 — 使用会话上下文包装 executeCmdMode
		ExecCmd: func(ctx context.Context, command string) (string, error) {
			if agent == nil {
				return "", fmt.Errorf("no agent available")
			}
			if !al.isExecAdmin(msg.Sender) {
				return "", fmt.Errorf("permission denied: you are not authorized to execute commands")
			}
			return al.executeCmdMode(ctx, agent, command, sessionKey, msg.Channel, msg.ChatID, true)
		},

		// 一次性 AI 查询，用于 /hipico（保持 modeCmd 不变，使用独立的会话键）
		RunOneShot: func(ctx context.Context, message string) (string, error) {
			if agent == nil {
				return "", fmt.Errorf("no agent available")
			}
			if !al.isExecAdmin(msg.Sender) {
				return "", fmt.Errorf("permission denied: you are not authorized to use this command")
			}
			workDir := al.getSessionWorkDir(sessionKey)
			if workDir == "" {
				workDir = agent.Workspace
			}
			return al.runAgentLoop(ctx, agent, processOptions{
				SessionKey:      sessionKey + ":hipico",
				Channel:         msg.Channel,
				ChatID:          msg.ChatID,
				UserMessage:     message,
				Media:           msg.Media,
				DefaultResponse: defaultResponse,
				EnableSummary:   false,
				SendResponse:    false,
				WorkingDir:      workDir,
			})
		},

		// 会话管理
		ClearSession: func() error {
			if agent == nil {
				return fmt.Errorf("no agent available")
			}
			agent.Sessions.TruncateHistory(sessionKey, 0)
			agent.Sessions.SetSummary(sessionKey, "")
			return agent.Sessions.Save(sessionKey)
		},
		CompactSession: func() error {
			if agent == nil {
				return fmt.Errorf("no agent available")
			}
			al.summarizeSession(agent, sessionKey)
			return nil
		},

		// 任务管理，用于停止 AI 处理
		StopLatestTask: func() (bool, string) {
			task, ok := al.taskQueue.StopLatest()
			if !ok {
				return false, ""
			}
			return true, task.String()
		},
		StopLatestTaskInSession: func() (bool, string) {
			task, ok := al.taskQueue.StopLatestBySession(sessionKey)
			if !ok {
				return false, ""
			}
			return true, task.String()
		},
		ListActiveTasks: func() []string {
			tasks := al.taskQueue.List()
			result := make([]string, len(tasks))
			for i, task := range tasks {
				result[i] = task.String()
			}
			return result
		},

		// 交互模式管理
		GetInteractiveMode: func() interactive.Mode {
			return al.getInteractiveMode(sessionKey)
		},
		SetInteractiveMode: func(mode interactive.Mode) interactive.Mode {
			oldMode := al.getInteractiveMode(sessionKey)
			al.setInteractiveMode(sessionKey, mode)
			return oldMode
		},
		GetPendingConfirmation: func() *struct {
			ID      string
			Message string
			Options []struct {
				ID string
			}
		} {
			conf := al.interactiveMgr.GetPendingConfirmation(sessionKey)
			if conf == nil {
				return nil
			}
			result := &struct {
				ID      string
				Message string
				Options []struct {
					ID string
				}
			}{
				ID:      conf.ID,
				Message: conf.Message,
				Options: make([]struct {
					ID string
				}, len(conf.Options)),
			}
			for i, opt := range conf.Options {
				result.Options[i] = struct {
					ID string
				}{
					ID: opt.ID,
				}
			}
			return result
		},
		RespondToConfirmation: func(response string) error {
			conf := al.interactiveMgr.GetPendingConfirmation(sessionKey)
			if conf == nil {
				return fmt.Errorf("no pending confirmation")
			}
			return al.interactiveMgr.RespondToConfirmation(conf.ID, response)
		},
		CancelConfirmation: func() error {
			conf := al.interactiveMgr.GetPendingConfirmation(sessionKey)
			if conf == nil {
				return fmt.Errorf("no pending confirmation")
			}
			return al.interactiveMgr.CancelConfirmation(conf.ID)
		},
	}

	if agent != nil {
		rt.GetModelInfo = func() (string, string) {
			return agent.Model, al.cfg.Agents.Defaults.Provider
		}
		rt.SwitchModel = func(value string) (string, error) {
			oldModel := agent.Model
			agent.Model = value
			return oldModel, nil
		}
	}
	return rt
}

// mapCommandError 将命令执行结果中的错误格式化为用户可读的错误信息。
func mapCommandError(result commands.ExecuteResult) string {
	if result.Command == "" {
		return fmt.Sprintf("Failed to execute command: %v", result.Err)
	}
	return fmt.Sprintf("Failed to execute /%s: %v", result.Command, result.Err)
}

// isExecAdmin 判断发送者是否在 tools.exec.exec_admins 列表中，
// 使用与频道 allow_from 相同的身份匹配逻辑。
// CLI 调用（空发送者）始终视为管理员。
func (al *AgentLoop) isExecAdmin(sender bus.SenderInfo) bool {
	if sender.PlatformID == "" && sender.CanonicalID == "" && sender.Username == "" {
		return true // 本地 CLI — 无频道身份，始终信任
	}
	admins := al.cfg.Tools.Exec.ExecAdmins
	for _, admin := range admins {
		if channels.MatchAllowed(sender, admin) {
			return true
		}
	}
	return false
}

// executeCmdMode 在命令模式下通过 ExecTool 执行 shell 命令。
// 输出格式化为控制台代码块以便在频道中显示。
// isAdmin 为已验证的执行管理员绕过所有命令限制。
func (al *AgentLoop) executeCmdMode(
	ctx context.Context,
	agent *AgentInstance,
	content, sessionKey, channel, chatID string,
	isAdmin bool,
) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", nil
	}

	// 特殊处理 cd 命令
	if content == "cd" || strings.HasPrefix(content, "cd ") {
		return al.handleCdCommand(content, sessionKey, agent), nil
	}

	// 拦截交互式编辑器
	if msg := interceptEditor(content); msg != "" {
		return msg, nil
	}

	// 获取工作目录
	workDir := al.getSessionWorkDir(sessionKey)
	if workDir == "" {
		workDir = agent.Workspace
	}

	execArgs := map[string]any{
		"command":     content,
		"working_dir": workDir,
	}

	// 通过 ExecTool 执行 — 管理员绕过命令限制
	var result *tools.ToolResult
	if isAdmin {
		if t, ok := agent.Tools.Get("exec"); ok {
			if et, ok := t.(*tools.ExecTool); ok {
				result = et.ExecuteUnrestricted(ctx, execArgs)
			}
		}
	}
	if result == nil {
		result = agent.Tools.ExecuteWithContext(ctx, "exec", execArgs, channel, chatID, nil)
	}

	displayDir := shortenHomePath(workDir)
	output := result.ForLLM
	if output == "" {
		output = "(no output)"
	}

	// 为 ls -l 输出添加 emoji 类型标识符（仅当用户显式使用 ls -l 时）
	if isLsCommand(content) && hasLongFlag(content) {
		output = formatLsOutput(output)
	}

	// 格式化为控制台代码块：提示符行 + 输出（显示原始命令，非修改后的）
	return fmt.Sprintf("```\n%s$ %s\n%s\n```", displayDir, content, output), nil
}

// handleCdCommand 处理命令模式下的 cd 命令，更新每个会话的工作目录。
// 特殊路径（cd、cd ~、cd /）会被重定向到工作区目录以确保安全。
// 使用 tools.ValidatePath 进行稳健的路径包含检查：filepath.Rel + filepath.IsLocal + 符号链接解析。
func (al *AgentLoop) handleCdCommand(content, sessionKey string, agent *AgentInstance) string {
	parts := strings.Fields(content)
	workspace := agent.Workspace
	var target string

	if len(parts) < 2 || parts[1] == "~" || parts[1] == "/" {
		// cd、cd ~、cd / → 始终跳转到工作区
		target = workspace
	} else {
		target = parts[1]
		// 去除空字节（纵深防御，防止绕过攻击）
		target = strings.ReplaceAll(target, "\x00", "")
		// 展开 ~ 前缀：将 ~ 视为工作区根目录（而非 $HOME）
		if strings.HasPrefix(target, "~/") {
			target = workspace + target[1:]
		}
		// 将相对路径解析为基于会话工作目录的绝对路径
		if !filepath.IsAbs(target) {
			currentDir := al.getSessionWorkDir(sessionKey)
			if currentDir == "" {
				currentDir = workspace
			}
			target = filepath.Join(currentDir, target)
		}
	}

	// 验证路径在工作区内（正确处理路径遍历、符号链接、前缀匹配）
	validated, err := tools.ValidatePath(target, workspace, true)
	if err != nil {
		// 路径逃逸工作区 — 回退到工作区根目录
		target = workspace
	} else {
		target = validated
	}

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Sprintf("cd: %s: No such file or directory", target)
	}
	if !info.IsDir() {
		return fmt.Sprintf("cd: %s: Not a directory", target)
	}

	al.setSessionWorkDir(sessionKey, target)
	return fmt.Sprintf("```\n%s$\n```", shortenHomePath(target))
}

// shortenHomePath 将路径中的用户主目录前缀替换为 ~ 以便显示。
func shortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

// handleEditCommand 处理命令模式下的 :edit 命令，用于文件查看和编辑。
// 语法：
//
//	:edit                           → 显示用法
//	:edit <file>                    → 显示带行号的文件内容
//	:edit <file> <N> <text>         → 替换第 N 行
//	:edit <file> +<N> <text>        → 在第 N 行后插入
//	:edit <file> -<N>               → 删除第 N 行
//	:edit <file> -m """<content>""" → 写入完整内容（如需则创建文件）
func (al *AgentLoop) handleEditCommand(content, workDir, workspace string) string {
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), "/edit"))
	if raw == "" {
		return editUsage()
	}

	// 按第一个换行符拆分以获取命令行
	firstLine := raw
	if idx := strings.Index(raw, "\n"); idx != -1 {
		firstLine = raw[:idx]
	}

	parts := strings.Fields(firstLine)
	if len(parts) == 0 {
		return editUsage()
	}

	filename, err := resolveEditPath(parts[0], workDir, workspace)
	if err != nil {
		return fmt.Sprintf("Access denied: %s", err)
	}

	// /edit <file> — 显示文件内容
	if len(parts) == 1 && !strings.Contains(raw, "\n") {
		return editShowFile(filename)
	}

	// /edit <file> -m """..."""
	if len(parts) >= 2 && parts[1] == "-m" {
		return editMultiline(filename, raw)
	}

	// 行操作：N text、+N text、-N
	if len(parts) >= 2 {
		// 获取行操作标记之后的原始文本（保留原始空格）
		afterFile := strings.TrimSpace(firstLine[len(parts[0]):])
		return editLineOp(filename, afterFile)
	}

	return editUsage()
}

// resolveEditPath 解析编辑路径，将相对路径基于工作目录解析，并验证是否在工作区内。
func resolveEditPath(name, workDir, workspace string) (string, error) {
	// 将 ~ 视为工作区根目录（而非 $HOME）
	if name == "~" {
		name = "."
	} else if strings.HasPrefix(name, "~/") {
		name = name[2:]
	}
	// 将相对路径基于 workDir 解析
	if !filepath.IsAbs(name) {
		name = filepath.Join(workDir, name)
	}
	// 验证路径在工作区内（阻止工作区外的绝对路径、符号链接逃逸、路径遍历）
	return tools.ValidatePath(name, workspace, true)
}

// editUsage 返回 /edit 命令的使用说明。
func editUsage() string {
	return "Usage:\n" +
		"  /edit <file>              — view file\n" +
		"  /edit <file> <N> <text>   — replace line N\n" +
		"  /edit <file> +<N> <text>  — insert after line N\n" +
		"  /edit <file> -<N>         — delete line N\n" +
		"  /edit <file> -m \"\"\"       — write content\n" +
		"  <content>\n" +
		"  \"\"\""
}

// editShowFile 读取并显示带有行号的文件内容。
func editShowFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf(
				"File not found: %s\nUse /edit %s -m \"\"\" to create it.",
				shortenHomePath(path),
				filepath.Base(path),
			)
		}
		return fmt.Sprintf("Error reading file: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	// 移除 Split 产生的末尾空行
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	const maxLines = 50
	var b strings.Builder
	b.WriteString(fmt.Sprintf("``` %s (%d lines)\n", filepath.Base(path), len(lines)))
	if len(lines) <= maxLines {
		for i, line := range lines {
			b.WriteString(fmt.Sprintf("%4d│ %s\n", i+1, line))
		}
	} else {
		for i := 0; i < maxLines; i++ {
			b.WriteString(fmt.Sprintf("%4d│ %s\n", i+1, lines[i]))
		}
		b.WriteString(fmt.Sprintf("  ...│ (%d more lines)\n", len(lines)-maxLines))
	}
	b.WriteString("```")
	return b.String()
}

// editMultiline 处理多行写入操作，将内容写入指定文件。
func editMultiline(filename, raw string) string {
	// raw = `<file> -m """..."""`
	start := strings.Index(raw, `"""`)
	if start == -1 {
		return editUsage()
	}
	rest := raw[start+3:]
	// 去除开头 """ 后的前导换行符
	rest = strings.TrimPrefix(rest, "\n")

	// 查找结束的 """
	end := strings.LastIndex(rest, `"""`)
	if end == -1 || end == 0 {
		// 没有结束的三引号 — 将剩余内容全部作为内容
		end = len(rest)
	}
	content := rest[:end]

	// 确保末尾有换行符
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	// 如需则创建父目录
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("Error creating directory: %v", err)
	}

	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err)
	}

	lineCount := strings.Count(content, "\n")
	return fmt.Sprintf("```\n✓ Wrote %d lines → %s\n```", lineCount, shortenHomePath(filename))
}

// editLineOp 执行单行操作（替换、插入或删除）。
func editLineOp(filename, rawArgs string) string {
	rawArgs = strings.TrimSpace(rawArgs)
	// 拆分为操作标记和文本
	spaceIdx := strings.IndexByte(rawArgs, ' ')
	var op, text string
	if spaceIdx == -1 {
		op = rawArgs
	} else {
		op = rawArgs[:spaceIdx]
		text = rawArgs[spaceIdx+1:]
	}

	var lineNum int
	var action string // "replace"、"insert"、"delete"
	var err error

	if strings.HasPrefix(op, "+") {
		action = "insert"
		lineNum, err = strconv.Atoi(op[1:])
	} else if strings.HasPrefix(op, "-") {
		action = "delete"
		lineNum, err = strconv.Atoi(op[1:])
	} else {
		action = "replace"
		lineNum, err = strconv.Atoi(op)
	}
	if err != nil || lineNum < 1 {
		return "Invalid line number. Use a positive integer."
	}

	// 读取现有文件
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("File not found: %s", shortenHomePath(filename))
		}
		return fmt.Sprintf("Error reading file: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	switch action {
	case "delete":
		if lineNum > len(lines) {
			return fmt.Sprintf("Line %d out of range (file has %d lines).", lineNum, len(lines))
		}
		deleted := lines[lineNum-1]
		lines = append(lines[:lineNum-1], lines[lineNum:]...)
		if err := os.WriteFile(filename, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return fmt.Sprintf("Error writing file: %v", err)
		}
		return fmt.Sprintf("```\n✓ Deleted line %d: %s\n(%d lines remaining)\n```", lineNum, deleted, len(lines))

	case "replace":
		if text == "" {
			return "Usage: :edit <file> <N> <text>"
		}
		if lineNum > len(lines) {
			return fmt.Sprintf("Line %d out of range (file has %d lines).", lineNum, len(lines))
		}
		old := lines[lineNum-1]
		lines[lineNum-1] = text
		if err := os.WriteFile(filename, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return fmt.Sprintf("Error writing file: %v", err)
		}
		return fmt.Sprintf("```\n✓ Line %d replaced\n  was: %s\n  now: %s\n```", lineNum, old, text)

	case "insert":
		if text == "" {
			return "Usage: :edit <file> +<N> <text>"
		}
		if lineNum > len(lines) {
			lineNum = len(lines) // 在末尾插入
		}
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:lineNum]...)
		newLines = append(newLines, text)
		newLines = append(newLines, lines[lineNum:]...)
		if err := os.WriteFile(filename, []byte(strings.Join(newLines, "\n")+"\n"), 0o644); err != nil {
			return fmt.Sprintf("Error writing file: %v", err)
		}
		return fmt.Sprintf("```\n✓ Inserted after line %d: %s\n(%d lines total)\n```", lineNum, text, len(newLines))
	}

	return editUsage()
}

// interceptEditor 检测交互式编辑器命令并返回友好的重定向提示信息。
func interceptEditor(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	name := parts[0]
	switch name {
	case "vim", "vi", "nvim", "nano", "emacs", "pico", "joe", "mcedit":
		return fmt.Sprintf("⚠ %s requires a terminal and cannot run here.\nUse /edit instead:\n\n"+
			"/edit <file>              — view file\n"+
			"/edit <file> -m \"\"\"       — write content\n"+
			"<content>\n"+
			"\"\"\"\n\n"+
			"Type :help for all commands.", name)
	}
	return ""
}

// isLsCommand 检查 shell 命令是否为 ls 调用。
func isLsCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	return cmd == "ls" || strings.HasPrefix(cmd, "ls ")
}

// hasLongFlag 检查 ls 命令是否包含 -l 标志。
func hasLongFlag(cmd string) bool {
	for _, p := range strings.Fields(cmd)[1:] {
		if strings.HasPrefix(p, "-") && !strings.HasPrefix(p, "--") && strings.ContainsRune(p, 'l') {
			return true
		}
	}
	return false
}

// formatLsOutput 为 ls -l 样式的输出行添加 emoji 类型标识符。
func formatLsOutput(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = formatLsLine(line)
	}
	return strings.Join(lines, "\n")
}

// formatLsLine 根据文件类型为单行 ls -l 输出添加 emoji 前缀。
func formatLsLine(line string) string {
	// 跳过空行、"total" 行和过短的行（不符合 ls -l 格式）
	if line == "" || strings.HasPrefix(line, "total ") || len(line) < 10 {
		return line
	}

	// 检查行是否以权限字符串开头（如 drwxr-xr-x）
	perms := line[:10]
	if !isPermString(perms) {
		return line
	}

	fileType := perms[0]
	var emoji string
	switch fileType {
	case 'd':
		emoji = "\U0001F4C1" // 📁
	case 'l':
		emoji = "\U0001F517" // 🔗
	case 'b', 'c':
		emoji = "\U0001F4BE" // 💾
	case 'p', 's':
		emoji = "\U0001F50C" // 🔌
	default:
		// 普通文件：检查可执行位（属主/属组/其他用户的 x 位置）
		if perms[3] == 'x' || perms[6] == 'x' || perms[9] == 'x' {
			emoji = "\u26A1" // ⚡
		} else {
			emoji = fileEmojiByExt(line)
		}
	}

	return emoji + " " + line
}

// isPermString 检查 10 字符的字符串是否类似 Unix 权限字符串。
func isPermString(s string) bool {
	if len(s) != 10 {
		return false
	}
	// 第一个字符：文件类型
	switch s[0] {
	case '-', 'd', 'l', 'b', 'c', 'p', 's':
	default:
		return false
	}
	// 后续 9 个字符：rwx 或 -（加上 s/S/t/T 用于 setuid/setgid/sticky 位）
	for _, c := range s[1:] {
		switch c {
		case 'r', 'w', 'x', '-', 's', 'S', 't', 'T':
		default:
			return false
		}
	}
	return true
}

// fileEmojiByExt 根据 ls -l 行中的文件扩展名返回对应的 emoji。
func fileEmojiByExt(line string) string {
	// 提取文件名：最后一个以空白分隔的字段（对于符号链接，取 " -> " 之前的部分）
	name := line
	if idx := strings.LastIndex(line, " -> "); idx != -1 {
		name = line[:idx]
	}
	if idx := strings.LastIndex(name, " "); idx != -1 {
		name = name[idx+1:]
	}
	name = strings.ToLower(name)

	ext := filepath.Ext(name)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp", ".bmp", ".ico", ".tiff":
		return "\U0001F5BC" // 🖼
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a":
		return "\U0001F3B5" // 🎵
	case ".mp4", ".avi", ".mkv", ".mov", ".webm", ".flv", ".wmv":
		return "\U0001F3AC" // 🎬
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".zst", ".tgz":
		return "\U0001F4E6" // 📦
	default:
		return "\U0001F4C4" // 📄
	}
}
