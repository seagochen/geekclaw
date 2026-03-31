package tools

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFilesystemTool_ReadFile_Success 验证文件读取成功的情况
func TestFilesystemTool_ReadFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0o644)

	tool := NewReadFileTool("", false, MaxReadFileSize)
	ctx := context.Background()
	args := map[string]any{
		"path": testFile,
	}

	result := tool.Execute(ctx, args)

	// 成功时不应返回错误
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// ForLLM 应包含文件内容
	if !strings.Contains(result.ForLLM, "test content") {
		t.Errorf("Expected ForLLM to contain 'test content', got: %s", result.ForLLM)
	}

	// ReadFile 返回 NewToolResult，仅设置 ForLLM，不设置 ForUser
	// 这是预期行为——文件内容发送给 LLM，不直接显示给用户
	if result.ForUser != "" {
		t.Errorf("Expected ForUser to be empty for NewToolResult, got: %s", result.ForUser)
	}
}

// TestFilesystemTool_ReadFile_NotFound 验证文件不存在时的错误处理
func TestFilesystemTool_ReadFile_NotFound(t *testing.T) {
	tool := NewReadFileTool("", false, MaxReadFileSize)
	ctx := context.Background()
	args := map[string]any{
		"path": "/nonexistent_file_12345.txt",
	}

	result := tool.Execute(ctx, args)

	// 失败应标记为错误
	if !result.IsError {
		t.Errorf("Expected error for missing file, got IsError=false")
	}

	// 应包含错误信息
	if !strings.Contains(result.ForLLM, "failed to open file") && !strings.Contains(result.ForUser, "failed to read") {
		t.Errorf("Expected error message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestFilesystemTool_ReadFile_MissingPath 验证缺少 path 参数时的错误处理
func TestFilesystemTool_ReadFile_MissingPath(t *testing.T) {
	tool := &ReadFileTool{}
	ctx := context.Background()
	args := map[string]any{}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when path is missing")
	}

	// 应提示必填参数缺失
	if !strings.Contains(result.ForLLM, "path is required") && !strings.Contains(result.ForUser, "path is required") {
		t.Errorf("Expected 'path is required' message, got ForLLM: %s", result.ForLLM)
	}
}

// TestFilesystemTool_WriteFile_Success 验证文件写入成功的情况
func TestFilesystemTool_WriteFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "newfile.txt")

	tool := NewWriteFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path":    testFile,
		"content": "hello world",
	}

	result := tool.Execute(ctx, args)

	// 成功时不应返回错误
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// WriteFile 应返回 SilentResult
	if !result.Silent {
		t.Errorf("Expected Silent=true for WriteFile, got false")
	}

	// ForUser 应为空（静默结果）
	if result.ForUser != "" {
		t.Errorf("Expected ForUser to be empty for SilentResult, got: %s", result.ForUser)
	}

	// 验证文件已实际写入
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("Expected file content 'hello world', got: %s", string(content))
	}
}

// TestFilesystemTool_WriteFile_CreateDir 验证目录自动创建功能
func TestFilesystemTool_WriteFile_CreateDir(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "subdir", "newfile.txt")

	tool := NewWriteFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path":    testFile,
		"content": "test",
	}

	result := tool.Execute(ctx, args)

	// 成功时不应返回错误
	if result.IsError {
		t.Errorf("Expected success with directory creation, got IsError=true: %s", result.ForLLM)
	}

	// 验证目录已创建且文件已写入
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(content) != "test" {
		t.Errorf("Expected file content 'test', got: %s", string(content))
	}
}

// TestFilesystemTool_WriteFile_MissingPath 验证缺少 path 参数时的错误处理
func TestFilesystemTool_WriteFile_MissingPath(t *testing.T) {
	tool := NewWriteFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"content": "test",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when path is missing")
	}
}

// TestFilesystemTool_WriteFile_MissingContent 验证缺少 content 参数时的错误处理
func TestFilesystemTool_WriteFile_MissingContent(t *testing.T) {
	tool := NewWriteFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path": "/tmp/test.txt",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when content is missing")
	}

	// 应提示必填参数缺失
	if !strings.Contains(result.ForLLM, "content is required") &&
		!strings.Contains(result.ForUser, "content is required") {
		t.Errorf("Expected 'content is required' message, got ForLLM: %s", result.ForLLM)
	}
}

// TestFilesystemTool_ListDir_Success 验证目录列举成功的情况
func TestFilesystemTool_ListDir_Success(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content"), 0o644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755)

	tool := NewListDirTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path": tmpDir,
	}

	result := tool.Execute(ctx, args)

	// 成功时不应返回错误
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// 应列举文件和目录
	if !strings.Contains(result.ForLLM, "file1.txt") || !strings.Contains(result.ForLLM, "file2.txt") {
		t.Errorf("Expected files in listing, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "subdir") {
		t.Errorf("Expected subdir in listing, got: %s", result.ForLLM)
	}
}

// TestFilesystemTool_ListDir_NotFound 验证目录不存在时的错误处理
func TestFilesystemTool_ListDir_NotFound(t *testing.T) {
	tool := NewListDirTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path": "/nonexistent_directory_12345",
	}

	result := tool.Execute(ctx, args)

	// 失败应标记为错误
	if !result.IsError {
		t.Errorf("Expected error for non-existent directory, got IsError=false")
	}

	// 应包含错误信息
	if !strings.Contains(result.ForLLM, "failed to read") && !strings.Contains(result.ForUser, "failed to read") {
		t.Errorf("Expected error message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestFilesystemTool_ListDir_DefaultPath 验证默认路径为当前目录
func TestFilesystemTool_ListDir_DefaultPath(t *testing.T) {
	tool := NewListDirTool("", false)
	ctx := context.Background()
	args := map[string]any{}

	result := tool.Execute(ctx, args)

	// 应使用 "." 作为默认路径
	if result.IsError {
		t.Errorf("Expected success with default path '.', got IsError=true: %s", result.ForLLM)
	}
}

// 阻断看似在工作区内但通过符号链接指向外部的路径。
func TestFilesystemTool_ReadFile_RejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	link := filepath.Join(workspace, "leak.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	tool := NewReadFileTool(workspace, true, MaxReadFileSize)
	result := tool.Execute(context.Background(), map[string]any{
		"path": link,
	})

	if !result.IsError {
		t.Fatalf("expected symlink escape to be blocked")
	}
	// os.Root 根据平台/实现可能返回不同错误，
	// 但一定会报错。
	// 我们的封装返回 "access denied or file not found"
	if !strings.Contains(result.ForLLM, "access denied") && !strings.Contains(result.ForLLM, "file not found") &&
		!strings.Contains(result.ForLLM, "no such file") {
		t.Fatalf("expected symlink escape error, got: %s", result.ForLLM)
	}
}

func TestFilesystemTool_EmptyWorkspace_AccessDenied(t *testing.T) {
	tool := NewReadFileTool("", true, MaxReadFileSize) // restrict=true 但 workspace=""

	// 尝试读取一个敏感文件（用工作区外的临时文件模拟）
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, "shadow")
	os.WriteFile(secretFile, []byte("secret data"), 0o600)

	result := tool.Execute(context.Background(), map[string]any{
		"path": secretFile,
	})

	// 预期 IsError=true（因工作区为空而被阻止访问）
	assert.True(t, result.IsError, "Security Regression: Empty workspace allowed access! content: %s", result.ForLLM)

	// 验证失败原因正确
	assert.Contains(t, result.ForLLM, "workspace is not defined", "Expected 'workspace is not defined' error")
}

// TestRootMkdirAll 验证 root.MkdirAll（由 atomicWriteFileInRoot 使用）能处理所有情况：
// 单级目录、深层嵌套目录、已存在目录，以及文件阻塞目录路径的情况。
func TestRootMkdirAll(t *testing.T) {
	workspace := t.TempDir()
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatalf("failed to open root: %v", err)
	}
	defer root.Close()

	// 情况 1：单级目录
	err = root.MkdirAll("dir1", 0o755)
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(workspace, "dir1"))
	assert.NoError(t, err)

	// 情况 2：深层嵌套目录
	err = root.MkdirAll("a/b/c/d", 0o755)
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(workspace, "a/b/c/d"))
	assert.NoError(t, err)

	// 情况 3：已存在——必须具有幂等性
	err = root.MkdirAll("a/b/c/d", 0o755)
	assert.NoError(t, err)

	// 情况 4：普通文件阻塞目录创建——必须报错
	err = os.WriteFile(filepath.Join(workspace, "file_exists"), []byte("data"), 0o644)
	assert.NoError(t, err)
	err = root.MkdirAll("file_exists", 0o755)
	assert.Error(t, err, "expected error when a file exists at the directory path")
}

func TestFilesystemTool_WriteFile_Restricted_CreateDir(t *testing.T) {
	workspace := t.TempDir()
	tool := NewWriteFileTool(workspace, true)
	ctx := context.Background()

	testFile := "deep/nested/path/to/file.txt"
	content := "deep content"
	args := map[string]any{
		"path":    testFile,
		"content": content,
	}

	result := tool.Execute(ctx, args)
	assert.False(t, result.IsError, "Expected success, got: %s", result.ForLLM)

	// 验证文件内容
	actualPath := filepath.Join(workspace, testFile)
	data, err := os.ReadFile(actualPath)
	assert.NoError(t, err)
	assert.Equal(t, content, string(data))
}

// TestHostRW_Read_PermissionDenied 验证 hostRW.Read 能正确暴露访问拒绝错误。
func TestHostRW_Read_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}
	tmpDir := t.TempDir()
	protected := filepath.Join(tmpDir, "protected.txt")
	err := os.WriteFile(protected, []byte("secret"), 0o000)
	assert.NoError(t, err)
	defer os.Chmod(protected, 0o644) // 确保清理

	_, err = (&hostFs{}).ReadFile(protected)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

// TestHostRW_Read_Directory 验证 hostRW.Read 在给定目录路径时返回错误。
func TestHostRW_Read_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := (&hostFs{}).ReadFile(tmpDir)
	assert.Error(t, err, "expected error when reading a directory as a file")
}

// TestRootRW_Read_Directory 验证 rootRW.Read 在给定目录时返回错误。
func TestRootRW_Read_Directory(t *testing.T) {
	workspace := t.TempDir()
	root, err := os.OpenRoot(workspace)
	assert.NoError(t, err)
	defer root.Close()

	// 创建子目录
	err = root.Mkdir("subdir", 0o755)
	assert.NoError(t, err)

	_, err = (&sandboxFs{workspace: workspace}).ReadFile("subdir")
	assert.Error(t, err, "expected error when reading a directory as a file")
}

// TestHostRW_Write_ParentDirMissing 验证 hostRW.Write 能自动创建父目录。
func TestHostRW_Write_ParentDirMissing(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "a", "b", "c", "file.txt")

	err := (&hostFs{}).WriteFile(target, []byte("hello"))
	assert.NoError(t, err)

	data, err := os.ReadFile(target)
	assert.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

// TestRootRW_Write_ParentDirMissing 验证 rootRW.Write 能在沙箱内
// 自动创建嵌套父目录。
func TestRootRW_Write_ParentDirMissing(t *testing.T) {
	workspace := t.TempDir()

	relPath := "x/y/z/file.txt"
	err := (&sandboxFs{workspace: workspace}).WriteFile(relPath, []byte("nested"))
	assert.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(workspace, relPath))
	assert.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

// TestHostRW_Write 验证 hostRW.Write 辅助函数
func TestHostRW_Write(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "atomic_test.txt")
	testData := []byte("atomic test content")

	err := (&hostFs{}).WriteFile(testFile, testData)
	assert.NoError(t, err)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, testData, content)

	// 验证覆盖写入正确
	newData := []byte("new atomic content")
	err = (&hostFs{}).WriteFile(testFile, newData)
	assert.NoError(t, err)

	content, err = os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, newData, content)
}

// TestRootRW_Write 验证 rootRW.Write 辅助函数
func TestRootRW_Write(t *testing.T) {
	tmpDir := t.TempDir()

	relPath := "atomic_root_test.txt"
	testData := []byte("atomic root test content")

	erw := &sandboxFs{workspace: tmpDir}
	err := erw.WriteFile(relPath, testData)
	assert.NoError(t, err)

	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	f, err := root.Open(relPath)
	assert.NoError(t, err)
	defer f.Close()

	content, err := io.ReadAll(f)
	assert.NoError(t, err)
	assert.Equal(t, testData, content)

	// 验证覆盖写入正确
	newData := []byte("new root atomic content")
	err = erw.WriteFile(relPath, newData)
	assert.NoError(t, err)

	f2, err := root.Open(relPath)
	assert.NoError(t, err)
	defer f2.Close()

	content, err = io.ReadAll(f2)
	assert.NoError(t, err)
	assert.Equal(t, newData, content)
}

// TestWhitelistFs_AllowsMatchingPaths 验证 whitelistFs 允许访问匹配白名单模式的路径，
// 同时阻止不匹配的路径。
func TestWhitelistFs_AllowsMatchingPaths(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "allowed.txt")
	os.WriteFile(outsideFile, []byte("outside content"), 0o644)

	// 该模式允许访问 outsideDir。
	patterns := []*regexp.Regexp{regexp.MustCompile(`^` + regexp.QuoteMeta(outsideDir))}

	tool := NewReadFileTool(workspace, true, MaxReadFileSize, patterns)

	// 读取白名单内的路径应成功。
	result := tool.Execute(context.Background(), map[string]any{"path": outsideFile})
	if result.IsError {
		t.Errorf("expected whitelisted path to be readable, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "outside content") {
		t.Errorf("expected file content, got: %s", result.ForLLM)
	}

	// 读取白名单外的工作区外路径应失败。
	otherDir := t.TempDir()
	otherFile := filepath.Join(otherDir, "blocked.txt")
	os.WriteFile(otherFile, []byte("blocked"), 0o644)

	result = tool.Execute(context.Background(), map[string]any{"path": otherFile})
	if !result.IsError {
		t.Errorf("expected non-whitelisted path to be blocked, got: %s", result.ForLLM)
	}
}

// TestReadFileTool_ChunkedReading 通过使用 'offset' 和 'length' 分块读取文件，
// 验证工具的分页逻辑。
func TestReadFileTool_ChunkedReading(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "pagination_test.txt")

	// 创建一个内容恰好为 26 字节的测试文件
	fullContent := "abcdefghijklmnopqrstuvwxyz"
	err := os.WriteFile(testFile, []byte(fullContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	tool := NewReadFileTool(tmpDir, false, MaxReadFileSize)
	ctx := context.Background()

	// --- 第 1 步：读取第一个分块（10 字节）---
	args1 := map[string]any{
		"path":   testFile,
		"offset": 0,
		"length": 10,
	}
	result1 := tool.Execute(ctx, args1)

	if result1.IsError {
		t.Fatalf("Chunk 1 failed: %s", result1.ForLLM)
	}

	// 期望前 10 个字符
	if !strings.Contains(result1.ForLLM, "abcdefghij") {
		t.Errorf("Chunk 1 should contain 'abcdefghij', got: %s", result1.ForLLM)
	}
	// 期望头部指示文件被截断
	if !strings.Contains(result1.ForLLM, "[TRUNCATED") {
		t.Errorf("Chunk 1 header should indicate truncation, got: %s", result1.ForLLM)
	}
	// 期望头部建议下一个偏移量（10）
	if !strings.Contains(result1.ForLLM, "offset=10") {
		t.Errorf("Chunk 1 header should suggest next offset=10, got: %s", result1.ForLLM)
	}

	// 第 2 步：读取第二个分块（10 字节）---
	args2 := map[string]any{
		"path":   testFile,
		"offset": 10,
		"length": 10,
	}
	result2 := tool.Execute(ctx, args2)

	if result2.IsError {
		t.Fatalf("Chunk 2 failed: %s", result2.ForLLM)
	}

	// 期望接下来的 10 个字符
	if !strings.Contains(result2.ForLLM, "klmnopqrst") {
		t.Errorf("Chunk 2 should contain 'klmnopqrst', got: %s", result2.ForLLM)
	}
	// 期望头部建议下一个偏移量（20）
	if !strings.Contains(result2.ForLLM, "offset=20") {
		t.Errorf("Chunk 2 header should suggest next offset=20, got: %s", result2.ForLLM)
	}

	// 第 3 步：读取最后一个分块（剩余 6 字节）---
	// 请求 10 字节，但文件中只剩 6 字节
	args3 := map[string]any{
		"path":   testFile,
		"offset": 20,
		"length": 10,
	}
	result3 := tool.Execute(ctx, args3)

	if result3.IsError {
		t.Fatalf("Chunk 3 failed: %s", result3.ForLLM)
	}

	// 期望最后 6 个字符
	if !strings.Contains(result3.ForLLM, "uvwxyz") {
		t.Errorf("Chunk 3 should contain 'uvwxyz', got: %s", result3.ForLLM)
	}
	// 期望头部指示文件结尾
	if !strings.Contains(result3.ForLLM, "[END OF FILE") {
		t.Errorf("Chunk 3 header should indicate end of file, got: %s", result3.ForLLM)
	}

	// 确保最后一个分块中不包含 TRUNCATED 消息
	if strings.Contains(result3.ForLLM, "[TRUNCATED") {
		t.Errorf("Chunk 3 header should NOT indicate truncation, got: %s", result3.ForLLM)
	}
}

// TestReadFileTool_OffsetBeyondEOF 检查请求的偏移量超过文件总大小时的行为。
func TestReadFileTool_OffsetBeyondEOF(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "short.txt")

	// 创建一个仅有 5 字节的文件
	err := os.WriteFile(testFile, []byte("12345"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	tool := NewReadFileTool(tmpDir, false, MaxReadFileSize)
	ctx := context.Background()

	args := map[string]any{
		"path":   testFile,
		"offset": int64(100), // 超出文件末尾的偏移量
	}

	result := tool.Execute(ctx, args)

	// 不应归类为工具执行错误
	if result.IsError {
		t.Errorf("A mistake was not expected, obtained IsError=true: %s", result.ForLLM)
	}

	// 必须返回代码中指定的精确字符串
	expectedMsg := "[END OF FILE - no content at this offset]"
	if result.ForLLM != expectedMsg {
		t.Errorf("The message %q was expected, obtained: %q", expectedMsg, result.ForLLM)
	}
}
