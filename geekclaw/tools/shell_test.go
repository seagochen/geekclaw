package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// TestShellTool_Success 验证命令成功执行的情况
func TestShellTool_Success(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	args := map[string]any{
		"command": "echo 'hello world'",
	}

	result := tool.Execute(ctx, args)

	// 成功时不应返回错误
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// ForUser 应包含命令输出
	if !strings.Contains(result.ForUser, "hello world") {
		t.Errorf("Expected ForUser to contain 'hello world', got: %s", result.ForUser)
	}

	// ForLLM 应包含完整输出
	if !strings.Contains(result.ForLLM, "hello world") {
		t.Errorf("Expected ForLLM to contain 'hello world', got: %s", result.ForLLM)
	}
}

// TestShellTool_Failure 验证命令执行失败的情况
func TestShellTool_Failure(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	args := map[string]any{
		"command": "ls /nonexistent_directory_12345",
	}

	result := tool.Execute(ctx, args)

	// 失败应标记为错误
	if !result.IsError {
		t.Errorf("Expected error for failed command, got IsError=false")
	}

	// ForUser 应包含错误信息
	if result.ForUser == "" {
		t.Errorf("Expected ForUser to contain error info, got empty string")
	}

	// ForLLM 应包含退出码或错误信息
	if !strings.Contains(result.ForLLM, "Exit code") && result.ForUser == "" {
		t.Errorf("Expected ForLLM to contain exit code or error, got: %s", result.ForLLM)
	}
}

// TestShellTool_Timeout 验证命令超时处理
func TestShellTool_Timeout(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	tool.SetTimeout(100 * time.Millisecond)

	ctx := context.Background()
	args := map[string]any{
		"command": "sleep 10",
	}

	result := tool.Execute(ctx, args)

	// 超时应标记为错误
	if !result.IsError {
		t.Errorf("Expected error for timeout, got IsError=false")
	}

	// 应提示超时
	if !strings.Contains(result.ForLLM, "timed out") && !strings.Contains(result.ForUser, "timed out") {
		t.Errorf("Expected timeout message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestShellTool_WorkingDir 验证自定义工作目录
func TestShellTool_WorkingDir(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0o644)

	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	args := map[string]any{
		"command":     "cat test.txt",
		"working_dir": tmpDir,
	}

	result := tool.Execute(ctx, args)

	if result.IsError {
		t.Errorf("Expected success in custom working dir, got error: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForUser, "test content") {
		t.Errorf("Expected output from custom dir, got: %s", result.ForUser)
	}
}

// TestShellTool_DangerousCommand 验证安全防护能阻止危险命令
func TestShellTool_DangerousCommand(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	args := map[string]any{
		"command": "rm -rf /",
	}

	result := tool.Execute(ctx, args)

	// 危险命令应被阻止
	if !result.IsError {
		t.Errorf("Expected dangerous command to be blocked (IsError=true)")
	}

	if !strings.Contains(result.ForLLM, "blocked") && !strings.Contains(result.ForUser, "blocked") {
		t.Errorf("Expected 'blocked' message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

func TestShellTool_DangerousCommand_KillBlocked(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	args := map[string]any{
		"command": "kill 12345",
	}

	result := tool.Execute(ctx, args)
	if !result.IsError {
		t.Errorf("Expected kill command to be blocked")
	}
	if !strings.Contains(result.ForLLM, "blocked") && !strings.Contains(result.ForUser, "blocked") {
		t.Errorf("Expected blocked message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestShellTool_MissingCommand 验证缺少 command 参数时的错误处理
func TestShellTool_MissingCommand(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	args := map[string]any{}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when command is missing")
	}
}

// TestShellTool_StderrCapture 验证 stderr 被捕获并包含在输出中
func TestShellTool_StderrCapture(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	args := map[string]any{
		"command": "sh -c 'echo stdout; echo stderr >&2'",
	}

	result := tool.Execute(ctx, args)

	// stdout 和 stderr 都应包含在输出中
	if !strings.Contains(result.ForLLM, "stdout") {
		t.Errorf("Expected stdout in output, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "stderr") {
		t.Errorf("Expected stderr in output, got: %s", result.ForLLM)
	}
}

// TestShellTool_OutputTruncation 验证长输出会被截断
func TestShellTool_OutputTruncation(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	ctx := context.Background()
	// 生成长输出（>10000 字符）
	args := map[string]any{
		"command": "python3 -c \"print('x' * 20000)\" || echo " + strings.Repeat("x", 20000),
	}

	result := tool.Execute(ctx, args)

	// 应包含截断提示或输出已被截断
	if len(result.ForLLM) > 15000 {
		t.Errorf("Expected output to be truncated, got length: %d", len(result.ForLLM))
	}
}

// TestShellTool_WorkingDir_OutsideWorkspace 验证 working_dir 无法直接逃脱工作区
func TestShellTool_WorkingDir_OutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	tool, err := NewExecTool(workspace, true)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"command":     "pwd",
		"working_dir": outsideDir,
	})

	if !result.IsError {
		t.Fatalf("expected working_dir outside workspace to be blocked, got output: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "blocked") {
		t.Errorf("expected 'blocked' in error, got: %s", result.ForLLM)
	}
}

// TestShellTool_WorkingDir_SymlinkEscape 验证工作区内指向外部的符号链接
// 不能作为 working_dir 用于逃脱沙箱。
func TestShellTool_WorkingDir_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	secretDir := filepath.Join(root, "secret")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("failed to create secret dir: %v", err)
	}
	os.WriteFile(filepath.Join(secretDir, "secret.txt"), []byte("top secret"), 0o644)

	// 符号链接位于工作区内，但解析后指向工作区外的 secretDir
	link := filepath.Join(workspace, "escape")
	if err := os.Symlink(secretDir, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	tool, err := NewExecTool(workspace, true)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"command":     "cat secret.txt",
		"working_dir": link,
	})

	if !result.IsError {
		t.Fatalf("expected symlink working_dir escape to be blocked, got output: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "blocked") {
		t.Errorf("expected 'blocked' in error, got: %s", result.ForLLM)
	}
}

// TestShellTool_RemoteChannelBlockedByDefault 验证远程频道默认被阻止执行命令
func TestShellTool_RemoteChannelBlockedByDefault(t *testing.T) {
	cfg := &config.Config{}
	cfg.Tools.Exec.AllowRemote = false

	tool, err := NewExecToolWithConfig("", false, cfg)
	if err != nil {
		t.Fatalf("NewExecToolWithConfig() error: %v", err)
	}
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{"command": "echo hi"})

	if !result.IsError {
		t.Fatal("expected remote-channel exec to be blocked")
	}
	if !strings.Contains(result.ForLLM, "restricted to internal channels") {
		t.Errorf("expected 'restricted to internal channels' message, got: %s", result.ForLLM)
	}
}

// TestShellTool_InternalChannelAllowed 验证内部频道允许执行命令
func TestShellTool_InternalChannelAllowed(t *testing.T) {
	cfg := &config.Config{}
	cfg.Tools.Exec.AllowRemote = false

	tool, err := NewExecToolWithConfig("", false, cfg)
	if err != nil {
		t.Fatalf("NewExecToolWithConfig() error: %v", err)
	}
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{"command": "echo hi"})

	if result.IsError {
		t.Fatalf("expected internal channel exec to succeed, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "hi") {
		t.Errorf("expected output to contain 'hi', got: %s", result.ForLLM)
	}
}

// TestShellTool_EmptyChannelBlockedWhenNotAllowRemote 验证无频道上下文时默认拒绝执行（故障关闭）
func TestShellTool_EmptyChannelBlockedWhenNotAllowRemote(t *testing.T) {
	cfg := &config.Config{}
	cfg.Tools.Exec.AllowRemote = false

	tool, err := NewExecToolWithConfig("", false, cfg)
	if err != nil {
		t.Fatalf("NewExecToolWithConfig() error: %v", err)
	}
	result := tool.Execute(context.Background(), map[string]any{
		"command": "echo hi",
	})

	if !result.IsError {
		t.Fatal("expected exec with empty channel to be blocked when allowRemote=false")
	}
}

// TestShellTool_AllowRemoteBypassesChannelCheck 验证 allowRemote=true 时允许任意频道
func TestShellTool_AllowRemoteBypassesChannelCheck(t *testing.T) {
	cfg := &config.Config{}
	cfg.Tools.Exec.AllowRemote = true

	tool, err := NewExecToolWithConfig("", false, cfg)
	if err != nil {
		t.Fatalf("NewExecToolWithConfig() error: %v", err)
	}
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{"command": "echo hi"})

	if result.IsError {
		t.Fatalf("expected allowRemote=true to permit remote channel, got: %s", result.ForLLM)
	}
}

// TestShellTool_RestrictToWorkspace 验证工作区限制功能
func TestShellTool_RestrictToWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, false)
	if err != nil {
		t.Errorf("unable to configure exec tool: %s", err)
	}

	tool.SetRestrictToWorkspace(true)

	ctx := context.Background()
	args := map[string]any{
		"command": "cat ../../etc/passwd",
	}

	result := tool.Execute(ctx, args)

	// 路径穿越应被阻止
	if !result.IsError {
		t.Errorf("Expected path traversal to be blocked with restrictToWorkspace=true")
	}

	if !strings.Contains(result.ForLLM, "blocked") && !strings.Contains(result.ForUser, "blocked") {
		t.Errorf(
			"Expected 'blocked' message for path traversal, got ForLLM: %s, ForUser: %s",
			result.ForLLM,
			result.ForUser,
		)
	}
}

// TestGuardCommand_DotSlashExecutable 验证 ./executable 风格的命令
// 不被 guardCommand 中的路径提取正则表达式阻止。
func TestGuardCommand_DotSlashExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, true)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}

	// 在工作区中创建测试脚本
	scriptPath := filepath.Join(tmpDir, "test.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok"), 0o755)

	ctx := context.Background()
	result := tool.Execute(ctx, map[string]any{
		"command":     "./test.sh",
		"working_dir": tmpDir,
	})

	if result.IsError {
		t.Errorf("Expected ./test.sh to be allowed, got error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "ok") {
		t.Errorf("Expected output 'ok', got: %s", result.ForLLM)
	}
}

// TestGuardCommand_URLEncodedTraversal 验证 URL 编码的路径穿越序列
// （%2e%2e%2f → ../）能被检测并阻止。
func TestGuardCommand_URLEncodedTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, true)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}
	tool.SetRestrictToWorkspace(true)

	msg := tool.guardCommand("cat %2e%2e%2f%2e%2e%2fetc/passwd", tmpDir, false)
	if msg == "" {
		t.Error("Expected URL-encoded path traversal to be blocked")
	}
}

// TestGuardCommand_NullByte 验证命令中的空字节在守卫检查前被清除，
// 从而无法绕过路径穿越检测。
func TestGuardCommand_NullByte(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, true)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}
	tool.SetRestrictToWorkspace(true)

	msg := tool.guardCommand("cat foo\x00../../etc/passwd", tmpDir, false)
	if msg == "" {
		t.Error("Expected null-byte traversal to be blocked")
	}
}

// TestGuardCommand_SuBlocked 验证 su 及相关提权命令
// 被拒绝模式阻止。
func TestGuardCommand_SuBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, true)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}

	cases := []string{"su", "su -", "su root", "doas ls", "pkexec /bin/bash"}
	for _, cmd := range cases {
		msg := tool.guardCommand(cmd, tmpDir, false)
		if msg == "" {
			t.Errorf("Expected %q to be blocked", cmd)
		}
	}
}

// TestGuardCommand_SuNoFalsePositive 验证包含 "su" 子字符串的单词
// 不会被误判阻止（如 summary、result、surplus）。
func TestGuardCommand_SuNoFalsePositive(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, false) // 本测试不启用工作区限制
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}

	cases := []string{"echo surplus", "cat summary.txt", "ls result/"}
	for _, cmd := range cases {
		msg := tool.guardCommand(cmd, tmpDir, false)
		if msg != "" {
			t.Errorf("Expected %q to NOT be blocked, got: %s", cmd, msg)
		}
	}
}

// TestShellTool_DevNullAllowed 验证 /dev/null 重定向不被阻止（issue #964）。
func TestShellTool_DevNullAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, true)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}

	commands := []string{
		"echo hello 2>/dev/null",
		"echo hello >/dev/null",
		"echo hello > /dev/null",
		"echo hello 2> /dev/null",
		"echo hello >/dev/null 2>&1",
		"find " + tmpDir + " -name '*.go' 2>/dev/null",
	}

	for _, cmd := range commands {
		result := tool.Execute(context.Background(), map[string]any{"command": cmd})
		if result.IsError && strings.Contains(result.ForLLM, "blocked") {
			t.Errorf("command should not be blocked: %s\n  error: %s", cmd, result.ForLLM)
		}
	}
}

// TestShellTool_BlockDevices 验证向块设备的写入被阻止（issue #965）。
func TestShellTool_BlockDevices(t *testing.T) {
	tool, err := NewExecTool("", false)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}

	blocked := []string{
		"echo x > /dev/sda",
		"echo x > /dev/hda",
		"echo x > /dev/vda",
		"echo x > /dev/xvda",
		"echo x > /dev/nvme0n1",
		"echo x > /dev/mmcblk0",
		"echo x > /dev/loop0",
		"echo x > /dev/dm-0",
		"echo x > /dev/md0",
		"echo x > /dev/sr0",
		"echo x > /dev/nbd0",
	}

	for _, cmd := range blocked {
		result := tool.Execute(context.Background(), map[string]any{"command": cmd})
		if !result.IsError {
			t.Errorf("expected block device write to be blocked: %s", cmd)
		}
	}
}

// TestShellTool_SafePathsInWorkspaceRestriction 验证即使工作区限制启用，
// 安全的内核伪设备仍被允许访问。
func TestShellTool_SafePathsInWorkspaceRestriction(t *testing.T) {
	tmpDir := t.TempDir()
	tool, err := NewExecTool(tmpDir, true)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}

	// 这些路径指向工作区外，但应通过 safePaths 白名单被允许。
	commands := []string{
		"cat /dev/urandom | head -c 16 | od",
		"echo test > /dev/null",
		"dd if=/dev/zero bs=1 count=1",
	}

	for _, cmd := range commands {
		result := tool.Execute(context.Background(), map[string]any{"command": cmd})
		if result.IsError && strings.Contains(result.ForLLM, "path outside working dir") {
			t.Errorf("safe path should not be blocked by workspace check: %s\n  error: %s", cmd, result.ForLLM)
		}
	}
}

// TestShellTool_CustomAllowPatterns 验证自定义允许模式能豁免
// 命令不受拒绝模式检查。
func TestShellTool_CustomAllowPatterns(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Exec: config.ExecConfig{
				CustomAllowPatterns: []string{`\bgit\s+push\s+origin\b`},
			},
		},
	}

	tool, err := NewExecToolWithConfig("", false, cfg)
	if err != nil {
		t.Fatalf("unable to configure exec tool: %s", err)
	}

	// "git push origin main" 应被自定义允许模式豁免。
	result := tool.Execute(context.Background(), map[string]any{
		"command": "git push origin main",
	})
	if result.IsError && strings.Contains(result.ForLLM, "blocked") {
		t.Errorf("custom allow pattern should exempt 'git push origin main', got: %s", result.ForLLM)
	}

	// "git push upstream main" 仍应被拒绝模式阻止（不匹配允许模式）。
	result = tool.Execute(context.Background(), map[string]any{
		"command": "git push upstream main",
	})
	if !result.IsError {
		t.Errorf("'git push upstream main' should still be blocked by deny pattern")
	}
}

