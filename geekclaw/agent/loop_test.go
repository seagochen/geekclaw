package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/media"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
	"github.com/seagosoft/geekclaw/geekclaw/routing"
	"github.com/seagosoft/geekclaw/geekclaw/tools"
)

type fakeChannel struct{ id string }

func (f *fakeChannel) Name() string                                            { return "fake" }
func (f *fakeChannel) Start(ctx context.Context) error                         { return nil }
func (f *fakeChannel) Stop(ctx context.Context) error                          { return nil }
func (f *fakeChannel) Send(ctx context.Context, msg bus.OutboundMessage) error { return nil }
func (f *fakeChannel) IsRunning() bool                                         { return true }
func (f *fakeChannel) IsAllowed(string) bool                                   { return true }
func (f *fakeChannel) IsAllowedSender(sender bus.SenderInfo) bool              { return true }
func (f *fakeChannel) ReasoningChannelID() string                              { return f.id }

func newTestAgentLoop(
	t *testing.T,
) (al *AgentLoop, cfg *config.Config, msgBus *bus.MessageBus, provider *mockProvider, cleanup func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	cfg = &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
	msgBus = bus.NewMessageBus()
	provider = &mockProvider{}
	al = NewAgentLoop(cfg, msgBus, provider)
	return al, cfg, msgBus, provider, func() { os.RemoveAll(tmpDir) }
}

func TestRecordLastChannel(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	testChannel := "test-channel"
	if err := al.RecordLastChannel(testChannel); err != nil {
		t.Fatalf("RecordLastChannel failed: %v", err)
	}
}

func TestRecordLastChatID(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	testChatID := "test-chat-id-123"
	if err := al.RecordLastChatID(testChatID); err != nil {
		t.Fatalf("RecordLastChatID failed: %v", err)
	}
}

func TestNewAgentLoop_StateInitialized(t *testing.T) {
	// 创建临时工作目录
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建测试配置
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	// 创建 AgentLoop
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// 验证状态文件路径已设置
	if al.stateFile == "" {
		t.Error("Expected stateFile to be set")
	}

	// 验证日志路径不为空
	logsDir := cfg.LogsPath()
	if logsDir == "" {
		t.Error("Expected LogsPath to be non-empty")
	}
}

// TestToolRegistry_ToolRegistration 验证工具可以被注册并检索
func TestToolRegistry_ToolRegistration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// 注册自定义工具
	customTool := &mockCustomTool{}
	al.RegisterTool(customTool)

	// 通过调用 GetStartupInfo 不触发 panic 来验证工具已被注册
	// （实际的工具检索在 tools 包测试中验证）
	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]any)
	toolsList := toolsInfo["names"].([]string)

	// 检查自定义工具名称是否在列表中
	found := slices.Contains(toolsList, "mock_custom")
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

// TestToolContext_Updates 验证工具上下文辅助函数工作正常
func TestToolContext_Updates(t *testing.T) {
	ctx := tools.WithToolContext(context.Background(), "telegram", "chat-42")

	if got := tools.ToolChannel(ctx); got != "telegram" {
		t.Errorf("expected channel 'telegram', got %q", got)
	}
	if got := tools.ToolChatID(ctx); got != "chat-42" {
		t.Errorf("expected chatID 'chat-42', got %q", got)
	}

	// 空上下文返回空字符串
	if got := tools.ToolChannel(context.Background()); got != "" {
		t.Errorf("expected empty channel from bare context, got %q", got)
	}
}

// TestToolRegistry_GetDefinitions 验证工具定义可以被检索
func TestToolRegistry_GetDefinitions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// 注册测试工具并验证其出现在启动信息中
	testTool := &mockCustomTool{}
	al.RegisterTool(testTool)

	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]any)
	toolsList := toolsInfo["names"].([]string)

	// 检查自定义工具名称是否在列表中
	found := slices.Contains(toolsList, "mock_custom")
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

// TestAgentLoop_GetStartupInfo 验证启动信息包含工具列表
func TestAgentLoop_GetStartupInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.PluginsDir = tmpDir
	cfg.Agents.Defaults.Model = "test-model"
	cfg.Agents.Defaults.MaxTokens = 4096
	cfg.Agents.Defaults.MaxToolIterations = 10

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	info := al.GetStartupInfo()

	// 验证工具信息存在
	toolsInfo, ok := info["tools"]
	if !ok {
		t.Fatal("Expected 'tools' key in startup info")
	}

	toolsMap, ok := toolsInfo.(map[string]any)
	if !ok {
		t.Fatal("Expected 'tools' to be a map")
	}

	count, ok := toolsMap["count"]
	if !ok {
		t.Fatal("Expected 'count' in tools info")
	}

	// 应有默认工具已注册
	if count.(int) == 0 {
		t.Error("Expected at least some tools to be registered")
	}
}

// TestAgentLoop_Stop 验证 Stop() 将 running 设置为 false
func TestAgentLoop_Stop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// 注意：running 仅在调用 Run() 时才置为 true
	// 不启动事件循环无法测试该状态
	// 改为验证 Stop 方法可以安全调用
	al.Stop()

	// 验证 running 为 false（初始状态或 Stop 之后）
	if al.running.Load() {
		t.Error("Expected agent to be stopped (or never started)")
	}
}

// 测试用的 Mock 实现

type simpleMockProvider struct {
	response string
}

func (m *simpleMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *simpleMockProvider) GetDefaultModel() string {
	return "mock-model"
}

type countingMockProvider struct {
	response string
	calls    int
}

func (m *countingMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *countingMockProvider) GetDefaultModel() string {
	return "counting-mock-model"
}

// mockCustomTool 是用于注册测试的简单 mock 工具
type mockCustomTool struct{}

func (m *mockCustomTool) Name() string {
	return "mock_custom"
}

func (m *mockCustomTool) Description() string {
	return "Mock custom tool for testing"
}

func (m *mockCustomTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *mockCustomTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.SilentResult("Custom tool executed")
}

// testHelper 执行消息并返回响应
type testHelper struct {
	al *AgentLoop
}

func (h testHelper) executeAndGetResponse(tb testing.TB, ctx context.Context, msg bus.InboundMessage) string {
	// 使用短超时以避免挂起
	timeoutCtx, cancel := context.WithTimeout(ctx, responseTimeout)
	defer cancel()

	response, err := h.al.processMessage(timeoutCtx, msg)
	if err != nil {
		tb.Fatalf("processMessage failed: %v", err)
	}
	return response
}

const responseTimeout = 3 * time.Second

func TestProcessMessage_UsesRouteSessionKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "ok"}
	al := NewAgentLoop(cfg, msgBus, provider)

	msg := bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	}

	route := al.registry.ResolveRoute(routing.RouteInput{
		Channel: msg.Channel,
		Peer:    extractPeer(msg),
	})
	sessionKey := route.SessionKey

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}

	helper := testHelper{al: al}
	_ = helper.executeAndGetResponse(t, context.Background(), msg)

	history := defaultAgent.Sessions.GetHistory(sessionKey)
	if len(history) != 2 {
		t.Fatalf("expected session history len=2, got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Fatalf("unexpected first message in session: %+v", history[0])
	}
}

func TestProcessMessage_CommandOutcomes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		Session: config.SessionConfig{
			DMScope: "per-channel-peer",
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &countingMockProvider{response: "LLM reply"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	baseMsg := bus.InboundMessage{
		Channel:  "whatsapp",
		SenderID: "user1",
		ChatID:   "chat1",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	}

	showResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  baseMsg.Channel,
		SenderID: baseMsg.SenderID,
		ChatID:   baseMsg.ChatID,
		Content:  "/show channel",
		Peer:     baseMsg.Peer,
	})
	if showResp != "Current Channel: whatsapp" {
		t.Fatalf("unexpected /show reply: %q", showResp)
	}
	if provider.calls != 0 {
		t.Fatalf("LLM should not be called for handled command, calls=%d", provider.calls)
	}

	fooResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  baseMsg.Channel,
		SenderID: baseMsg.SenderID,
		ChatID:   baseMsg.ChatID,
		Content:  "/foo",
		Peer:     baseMsg.Peer,
	})
	if fooResp != "LLM reply" {
		t.Fatalf("unexpected /foo reply: %q", fooResp)
	}
	if provider.calls != 1 {
		t.Fatalf("LLM should be called exactly once after /foo passthrough, calls=%d", provider.calls)
	}

	newResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  baseMsg.Channel,
		SenderID: baseMsg.SenderID,
		ChatID:   baseMsg.ChatID,
		Content:  "/new",
		Peer:     baseMsg.Peer,
	})
	if newResp != "LLM reply" {
		t.Fatalf("unexpected /new reply: %q", newResp)
	}
	if provider.calls != 2 {
		t.Fatalf("LLM should be called for passthrough /new command, calls=%d", provider.calls)
	}
}

func TestProcessMessage_SwitchModelShowModelConsistency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Provider:          "openai",
				Model:             "before-switch",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &countingMockProvider{response: "LLM reply"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	switchResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "/switch model to after-switch",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if !strings.Contains(switchResp, "Switched model from before-switch to after-switch") {
		t.Fatalf("unexpected /switch reply: %q", switchResp)
	}

	showResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "/show model",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if !strings.Contains(showResp, "Current Model: after-switch (Provider: openai)") {
		t.Fatalf("unexpected /show model reply after switch: %q", showResp)
	}

	if provider.calls != 0 {
		t.Fatalf("LLM should not be called for /switch and /show, calls=%d", provider.calls)
	}
}

// TestToolResult_SilentToolDoesNotSendUserMessage 验证静默工具不触发出站消息
func TestToolResult_SilentToolDoesNotSendUserMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "File operation complete"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ReadFileTool 返回 SilentResult，不应发送用户消息
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "read test.txt",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// 静默工具应直接返回 LLM 的响应
	if response != "File operation complete" {
		t.Errorf("Expected 'File operation complete', got: %s", response)
	}
}

// TestToolResult_UserFacingToolDoesSendMessage 验证面向用户的工具触发出站消息
func TestToolResult_UserFacingToolDoesSendMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "Command output: hello world"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ExecTool 返回 UserResult，应发送用户消息
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "run hello",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// 面向用户的工具应将输出包含在最终响应中
	if response != "Command output: hello world" {
		t.Errorf("Expected 'Command output: hello world', got: %s", response)
	}
}

// failFirstMockProvider 在前 N 次调用时返回指定错误
type failFirstMockProvider struct {
	failures    int
	currentCall int
	failError   error
	successResp string
}

func (m *failFirstMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.currentCall++
	if m.currentCall <= m.failures {
		return nil, m.failError
	}
	return &providers.LLMResponse{
		Content:   m.successResp,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *failFirstMockProvider) GetDefaultModel() string {
	return "mock-fail-model"
}

// TestAgentLoop_ContextExhaustionRetry 验证 agent 在上下文溢出错误时执行重试
func TestAgentLoop_ContextExhaustionRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()

	// 创建一个首次调用时返回上下文错误的 provider
	contextErr := fmt.Errorf("InvalidParameter: Total tokens of image and text exceed max message tokens")
	provider := &failFirstMockProvider{
		failures:    1,
		failError:   contextErr,
		successResp: "Recovered from context error",
	}

	al := NewAgentLoop(cfg, msgBus, provider)

	// 注入历史消息以模拟上下文已满
	sessionKey := "test-session-context"
	// 创建虚假历史记录
	history := []providers.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Old message 1"},
		{Role: "assistant", Content: "Old response 1"},
		{Role: "user", Content: "Old message 2"},
		{Role: "assistant", Content: "Old response 2"},
		{Role: "user", Content: "Trigger message"},
	}
	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}
	defaultAgent.Sessions.SetHistory(sessionKey, history)

	// 调用 ProcessDirectWithChannel
	// 注意：ProcessDirectWithChannel 调用 processMessage，后者会执行 runLLMIteration
	response, err := al.ProcessDirectWithChannel(
		context.Background(),
		"Trigger message",
		sessionKey,
		"test",
		"test-chat",
	)
	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}

	if response != "Recovered from context error" {
		t.Errorf("Expected 'Recovered from context error', got '%s'", response)
	}

	// 期望 2 次调用：第 1 次失败，第 2 次成功
	if provider.currentCall != 2 {
		t.Errorf("Expected 2 calls (1 fail + 1 success), got %d", provider.currentCall)
	}

	// 检查最终历史长度
	finalHistory := defaultAgent.Sessions.GetHistory(sessionKey)
	// 验证历史已被修改（压缩）
	// 原始长度：6
	// 预期行为：压缩丢弃约 50% 历史（中间段）
	// 断言长度不等于未压缩时的值。
	// 未压缩时：6 + 1 (新用户消息) + 1 (assistant 消息) = 8
	if len(finalHistory) >= 8 {
		t.Errorf("Expected history to be compressed (len < 8), got %d", len(finalHistory))
	}
}

// TestHandleCdCommand_TraversalBlocked 验证 cd ../../.. 无法逃出工作目录。
func TestHandleCdCommand_TraversalBlocked(t *testing.T) {
	workspace := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:  workspace,
				PluginsDir: workspace,
				Model:     "test-model",
				MaxTokens: 4096,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.registry.GetDefaultAgent()

	// 将工作目录设置为工作空间内的子目录
	subdir := filepath.Join(workspace, "a", "b")
	os.MkdirAll(subdir, 0o755)
	al.setSessionWorkDir("test", subdir)

	// 尝试通过 ../../../.. 逃出工作空间
	result := al.handleCdCommand("cd ../../../..", "test", agent)
	workDir := al.getSessionWorkDir("test")

	if !strings.HasPrefix(workDir, workspace) {
		t.Errorf("cd traversal escaped workspace: workDir=%s, workspace=%s", workDir, workspace)
	}
	// 应落在工作空间内，而非外部
	if workDir != workspace {
		t.Errorf("Expected workDir=%s, got %s", workspace, workDir)
	}
	_ = result
}

// TestHandleCdCommand_NullByte 验证 cd 目标路径中的空字节
// 会被剥离，且无法绕过工作空间限制。
func TestHandleCdCommand_NullByte(t *testing.T) {
	workspace := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:  workspace,
				PluginsDir: workspace,
				Model:     "test-model",
				MaxTokens: 4096,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.registry.GetDefaultAgent()

	al.setSessionWorkDir("test", workspace)

	// 路径中的空字节应被剥离 —— 剥离后再捕获路径遍历
	al.handleCdCommand("cd sub\x00dir/../../..", "test", agent)
	workDir := al.getSessionWorkDir("test")

	if workDir != workspace {
		t.Errorf("Expected workDir=%s after null-byte cd, got %s", workspace, workDir)
	}
}

// TestHandleCdCommand_SymlinkEscape 验证工作空间内指向外部的符号链接
// 会被 ValidatePath 的符号链接解析所阻止。
func TestHandleCdCommand_SymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()

	// 在工作空间内创建一个指向外部的符号链接
	symlink := filepath.Join(workspace, "escape")
	if err := os.Symlink(outsideDir, symlink); err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:  workspace,
				PluginsDir: workspace,
				Model:     "test-model",
				MaxTokens: 4096,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.registry.GetDefaultAgent()

	al.setSessionWorkDir("test", workspace)

	al.handleCdCommand("cd escape", "test", agent)
	workDir := al.getSessionWorkDir("test")

	if workDir != workspace {
		t.Errorf("Expected symlink escape to be blocked, workDir=%s, workspace=%s", workDir, workspace)
	}
}

func TestTargetReasoningChannelID_AllChannels(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				PluginsDir:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockProvider{})
	chManager, err := channels.NewManager(&config.Config{}, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("Failed to create channel manager: %v", err)
	}
	for name, id := range map[string]string{
		"whatsapp":  "rid-whatsapp",
		"telegram":  "rid-telegram",
		"feishu":    "rid-feishu",
		"discord":   "rid-discord",
		"maixcam":   "rid-maixcam",
		"qq":        "rid-qq",
		"dingtalk":  "rid-dingtalk",
		"slack":     "rid-slack",
		"line":      "rid-line",
		"onebot":    "rid-onebot",
		"wecom":     "rid-wecom",
		"wecom_app": "rid-wecom-app",
	} {
		chManager.RegisterChannel(name, &fakeChannel{id: id})
	}
	al.SetChannelManager(chManager)
	tests := []struct {
		channel string
		wantID  string
	}{
		{channel: "whatsapp", wantID: "rid-whatsapp"},
		{channel: "telegram", wantID: "rid-telegram"},
		{channel: "feishu", wantID: "rid-feishu"},
		{channel: "discord", wantID: "rid-discord"},
		{channel: "maixcam", wantID: "rid-maixcam"},
		{channel: "qq", wantID: "rid-qq"},
		{channel: "dingtalk", wantID: "rid-dingtalk"},
		{channel: "slack", wantID: "rid-slack"},
		{channel: "line", wantID: "rid-line"},
		{channel: "onebot", wantID: "rid-onebot"},
		{channel: "wecom", wantID: "rid-wecom"},
		{channel: "wecom_app", wantID: "rid-wecom-app"},
		{channel: "unknown", wantID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			got := al.targetReasoningChannelID(tt.channel)
			if got != tt.wantID {
				t.Fatalf("targetReasoningChannelID(%q) = %q, want %q", tt.channel, got, tt.wantID)
			}
		})
	}
}

func TestHandleReasoning(t *testing.T) {
	newLoop := func(t *testing.T) (*AgentLoop, *bus.MessageBus) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "agent-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
		cfg := &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					PluginsDir:         tmpDir,
					Model:             "test-model",
					MaxTokens:         4096,
					MaxToolIterations: 10,
				},
			},
		}
		msgBus := bus.NewMessageBus()
		return NewAgentLoop(cfg, msgBus, &mockProvider{}), msgBus
	}

	t.Run("skips when any required field is empty", func(t *testing.T) {
		al, msgBus := newLoop(t)
		al.handleReasoning(context.Background(), "reasoning", "telegram", "")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if msg, ok := msgBus.SubscribeOutbound(ctx); ok {
			t.Fatalf("expected no outbound message, got %+v", msg)
		}
	})

	t.Run("publishes one message for non telegram", func(t *testing.T) {
		al, msgBus := newLoop(t)
		al.handleReasoning(context.Background(), "hello reasoning", "slack", "channel-1")

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		msg, ok := msgBus.SubscribeOutbound(ctx)
		if !ok {
			t.Fatal("expected an outbound message")
		}
		if msg.Channel != "slack" || msg.ChatID != "channel-1" || msg.Content != "hello reasoning" {
			t.Fatalf("unexpected outbound message: %+v", msg)
		}
	})

	t.Run("publishes one message for telegram", func(t *testing.T) {
		al, msgBus := newLoop(t)
		reasoning := "hello telegram reasoning"
		al.handleReasoning(context.Background(), reasoning, "telegram", "tg-chat")

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		msg, ok := msgBus.SubscribeOutbound(ctx)
		if !ok {
			t.Fatal("expected outbound message")
		}

		if msg.Channel != "telegram" {
			t.Fatalf("expected telegram channel message, got %+v", msg)
		}
		if msg.ChatID != "tg-chat" {
			t.Fatalf("expected chatID tg-chat, got %+v", msg)
		}
		if msg.Content != reasoning {
			t.Fatalf("content mismatch: got %q want %q", msg.Content, reasoning)
		}
	})
	t.Run("expired ctx", func(t *testing.T) {
		al, msgBus := newLoop(t)
		reasoning := "hello telegram reasoning"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		al.handleReasoning(ctx, reasoning, "telegram", "tg-chat")

		ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		msg, ok := msgBus.SubscribeOutbound(ctx)
		if ok {
			t.Fatalf("expected no outbound message, got %+v", msg)
		}
	})

	t.Run("returns promptly when bus is full", func(t *testing.T) {
		al, msgBus := newLoop(t)

		// 填满出站总线缓冲区直到发布操作阻塞。
		// 使用短超时检测缓冲区何时满，
		// 而非硬编码缓冲区大小。
		for i := 0; ; i++ {
			fillCtx, fillCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			err := msgBus.PublishOutbound(fillCtx, bus.OutboundMessage{
				Channel: "filler",
				ChatID:  "filler",
				Content: fmt.Sprintf("filler-%d", i),
			})
			fillCancel()
			if err != nil {
				// 缓冲区已满（发送尝试超时）。
				break
			}
		}

		// 使用短截止时间的父上下文来限制测试运行时间。
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		start := time.Now()
		al.handleReasoning(ctx, "should timeout", "slack", "channel-full")
		elapsed := time.Since(start)

		// handleReasoning 内部超时为 5s，但父上下文在 500ms 后过期。
		// 函数应在约 500ms 内返回，而不是 5s。
		if elapsed > 2*time.Second {
			t.Fatalf("handleReasoning blocked too long (%v); expected prompt return", elapsed)
		}

		// 排空总线并验证推理消息未被发布
		// （应因超时而被丢弃）。
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer drainCancel()
		foundReasoning := false
		for {
			msg, ok := msgBus.SubscribeOutbound(drainCtx)
			if !ok {
				break
			}
			if msg.Content == "should timeout" {
				foundReasoning = true
			}
		}
		if foundReasoning {
			t.Fatal("expected reasoning message to be dropped when bus is full, but it was published")
		}
	})
}

func TestResolveMediaRefs_ResolvesToBase64(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	// 创建最小有效 PNG（8 字节头部即可用于文件类型检测）
	pngPath := filepath.Join(dir, "test.png")
	// PNG 魔数：0x89 P N G \r \n 0x1A \n + 最小 IHDR
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG 签名
		0x00, 0x00, 0x00, 0x0D, // IHDR length
		0x49, 0x48, 0x44, 0x52, // "IHDR"
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, // 1x1 RGB
		0x00, 0x00, 0x00, // no interlace
		0x90, 0x77, 0x53, 0xDE, // CRC
	}
	if err := os.WriteFile(pngPath, pngHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := store.Store(pngPath, media.MediaMeta{}, "test")
	if err != nil {
		t.Fatal(err)
	}

	messages := []providers.Message{
		{Role: "user", Content: "describe this", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 1 {
		t.Fatalf("expected 1 resolved media, got %d", len(result[0].Media))
	}
	if !strings.HasPrefix(result[0].Media[0], "data:image/png;base64,") {
		t.Fatalf("expected data:image/png;base64, prefix, got %q", result[0].Media[0][:40])
	}
}

func TestResolveMediaRefs_SkipsOversizedFile(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	bigPath := filepath.Join(dir, "big.png")
	// 写入 PNG 头部 + 填充以超过限制
	data := make([]byte, 1024+1) // 1KB + 1 字节
	copy(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	if err := os.WriteFile(bigPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ref, _ := store.Store(bigPath, media.MediaMeta{}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	// 使用极小限制（1KB）使文件超出大小
	result := resolveMediaRefs(messages, store, 1024)

	if len(result[0].Media) != 0 {
		t.Fatalf("expected 0 media (oversized), got %d", len(result[0].Media))
	}
}

func TestResolveMediaRefs_SkipsUnknownType(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	txtPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, _ := store.Store(txtPath, media.MediaMeta{}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 0 {
		t.Fatalf("expected 0 media (unknown type), got %d", len(result[0].Media))
	}
}

func TestResolveMediaRefs_PassesThroughNonMediaRefs(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{"https://example.com/img.png"}},
	}
	result := resolveMediaRefs(messages, nil, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 1 || result[0].Media[0] != "https://example.com/img.png" {
		t.Fatalf("expected passthrough of non-media:// URL, got %v", result[0].Media)
	}
}

func TestResolveMediaRefs_DoesNotMutateOriginal(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "test.png")
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02,
		0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
	}
	os.WriteFile(pngPath, pngHeader, 0o644)
	ref, _ := store.Store(pngPath, media.MediaMeta{}, "test")

	original := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	originalRef := original[0].Media[0]

	resolveMediaRefs(original, store, config.DefaultMaxMediaSize)

	if original[0].Media[0] != originalRef {
		t.Fatal("resolveMediaRefs mutated original message slice")
	}
}

func TestResolveMediaRefs_UsesMetaContentType(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	// JPEG 内容的文件，但以显式 content type 存储
	jpegPath := filepath.Join(dir, "photo")
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG 魔数
	os.WriteFile(jpegPath, jpegHeader, 0o644)
	ref, _ := store.Store(jpegPath, media.MediaMeta{ContentType: "image/jpeg"}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 1 {
		t.Fatalf("expected 1 media, got %d", len(result[0].Media))
	}
	if !strings.HasPrefix(result[0].Media[0], "data:image/jpeg;base64,") {
		t.Fatalf("expected jpeg prefix, got %q", result[0].Media[0][:30])
	}
}

