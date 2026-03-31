package tools

import (
	"context"
	"testing"
)

// TestWebTool_WebSearch_NoProvider 验证未设置外部提供者时不创建工具
func TestWebTool_WebSearch_NoProvider(t *testing.T) {
	tool, err := NewWebSearchTool(WebSearchToolOptions{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if tool != nil {
		t.Errorf("Expected nil tool when no provider is configured")
	}
}

// TestWebTool_WebSearch_MissingQuery 验证缺少 query 参数时的错误处理
func TestWebTool_WebSearch_MissingQuery(t *testing.T) {
	mockProvider := &mockSearchProvider{result: "some results"}
	tool, err := NewWebSearchTool(WebSearchToolOptions{
		ExternalProvider:   mockProvider,
		ExternalMaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	ctx := context.Background()
	args := map[string]any{}

	result := tool.Execute(ctx, args)

	if !result.IsError {
		t.Errorf("Expected error when query is missing")
	}
}

type mockSearchProvider struct {
	result string
	err    error
}

func (m *mockSearchProvider) Search(_ context.Context, _ string, _ int) (string, error) {
	return m.result, m.err
}
