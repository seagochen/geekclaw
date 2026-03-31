package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPManager 定义了 MCP 管理器操作的接口。
// 这允许使用 mock 实现进行更简单的测试。
type MCPManager interface {
	CallTool(
		ctx context.Context,
		serverName, toolName string,
		arguments map[string]any,
	) (*mcp.CallToolResult, error)
}

// MCPTool 包装一个 MCP 工具以实现 Tool 接口。
type MCPTool struct {
	manager    MCPManager
	serverName string
	tool       *mcp.Tool
}

// NewMCPTool 创建一个新的 MCP 工具包装器。
func NewMCPTool(manager MCPManager, serverName string, tool *mcp.Tool) *MCPTool {
	return &MCPTool{
		manager:    manager,
		serverName: serverName,
		tool:       tool,
	}
}

// sanitizeIdentifierComponent 规范化字符串，使其可以安全地用作
// 下游提供者的工具/函数标识符的一部分。
// 它会：
//   - 将字符串转为小写
//   - 将不在 [a-z0-9_-] 中的字符替换为 '_'
//   - 将多个连续的 '_' 合并为单个 '_'
//   - 去除首尾的 '_'
//   - 如果结果为空则回退到 "unnamed"
//   - 截断过长的组件到合理长度
func sanitizeIdentifierComponent(s string) string {
	const maxLen = 64

	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))

	prevUnderscore := false
	for _, r := range s {
		isAllowed := (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-'

		if !isAllowed {
			// 将任何不允许的字符规范化为 '_'
			if !prevUnderscore {
				b.WriteRune('_')
				prevUnderscore = true
			}
			continue
		}

		if r == '_' {
			if prevUnderscore {
				continue
			}
			prevUnderscore = true
		} else {
			prevUnderscore = false
		}

		b.WriteRune(r)
	}

	result := strings.Trim(b.String(), "_")
	if result == "" {
		result = "unnamed"
	}

	if len(result) > maxLen {
		result = result[:maxLen]
	}

	return result
}

// Name 返回工具名称，以服务器名称为前缀。
// 总长度上限为 64 个字符（兼容 OpenAI API 限制）。
// 当清理是有损的或名称被截断时，会追加原始（未清理的）服务器名和工具名的
// 短哈希值，确保仅在不允许字符上不同的两个名称在清理后仍保持唯一。
func (t *MCPTool) Name() string {
	// 以服务器名称为前缀避免冲突，并清理各组件
	sanitizedServer := sanitizeIdentifierComponent(t.serverName)
	sanitizedTool := sanitizeIdentifierComponent(t.tool.Name)
	full := fmt.Sprintf("mcp_%s_%s", sanitizedServer, sanitizedTool)

	// 检查清理是否无损（仅小写转换，无字符替换/截断）
	lossless := strings.ToLower(t.serverName) == sanitizedServer &&
		strings.ToLower(t.tool.Name) == sanitizedTool

	const maxTotal = 64
	if lossless && len(full) <= maxTotal {
		return full
	}

	// 清理是有损的或名称过长：追加原始名称（而非清理后名称）的哈希值，
	// 使不同的原始名称始终产生不同的哈希。
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.serverName + "\x00" + t.tool.Name))
	suffix := fmt.Sprintf("%08x", h.Sum32()) // 8 个字符

	base := full
	if len(base) > maxTotal-9 {
		base = strings.TrimRight(full[:maxTotal-9], "_")
	}
	return base + "_" + suffix
}

// Description 返回工具描述。
func (t *MCPTool) Description() string {
	desc := t.tool.Description
	if desc == "" {
		desc = fmt.Sprintf("MCP tool from %s server", t.serverName)
	}
	// 在描述中添加服务器信息
	return fmt.Sprintf("[MCP:%s] %s", t.serverName, desc)
}

// Parameters 返回工具参数的 schema。
func (t *MCPTool) Parameters() map[string]any {
	// InputSchema 已经是一个 JSON Schema 对象
	schema := t.tool.InputSchema

	// 处理 nil schema
	if schema == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		}
	}

	// 首先尝试直接转换（快速路径）
	if schemaMap, ok := schema.(map[string]any); ok {
		return schemaMap
	}

	// 处理 json.RawMessage 和 []byte——直接反序列化
	var jsonData []byte
	if rawMsg, ok := schema.(json.RawMessage); ok {
		jsonData = rawMsg
	} else if bytes, ok := schema.([]byte); ok {
		jsonData = bytes
	}

	if jsonData != nil {
		var result map[string]any
		if err := json.Unmarshal(jsonData, &result); err == nil {
			return result
		}
		// 错误时的回退
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		}
	}

	// 对于其他类型（结构体等），通过 JSON 序列化/反序列化转换
	var err error
	jsonData, err = json.Marshal(schema)
	if err != nil {
		// 序列化失败时回退到空 schema
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		}
	}

	var result map[string]any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		// 反序列化失败时回退到空 schema
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		}
	}

	return result
}

// Execute 执行 MCP 工具。
func (t *MCPTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	result, err := t.manager.CallTool(ctx, t.serverName, t.tool.Name, args)
	if err != nil {
		return ErrorResult(fmt.Sprintf("MCP tool execution failed: %v", err)).WithError(err)
	}

	if result == nil {
		nilErr := fmt.Errorf("MCP tool returned nil result without error")
		return ErrorResult("MCP tool execution failed: nil result").WithError(nilErr)
	}

	// 处理服务器返回的错误结果
	if result.IsError {
		errMsg := extractContentText(result.Content)
		return ErrorResult(fmt.Sprintf("MCP tool returned error: %s", errMsg)).
			WithError(fmt.Errorf("MCP tool error: %s", errMsg))
	}

	// 从结果中提取文本内容
	output := extractContentText(result.Content)

	return NewToolResult(output)
}

// extractContentText 从 MCP 内容数组中提取文本。
func extractContentText(content []mcp.Content) string {
	var parts []string
	for _, c := range content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		case *mcp.ImageContent:
			// 对于图片，仅指示返回了图片
			parts = append(parts, fmt.Sprintf("[Image: %s]", v.MIMEType))
		default:
			// 对于其他内容类型，使用字符串表示
			parts = append(parts, fmt.Sprintf("[Content: %T]", v))
		}
	}
	return strings.Join(parts, "\n")
}
