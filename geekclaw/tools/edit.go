package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// EditFileTool 通过将 old_text 替换为 new_text 来编辑文件。
// old_text 必须在文件中精确存在。
type EditFileTool struct {
	fs fileSystem
}

// NewEditFileTool 创建一个新的 EditFileTool，可选择限制目录访问。
func NewEditFileTool(workspace string, restrict bool, allowPaths ...[]*regexp.Regexp) *EditFileTool {
	var patterns []*regexp.Regexp
	if len(allowPaths) > 0 {
		patterns = allowPaths[0]
	}
	return &EditFileTool{fs: buildFs(workspace, restrict, patterns)}
}

// Name 返回工具名称。
func (t *EditFileTool) Name() string {
	return "edit_file"
}

// Description 返回工具描述。
func (t *EditFileTool) Description() string {
	return "Edit a file by replacing old_text with new_text. The old_text must exist exactly in the file."
}

// Parameters 返回工具参数的 schema。
func (t *EditFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to edit",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "The exact text to find and replace",
			},
			"new_text": map[string]any{
				"type":        "string",
				"description": "The text to replace with",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

// Execute 执行文件编辑操作。
func (t *EditFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	oldText, ok := args["old_text"].(string)
	if !ok {
		return ErrorResult("old_text is required")
	}

	newText, ok := args["new_text"].(string)
	if !ok {
		return ErrorResult("new_text is required")
	}

	if err := editFile(t.fs, path, oldText, newText); err != nil {
		return ErrorResult(err.Error())
	}
	return SilentResult(fmt.Sprintf("File edited: %s", path))
}

// AppendFileTool 向文件末尾追加内容。
type AppendFileTool struct {
	fs fileSystem
}

// NewAppendFileTool 创建一个新的 AppendFileTool，可选择限制目录访问。
func NewAppendFileTool(workspace string, restrict bool, allowPaths ...[]*regexp.Regexp) *AppendFileTool {
	var patterns []*regexp.Regexp
	if len(allowPaths) > 0 {
		patterns = allowPaths[0]
	}
	return &AppendFileTool{fs: buildFs(workspace, restrict, patterns)}
}

// Name 返回工具名称。
func (t *AppendFileTool) Name() string {
	return "append_file"
}

// Description 返回工具描述。
func (t *AppendFileTool) Description() string {
	return "Append content to the end of a file"
}

// Parameters 返回工具参数的 schema。
func (t *AppendFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to append to",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to append",
			},
		},
		"required": []string{"path", "content"},
	}
}

// Execute 执行文件追加操作。
func (t *AppendFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return ErrorResult("content is required")
	}

	if err := appendFile(t.fs, path, content); err != nil {
		return ErrorResult(err.Error())
	}
	return SilentResult(fmt.Sprintf("Appended to %s", path))
}

// editFile 通过 sysFs 读取文件，执行替换，然后写回。
// 它使用 fileSystem 接口，使相同的逻辑可以同时用于受限和非受限模式。
func editFile(sysFs fileSystem, path, oldText, newText string) error {
	content, err := sysFs.ReadFile(path)
	if err != nil {
		return err
	}

	newContent, err := replaceEditContent(content, oldText, newText)
	if err != nil {
		return err
	}

	return sysFs.WriteFile(path, newContent)
}

// appendFile 通过 sysFs 读取现有内容（如果有），追加新内容，然后写回。
func appendFile(sysFs fileSystem, path, appendContent string) error {
	content, err := sysFs.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	newContent := append(content, []byte(appendContent)...)
	return sysFs.WriteFile(path, newContent)
}

// replaceEditContent 处理查找和替换单个 oldText 出现的核心逻辑。
func replaceEditContent(content []byte, oldText, newText string) ([]byte, error) {
	contentStr := string(content)

	if !strings.Contains(contentStr, oldText) {
		return nil, fmt.Errorf("old_text not found in file. Make sure it matches exactly")
	}

	count := strings.Count(contentStr, oldText)
	if count > 1 {
		return nil, fmt.Errorf("old_text appears %d times. Please provide more context to make it unique", count)
	}

	newContent := strings.Replace(contentStr, oldText, newText, 1)
	return []byte(newContent), nil
}
