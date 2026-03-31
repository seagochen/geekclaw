package tools

import (
	"context"
	"fmt"
)

// SearchProvider 定义搜索提供者的接口。
type SearchProvider interface {
	Search(ctx context.Context, query string, count int) (string, error)
}

// WebSearchTool 用于在网络上搜索当前信息。
type WebSearchTool struct {
	provider   SearchProvider
	maxResults int
}

// WebSearchToolOptions 包含创建 WebSearchTool 的选项。
type WebSearchToolOptions struct {
	Proxy string
	// ExternalProvider 是一个已启动的外部搜索插件。
	ExternalProvider   SearchProvider
	ExternalMaxResults int
}

// NewWebSearchTool 创建一个新的 WebSearchTool。
func NewWebSearchTool(opts WebSearchToolOptions) (*WebSearchTool, error) {
	if opts.ExternalProvider == nil {
		return nil, nil
	}
	maxResults := 5
	if opts.ExternalMaxResults > 0 {
		maxResults = opts.ExternalMaxResults
	}
	return &WebSearchTool{
		provider:   opts.ExternalProvider,
		maxResults: maxResults,
	}, nil
}

// Name 返回工具名称。
func (t *WebSearchTool) Name() string {
	return "web_search"
}

// Description 返回工具描述。
func (t *WebSearchTool) Description() string {
	return "Search the web for current information. Returns titles, URLs, and snippets from search results."
}

// Parameters 返回工具参数的 schema。
func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]any{
				"type":        "integer",
				"description": "Number of results (1-10)",
				"minimum":     1.0,
				"maximum":     10.0,
			},
		},
		"required": []string{"query"},
	}
}

// Execute 执行网络搜索操作。
func (t *WebSearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, ok := args["query"].(string)
	if !ok {
		return ErrorResult("query is required")
	}

	count := t.maxResults
	if c, ok := args["count"].(float64); ok {
		if int(c) > 0 && int(c) <= 10 {
			count = int(c)
		}
	}

	result, err := t.provider.Search(ctx, query, count)
	if err != nil {
		return ErrorResult(fmt.Sprintf("search failed: %v", err))
	}

	return UserResult(result)
}
