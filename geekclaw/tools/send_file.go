package tools

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"

	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/media"
)

// SendFileTool 允许 LLM 通过 MediaStore 管道向当前聊天频道的
// 用户发送本地文件（图片、文档等）。
type SendFileTool struct {
	workspace   string
	restrict    bool
	maxFileSize int
	mediaStore  media.MediaStore

	defaultChannel string
	defaultChatID  string
}

// NewSendFileTool 创建一个新的 SendFileTool。
func NewSendFileTool(workspace string, restrict bool, maxFileSize int, store media.MediaStore) *SendFileTool {
	if maxFileSize <= 0 {
		maxFileSize = config.DefaultMaxMediaSize
	}
	return &SendFileTool{
		workspace:   workspace,
		restrict:    restrict,
		maxFileSize: maxFileSize,
		mediaStore:  store,
	}
}

// Name 返回工具名称。
func (t *SendFileTool) Name() string { return "send_file" }

// Description 返回工具描述。
func (t *SendFileTool) Description() string {
	return "Send a local file (image, document, etc.) to the user on the current chat channel."
}

// Parameters 返回工具参数的 schema。
func (t *SendFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the local file. Relative paths are resolved from workspace.",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "Optional display filename. Defaults to the basename of path.",
			},
		},
		"required": []string{"path"},
	}
}

// SetContext 设置默认的 channel 和 chatID。
func (t *SendFileTool) SetContext(channel, chatID string) {
	t.defaultChannel = channel
	t.defaultChatID = chatID
}

// SetMediaStore 设置媒体存储。
func (t *SendFileTool) SetMediaStore(store media.MediaStore) {
	t.mediaStore = store
}

// Execute 执行文件发送操作。
func (t *SendFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}

	// 优先使用上下文注入的 channel/chatID（由 ExecuteWithContext 设置），回退到 SetContext 的值。
	channel := ToolChannel(ctx)
	if channel == "" {
		channel = t.defaultChannel
	}
	chatID := ToolChatID(ctx)
	if chatID == "" {
		chatID = t.defaultChatID
	}
	if channel == "" || chatID == "" {
		return ErrorResult("no target channel/chat available")
	}

	if t.mediaStore == nil {
		return ErrorResult("media store not configured")
	}

	resolved, err := ValidatePath(path, t.workspace, t.restrict)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return ErrorResult(fmt.Sprintf("file not found: %v", err))
	}
	if info.IsDir() {
		return ErrorResult("path is a directory, expected a file")
	}
	if info.Size() > int64(t.maxFileSize) {
		return ErrorResult(fmt.Sprintf(
			"file too large: %d bytes (max %d bytes)",
			info.Size(), t.maxFileSize,
		))
	}

	filename, _ := args["filename"].(string)
	if filename == "" {
		filename = filepath.Base(resolved)
	}

	mediaType := detectMediaType(resolved)
	scope := fmt.Sprintf("tool:send_file:%s:%s", channel, chatID)

	ref, err := t.mediaStore.Store(resolved, media.MediaMeta{
		Filename:    filename,
		ContentType: mediaType,
		Source:      "tool:send_file",
	}, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to register media: %v", err))
	}

	return MediaResult(fmt.Sprintf("File %q sent to user", filename), []string{ref})
}

// detectMediaType 确定文件的 MIME 类型。
// 首先使用魔术字节检测（h2non/filetype），然后回退到
// 通过 mime.TypeByExtension 的扩展名查找。
func detectMediaType(path string) string {
	kind, err := filetype.MatchFile(path)
	if err == nil && kind != filetype.Unknown {
		return kind.MIME.Value
	}

	if ext := filepath.Ext(path); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}

	return "application/octet-stream"
}
