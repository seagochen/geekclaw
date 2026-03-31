package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// MockLLMProvider 是 LLMProvider 的测试实现
type MockLLMProvider struct {
	lastOptions map[string]any
}

func (m *MockLLMProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	m.lastOptions = options
	// 查找最后一条用户消息以生成响应
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return &providers.LLMResponse{
				Content: "Task completed: " + messages[i].Content,
			}, nil
		}
	}
	return &providers.LLMResponse{Content: "No task provided"}, nil
}

func (m *MockLLMProvider) GetDefaultModel() string {
	return "test-model"
}

func (m *MockLLMProvider) SupportsTools() bool {
	return false
}

func (m *MockLLMProvider) GetContextWindow() int {
	return 4096
}

func TestSubagentManager_SetLLMOptions_AppliesToRunToolLoop(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	manager.SetLLMOptions(2048, 0.6)
	tool := NewSubagentTool(manager)

	ctx := WithToolContext(context.Background(), "cli", "direct")
	args := map[string]any{"task": "Do something"}
	result := tool.Execute(ctx, args)

	if result == nil || result.IsError {
		t.Fatalf("Expected successful result, got: %+v", result)
	}

	if provider.lastOptions == nil {
		t.Fatal("Expected LLM options to be passed, got nil")
	}
	if provider.lastOptions["max_tokens"] != 2048 {
		t.Fatalf("max_tokens = %v, want %d", provider.lastOptions["max_tokens"], 2048)
	}
	if provider.lastOptions["temperature"] != 0.6 {
		t.Fatalf("temperature = %v, want %v", provider.lastOptions["temperature"], 0.6)
	}
}

// TestSubagentTool_Name 验证工具名称
func TestSubagentTool_Name(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	if tool.Name() != "subagent" {
		t.Errorf("Expected name 'subagent', got '%s'", tool.Name())
	}
}

// TestSubagentTool_Description 验证工具描述
func TestSubagentTool_Description(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	desc := tool.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
	if !strings.Contains(desc, "subagent") {
		t.Errorf("Description should mention 'subagent', got: %s", desc)
	}
}

// TestSubagentTool_Parameters 验证工具参数的 schema 定义
func TestSubagentTool_Parameters(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	params := tool.Parameters()
	if params == nil {
		t.Error("Parameters should not be nil")
	}

	// 检查类型
	if params["type"] != "object" {
		t.Errorf("Expected type 'object', got: %v", params["type"])
	}

	// 检查属性
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("Properties should be a map")
	}

	// 验证 task 参数
	task, ok := props["task"].(map[string]any)
	if !ok {
		t.Fatal("Task parameter should exist")
	}
	if task["type"] != "string" {
		t.Errorf("Task type should be 'string', got: %v", task["type"])
	}

	// 验证 label 参数
	label, ok := props["label"].(map[string]any)
	if !ok {
		t.Fatal("Label parameter should exist")
	}
	if label["type"] != "string" {
		t.Errorf("Label type should be 'string', got: %v", label["type"])
	}

	// 检查必填字段
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("Required should be a string array")
	}
	if len(required) != 1 || required[0] != "task" {
		t.Errorf("Required should be ['task'], got: %v", required)
	}
}

// TestSubagentTool_Execute_Success 测试成功执行的情况
func TestSubagentTool_Execute_Success(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	ctx := WithToolContext(context.Background(), "telegram", "chat-123")
	args := map[string]any{
		"task":  "Write a haiku about coding",
		"label": "haiku-task",
	}

	result := tool.Execute(ctx, args)

	// 验证 ToolResult 基本结构
	if result == nil {
		t.Fatal("Result should not be nil")
	}

	// 验证无错误
	if result.IsError {
		t.Errorf("Expected success, got error: %s", result.ForLLM)
	}

	// 验证非异步
	if result.Async {
		t.Error("SubagentTool should be synchronous, not async")
	}

	// 验证非静默
	if result.Silent {
		t.Error("SubagentTool should not be silent")
	}

	// 验证 ForUser 包含简短摘要（不为空）
	if result.ForUser == "" {
		t.Error("ForUser should contain result summary")
	}
	if !strings.Contains(result.ForUser, "Task completed") {
		t.Errorf("ForUser should contain task completion, got: %s", result.ForUser)
	}

	// 验证 ForLLM 包含完整详情
	if result.ForLLM == "" {
		t.Error("ForLLM should contain full details")
	}
	if !strings.Contains(result.ForLLM, "haiku-task") {
		t.Errorf("ForLLM should contain label 'haiku-task', got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Task completed:") {
		t.Errorf("ForLLM should contain task result, got: %s", result.ForLLM)
	}
}

// TestSubagentTool_Execute_NoLabel 测试无 label 参数时的执行
func TestSubagentTool_Execute_NoLabel(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	ctx := context.Background()
	args := map[string]any{
		"task": "Test task without label",
	}

	result := tool.Execute(ctx, args)

	if result.IsError {
		t.Errorf("Expected success without label, got error: %s", result.ForLLM)
	}

	// 缺少 label 时 ForLLM 应显示 (unnamed)
	if !strings.Contains(result.ForLLM, "(unnamed)") {
		t.Errorf("ForLLM should show '(unnamed)' for missing label, got: %s", result.ForLLM)
	}
}

// TestSubagentTool_Execute_MissingTask 测试缺少 task 参数时的错误处理
func TestSubagentTool_Execute_MissingTask(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	ctx := context.Background()
	args := map[string]any{
		"label": "test",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误
	if !result.IsError {
		t.Error("Expected error for missing task parameter")
	}

	// ForLLM 应包含错误信息
	if !strings.Contains(result.ForLLM, "task is required") {
		t.Errorf("Error message should mention 'task is required', got: %s", result.ForLLM)
	}

	// Err 字段应被设置
	if result.Err == nil {
		t.Error("Err should be set for validation failure")
	}
}

// TestSubagentTool_Execute_NilManager 测试 manager 为 nil 时的错误处理
func TestSubagentTool_Execute_NilManager(t *testing.T) {
	tool := NewSubagentTool(nil)

	ctx := context.Background()
	args := map[string]any{
		"task": "test task",
	}

	result := tool.Execute(ctx, args)

	// 应返回错误
	if !result.IsError {
		t.Error("Expected error for nil manager")
	}

	if !strings.Contains(result.ForLLM, "Subagent manager not configured") {
		t.Errorf("Error message should mention manager not configured, got: %s", result.ForLLM)
	}
}

// TestSubagentTool_Execute_ContextPassing 验证 context 被正确使用
func TestSubagentTool_Execute_ContextPassing(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	channel := "test-channel"
	chatID := "test-chat"
	ctx := WithToolContext(context.Background(), channel, chatID)
	args := map[string]any{
		"task": "Test context passing",
	}

	result := tool.Execute(ctx, args)

	// 应执行成功
	if result.IsError {
		t.Errorf("Expected success with context, got error: %s", result.ForLLM)
	}

	// context 在内部使用；无法直接测试，
	// 但执行成功表明 context 已被正确处理
}

// TestSubagentTool_ForUserTruncation 验证长内容会为用户截断
func TestSubagentTool_ForUserTruncation(t *testing.T) {
	// 创建一个返回超长内容的 mock provider
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test")
	tool := NewSubagentTool(manager)

	ctx := context.Background()

	// 创建一个会生成长响应的任务
	longTask := strings.Repeat("This is a very long task description. ", 100)
	args := map[string]any{
		"task":  longTask,
		"label": "long-test",
	}

	result := tool.Execute(ctx, args)

	// ForUser 应截断至 500 字符 + "..."
	maxUserLen := 500
	if len(result.ForUser) > maxUserLen+3 { // +3 表示 "..."
		t.Errorf("ForUser should be truncated to ~%d chars, got: %d", maxUserLen, len(result.ForUser))
	}

	// ForLLM 应包含完整内容
	if !strings.Contains(result.ForLLM, longTask[:50]) {
		t.Error("ForLLM should contain reference to original task")
	}
}
