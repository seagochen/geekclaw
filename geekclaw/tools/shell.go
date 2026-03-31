package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// ExecTool 执行 shell 命令并返回其输出。
type ExecTool struct {
	workingDir          string
	timeout             time.Duration
	denyPatterns        []*regexp.Regexp
	allowPatterns       []*regexp.Regexp
	customAllowPatterns []*regexp.Regexp
	restrictToWorkspace bool
	allowRemote bool
}

var (
	// defaultDenyPatterns 是默认的危险命令拒绝模式列表。
	defaultDenyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\brm\s+-[rf]{1,2}\b`),
		regexp.MustCompile(`\bdel\s+/[fq]\b`),
		regexp.MustCompile(`\brmdir\s+/s\b`),
		// 匹配磁盘擦除命令（必须后接空格/参数）
		regexp.MustCompile(
			`\b(format|mkfs|diskpart)\b\s`,
		),
		regexp.MustCompile(`\bdd\s+if=`),
		// 阻止写入块设备（所有常见命名方案）。
		regexp.MustCompile(
			`>\s*/dev/(sd[a-z]|hd[a-z]|vd[a-z]|xvd[a-z]|nvme\d|mmcblk\d|loop\d|dm-\d|md\d|sr\d|nbd\d)`,
		),
		regexp.MustCompile(`\b(shutdown|reboot|poweroff)\b`),
		regexp.MustCompile(`:\(\)\s*\{.*\};\s*:`),
		regexp.MustCompile(`\|\s*sh\b`),
		regexp.MustCompile(`\|\s*bash\b`),
		regexp.MustCompile(`;\s*rm\s+-[rf]`),
		regexp.MustCompile(`&&\s*rm\s+-[rf]`),
		regexp.MustCompile(`\|\|\s*rm\s+-[rf]`),
		regexp.MustCompile(`\bsudo\b`),
		regexp.MustCompile(`\bsu\b`),
		regexp.MustCompile(`\bdoas\b`),
		regexp.MustCompile(`\bpkexec\b`),
		regexp.MustCompile(`\bchown\b`),
		regexp.MustCompile(`\bpkill\b`),
		regexp.MustCompile(`\bkillall\b`),
		regexp.MustCompile(`\bkill\s+-[9]\b`),
		regexp.MustCompile(`\bcurl\b.*\|\s*(sh|bash)`),
		regexp.MustCompile(`\bwget\b.*\|\s*(sh|bash)`),
		regexp.MustCompile(`\bnpm\s+install\s+-g\b`),
		regexp.MustCompile(`\bpip\s+install\s+--user\b`),
		regexp.MustCompile(`\bapt\s+(install|remove|purge)\b`),
		regexp.MustCompile(`\byum\s+(install|remove)\b`),
		regexp.MustCompile(`\bdnf\s+(install|remove)\b`),
		regexp.MustCompile(`\bdocker\s+run\b`),
		regexp.MustCompile(`\bdocker\s+exec\b`),
		regexp.MustCompile(`\bssh\b.*@`),
		regexp.MustCompile(`\beval\b`),
	}

	// absolutePathPattern 匹配命令中的绝对文件路径（Unix 和 Windows）。
	absolutePathPattern = regexp.MustCompile(`[A-Za-z]:\\[^\\\"']+|/[^\s\"']+`)

	// safePaths 是内核伪设备，无论工作区限制如何都可以安全引用。
	// 它们不包含用户数据，也不会导致破坏性写入。
	safePaths = map[string]bool{
		"/dev/null":    true,
		"/dev/zero":    true,
		"/dev/random":  true,
		"/dev/urandom": true,
		"/dev/stdin":   true,
		"/dev/stdout":  true,
		"/dev/stderr":  true,
	}
)

// NewExecTool 创建一个新的 ExecTool。
func NewExecTool(workingDir string, restrict bool) (*ExecTool, error) {
	return NewExecToolWithConfig(workingDir, restrict, nil)
}

// NewExecToolWithConfig 使用配置创建一个新的 ExecTool。
func NewExecToolWithConfig(workingDir string, restrict bool, config *config.Config) (*ExecTool, error) {
	denyPatterns := append([]*regexp.Regexp(nil), defaultDenyPatterns...)
	customAllowPatterns := make([]*regexp.Regexp, 0)
	allowRemote := true

	if config != nil {
		execConfig := config.Tools.Exec
		allowRemote = execConfig.AllowRemote
		if len(execConfig.CustomDenyPatterns) > 0 {
			fmt.Printf("Using custom deny patterns: %v\n", execConfig.CustomDenyPatterns)
			for _, pattern := range execConfig.CustomDenyPatterns {
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid custom deny pattern %q: %w", pattern, err)
				}
				denyPatterns = append(denyPatterns, re)
			}
		}
		for _, pattern := range execConfig.CustomAllowPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid custom allow pattern %q: %w", pattern, err)
			}
			customAllowPatterns = append(customAllowPatterns, re)
		}
	}

	timeout := 60 * time.Second
	if config != nil && config.Tools.Exec.TimeoutSeconds > 0 {
		timeout = time.Duration(config.Tools.Exec.TimeoutSeconds) * time.Second
	}

	return &ExecTool{
		workingDir:          workingDir,
		timeout:             timeout,
		denyPatterns:        denyPatterns,
		allowPatterns:       nil,
		customAllowPatterns: customAllowPatterns,
		restrictToWorkspace: restrict,
		allowRemote: allowRemote,
	}, nil
}

// Name 返回工具名称。
func (t *ExecTool) Name() string {
	return "exec"
}

// Description 返回工具描述。
func (t *ExecTool) Description() string {
	return "Execute a shell command and return its output. Use with caution."
}

// Parameters 返回工具参数的 schema。
func (t *ExecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Optional working directory for the command",
			},
		},
		"required": []string{"command"},
	}
}

// ExecuteUnrestricted 绕过所有命令防护执行命令。
// 仅在调用者已验证发送者是执行管理员后使用。
func (t *ExecTool) ExecuteUnrestricted(ctx context.Context, args map[string]any) *ToolResult {
	return t.execute(ctx, args, true)
}

// Execute 执行 shell 命令。
func (t *ExecTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	return t.execute(ctx, args, false)
}

// execute 是执行命令的内部实现。
func (t *ExecTool) execute(ctx context.Context, args map[string]any, adminOverride bool) *ToolResult {
	command, ok := args["command"].(string)
	if !ok {
		return ErrorResult("command is required")
	}

	// GHSA-pv8c-p6jf-3fpp: 阻止来自远程频道（如 Telegram webhooks）的 exec，
	// 除非通过配置显式启用。默认安全：空频道 = 阻止。
	if !t.allowRemote {
		channel := ToolChannel(ctx)
		if channel == "" {
			channel, _ = args["__channel"].(string)
		}
		channel = strings.TrimSpace(channel)
		if channel == "" || !channels.IsInternalChannel(channel) {
			return ErrorResult("exec is restricted to internal channels")
		}
	}

	cwd := t.workingDir
	if wd, ok := args["working_dir"].(string); ok && wd != "" {
		if t.restrictToWorkspace && t.workingDir != "" {
			resolvedWD, err := ValidatePath(wd, t.workingDir, true)
			if err != nil {
				return ErrorResult("Command blocked by safety guard (" + err.Error() + ")")
			}
			cwd = resolvedWD
		} else {
			cwd = wd
		}
	}

	if cwd == "" {
		wd, err := os.Getwd()
		if err == nil {
			cwd = wd
		}
	}

	if guardError := t.guardCommand(command, cwd, adminOverride); guardError != "" {
		return ErrorResult(guardError)
	}

	// 在执行前重新解析符号链接，缩小验证和 cmd.Dir 赋值之间的 TOCTOU 窗口
	if t.restrictToWorkspace && t.workingDir != "" && cwd != t.workingDir {
		resolved, err := filepath.EvalSymlinks(cwd)
		if err != nil {
			return ErrorResult(fmt.Sprintf("Command blocked by safety guard (path resolution failed: %v)", err))
		}
		absWorkspace, _ := filepath.Abs(t.workingDir)
		wsResolved, _ := filepath.EvalSymlinks(absWorkspace)
		if wsResolved == "" {
			wsResolved = absWorkspace
		}
		rel, err := filepath.Rel(wsResolved, resolved)
		if err != nil || !filepath.IsLocal(rel) {
			return ErrorResult("Command blocked by safety guard (working directory escaped workspace)")
		}
		cwd = resolved
	}

	// timeout == 0 表示无超时
	var cmdCtx context.Context
	var cancel context.CancelFunc
	if t.timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, t.timeout)
	} else {
		cmdCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	}
	if cwd != "" {
		cmd.Dir = cwd
	}

	prepareCommandForTermination(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return ErrorResult(fmt.Sprintf("failed to start command: %v", err))
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-cmdCtx.Done():
		_ = terminateProcessTree(cmd)
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			err = <-done
		}
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	if err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return UserErrorResult(fmt.Sprintf("Command timed out after %v", t.timeout))
		}
		output += fmt.Sprintf("\nExit code: %v", err)
	}

	if output == "" {
		output = "(no output)"
	}

	maxLen := 10000
	if len(output) > maxLen {
		output = output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
	}

	if err != nil {
		return UserErrorResult(output)
	}

	return UserResult(output)
}

// sanitizeCommand 去除空字节并解码 URL 编码序列，
// 以便编码的遍历模式（如 %2e%2e%2f）能被防护检测到。
func sanitizeCommand(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\x00", "")
	if decoded, err := url.PathUnescape(cmd); err == nil {
		cmd = decoded
	}
	return cmd
}

// guardCommand 检查命令是否被安全防护阻止。
func (t *ExecTool) guardCommand(command, cwd string, adminOverride bool) string {
	// 执行管理员：完全无限制。
	if adminOverride {
		return ""
	}

	cmd := sanitizeCommand(strings.TrimSpace(command))
	lower := strings.ToLower(cmd)

	// 自定义允许模式可以使命令免于拒绝检查。
	explicitlyAllowed := false
	for _, pattern := range t.customAllowPatterns {
		if pattern.MatchString(lower) {
			explicitlyAllowed = true
			break
		}
	}

	if !explicitlyAllowed {
		for _, pattern := range t.denyPatterns {
			if pattern.MatchString(lower) {
				return "Command blocked by safety guard (dangerous pattern detected)"
			}
		}
	}

	if len(t.allowPatterns) > 0 {
		allowed := false
		for _, pattern := range t.allowPatterns {
			if pattern.MatchString(lower) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "Command blocked by safety guard (not in allowlist)"
		}
	}

	if t.restrictToWorkspace {
		if strings.Contains(cmd, "..\\") || strings.Contains(cmd, "../") {
			return "Command blocked by safety guard (path traversal detected)"
		}

		cwdPath, err := filepath.Abs(cwd)
		if err != nil {
			return ""
		}

		matchIndices := absolutePathPattern.FindAllStringIndex(cmd, -1)

		for _, loc := range matchIndices {
			raw := cmd[loc[0]:loc[1]]
			// 跳过相对路径如 ./executable——正则从 "./executable" 提取
			// "/executable" 但它不是绝对路径。
			if loc[0] > 0 && cmd[loc[0]-1] == '.' {
				continue
			}

			p, err := filepath.Abs(raw)
			if err != nil {
				continue
			}

			if safePaths[p] {
				continue
			}

			rel, err := filepath.Rel(cwdPath, p)
			if err != nil {
				continue
			}

			if strings.HasPrefix(rel, "..") {
				return "Command blocked by safety guard (path outside working dir)"
			}
		}
	}

	return ""
}

// SetTimeout 设置命令执行超时时间。
func (t *ExecTool) SetTimeout(timeout time.Duration) {
	t.timeout = timeout
}

// SetRestrictToWorkspace 设置是否限制到工作目录。
func (t *ExecTool) SetRestrictToWorkspace(restrict bool) {
	t.restrictToWorkspace = restrict
}

// SetAllowPatterns 设置允许命令模式列表。
func (t *ExecTool) SetAllowPatterns(patterns []string) error {
	t.allowPatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("invalid allow pattern %q: %w", p, err)
		}
		t.allowPatterns = append(t.allowPatterns, re)
	}
	return nil
}
