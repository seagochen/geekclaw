package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEditTool_EditFile_Success 验证文件编辑成功的情况
func TestEditTool_EditFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Hello World\nThis is a test"), 0o644)

	tool := NewEditFileTool(tmpDir, true)
	ctx := context.Background()
	args := map[string]any{
		"path":     testFile,
		"old_text": "World",
		"new_text": "Universe",
	}

	result := tool.Execute(ctx, args)

	// 成功时不应返回错误
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// 应返回 SilentResult
	if !result.Silent {
		t.Errorf("Expected Silent=true for EditFile, got false")
	}

	// ForUser 应为空（静默结果）
	if result.ForUser != "" {
		t.Errorf("Expected ForUser to be empty for SilentResult, got: %s", result.ForUser)
	}

	// 验证文件已实际被编辑
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read edited file: %v", err)
	}
	contentStr := string(content)
	if !strings.Contains(contentStr, "Hello Universe") {
		t.Errorf("Expected file to contain 'Hello Universe', got: %s", contentStr)
	}
	if strings.Contains(contentStr, "Hello World") {
		t.Errorf("Expected 'Hello World' to be replaced, got: %s", contentStr)
	}
}

// TestEditTool_EditFile_NotFound 验证不存在文件的错误处理
func TestEditTool_EditFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nonexistent.txt")

	tool := NewEditFileTool(tmpDir, true)
	ctx := context.Background()
	args := map[string]any{
		"path":     testFile,
		"old_text": "old",
		"new_text": "new",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error for non-existent file")
	}

	// 应提示文件未找到
	if !strings.Contains(result.ForLLM, "not found") && !strings.Contains(result.ForUser, "not found") {
		t.Errorf("Expected 'file not found' message, got ForLLM: %s", result.ForLLM)
	}
}

// TestEditTool_EditFile_OldTextNotFound 验证 old_text 不存在时的错误处理
func TestEditTool_EditFile_OldTextNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Hello World"), 0o644)

	tool := NewEditFileTool(tmpDir, true)
	ctx := context.Background()
	args := map[string]any{
		"path":     testFile,
		"old_text": "Goodbye",
		"new_text": "Hello",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when old_text not found")
	}

	// 应提示 old_text 未找到
	if !strings.Contains(result.ForLLM, "not found") && !strings.Contains(result.ForUser, "not found") {
		t.Errorf("Expected 'not found' message, got ForLLM: %s", result.ForLLM)
	}
}

// TestEditTool_EditFile_MultipleMatches 验证 old_text 出现多次时的错误处理
func TestEditTool_EditFile_MultipleMatches(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test test test"), 0o644)

	tool := NewEditFileTool(tmpDir, true)
	ctx := context.Background()
	args := map[string]any{
		"path":     testFile,
		"old_text": "test",
		"new_text": "done",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when old_text appears multiple times")
	}

	// 应提示存在多个匹配
	if !strings.Contains(result.ForLLM, "times") && !strings.Contains(result.ForUser, "times") {
		t.Errorf("Expected 'multiple times' message, got ForLLM: %s", result.ForLLM)
	}
}

// TestEditTool_EditFile_OutsideAllowedDir 验证路径超出允许目录时的错误处理
func TestEditTool_EditFile_OutsideAllowedDir(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0o644)

	tool := NewEditFileTool(tmpDir, true) // 限制在 tmpDir 目录内
	ctx := context.Background()
	args := map[string]any{
		"path":     testFile,
		"old_text": "content",
		"new_text": "new",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	assert.True(t, result.IsError, "Expected error when path is outside allowed directory")

	// 应提示路径超出允许目录
	// 注意：ErrorResult 默认仅设置 ForLLM，ForUser 可能为空。
	// 我们检查 ForLLM，因为它是主要的错误传递通道。
	assert.True(
		t,
		strings.Contains(result.ForLLM, "outside") || strings.Contains(result.ForLLM, "access denied") ||
			strings.Contains(result.ForLLM, "escapes"),
		"Expected 'outside allowed' or 'access denied' message, got ForLLM: %s",
		result.ForLLM,
	)
}

// TestEditTool_EditFile_MissingPath 验证缺少 path 参数时的错误处理
func TestEditTool_EditFile_MissingPath(t *testing.T) {
	tool := NewEditFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"old_text": "old",
		"new_text": "new",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when path is missing")
	}
}

// TestEditTool_EditFile_MissingOldText 验证缺少 old_text 参数时的错误处理
func TestEditTool_EditFile_MissingOldText(t *testing.T) {
	tool := NewEditFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path":     "/tmp/test.txt",
		"new_text": "new",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when old_text is missing")
	}
}

// TestEditTool_EditFile_MissingNewText 验证缺少 new_text 参数时的错误处理
func TestEditTool_EditFile_MissingNewText(t *testing.T) {
	tool := NewEditFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path":     "/tmp/test.txt",
		"old_text": "old",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when new_text is missing")
	}
}

// TestEditTool_AppendFile_Success 验证文件追加成功的情况
func TestEditTool_AppendFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Initial content"), 0o644)

	tool := NewAppendFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path":    testFile,
		"content": "\nAppended content",
	}

	result := tool.Execute(ctx, args)

	// 成功时不应返回错误
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// 应返回 SilentResult
	if !result.Silent {
		t.Errorf("Expected Silent=true for AppendFile, got false")
	}

	// ForUser 应为空（静默结果）
	if result.ForUser != "" {
		t.Errorf("Expected ForUser to be empty for SilentResult, got: %s", result.ForUser)
	}

	// 验证内容已实际被追加
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	contentStr := string(content)
	if !strings.Contains(contentStr, "Initial content") {
		t.Errorf("Expected original content to remain, got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "Appended content") {
		t.Errorf("Expected appended content, got: %s", contentStr)
	}
}

// TestEditTool_AppendFile_MissingPath 验证缺少 path 参数时的错误处理
func TestEditTool_AppendFile_MissingPath(t *testing.T) {
	tool := NewAppendFileTool("", false)
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

// TestEditTool_AppendFile_MissingContent 验证缺少 content 参数时的错误处理
func TestEditTool_AppendFile_MissingContent(t *testing.T) {
	tool := NewAppendFileTool("", false)
	ctx := context.Background()
	args := map[string]any{
		"path": "/tmp/test.txt",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误结果
	if !result.IsError {
		t.Errorf("Expected error when content is missing")
	}
}

// TestReplaceEditContent 验证辅助函数 replaceEditContent
func TestReplaceEditContent(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		oldText     string
		newText     string
		expected    []byte
		expectError bool
	}{
		{
			name:        "successful replacement",
			content:     []byte("hello world"),
			oldText:     "world",
			newText:     "universe",
			expected:    []byte("hello universe"),
			expectError: false,
		},
		{
			name:        "old text not found",
			content:     []byte("hello world"),
			oldText:     "golang",
			newText:     "rust",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "multiple matches found",
			content:     []byte("test text test"),
			oldText:     "test",
			newText:     "done",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := replaceEditContent(tt.content, tt.oldText, tt.newText)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestAppendFileTool_AppendToNonExistent_Restricted 验证受限模式下 AppendFileTool
// 可以向不存在的文件追加内容——应静默创建该文件。
// 此测试覆盖 appendFileWithRW + rootRW 中 errors.Is(err, fs.ErrNotExist) 的代码路径。
func TestAppendFileTool_AppendToNonExistent_Restricted(t *testing.T) {
	workspace := t.TempDir()
	tool := NewAppendFileTool(workspace, true)
	ctx := context.Background()

	args := map[string]any{
		"path":    "brand_new_file.txt",
		"content": "first content",
	}

	result := tool.Execute(ctx, args)
	assert.False(
		t,
		result.IsError,
		"Expected success when appending to non-existent file in restricted mode, got: %s",
		result.ForLLM,
	)

	// 验证文件已以正确内容创建
	data, err := os.ReadFile(filepath.Join(workspace, "brand_new_file.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "first content", string(data))
}

// TestAppendFileTool_Restricted_Success 验证受限模式下 AppendFileTool
// 能正确向沙箱内的已有文件追加内容。
func TestAppendFileTool_Restricted_Success(t *testing.T) {
	workspace := t.TempDir()
	testFile := "existing.txt"
	err := os.WriteFile(filepath.Join(workspace, testFile), []byte("initial"), 0o644)
	assert.NoError(t, err)

	tool := NewAppendFileTool(workspace, true)
	ctx := context.Background()
	args := map[string]any{
		"path":    testFile,
		"content": " appended",
	}

	result := tool.Execute(ctx, args)
	assert.False(t, result.IsError, "Expected success, got: %s", result.ForLLM)
	assert.True(t, result.Silent)

	data, err := os.ReadFile(filepath.Join(workspace, testFile))
	assert.NoError(t, err)
	assert.Equal(t, "initial appended", string(data))
}

// TestEditFileTool_Restricted_InPlaceEdit 验证受限模式下 EditFileTool
// 能通过单次打开的 editFileInRoot 路径正确编辑文件。
func TestEditFileTool_Restricted_InPlaceEdit(t *testing.T) {
	workspace := t.TempDir()
	testFile := "edit_target.txt"
	err := os.WriteFile(filepath.Join(workspace, testFile), []byte("Hello World"), 0o644)
	assert.NoError(t, err)

	tool := NewEditFileTool(workspace, true)
	ctx := context.Background()
	args := map[string]any{
		"path":     testFile,
		"old_text": "World",
		"new_text": "Go",
	}

	result := tool.Execute(ctx, args)
	assert.False(t, result.IsError, "Expected success, got: %s", result.ForLLM)
	assert.True(t, result.Silent)

	data, err := os.ReadFile(filepath.Join(workspace, testFile))
	assert.NoError(t, err)
	assert.Equal(t, "Hello Go", string(data))
}

// TestEditFileTool_Restricted_FileNotFound 验证目标文件不存在时
// editFileInRoot 返回合适的错误信息。
func TestEditFileTool_Restricted_FileNotFound(t *testing.T) {
	workspace := t.TempDir()
	tool := NewEditFileTool(workspace, true)
	ctx := context.Background()
	args := map[string]any{
		"path":     "no_such_file.txt",
		"old_text": "old",
		"new_text": "new",
	}

	result := tool.Execute(ctx, args)
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "not found")
}
