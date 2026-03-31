package tools

import (
	"context"
	"fmt"
	"strings"
)

// SpawnTool 生成子代理在后台处理任务。
type SpawnTool struct {
	manager        *SubagentManager
	allowlistCheck func(targetAgentID string) bool
}

// 编译时检查：SpawnTool 实现了 AsyncExecutor。
var _ AsyncExecutor = (*SpawnTool)(nil)

// NewSpawnTool 创建一个新的 SpawnTool。
func NewSpawnTool(manager *SubagentManager) *SpawnTool {
	return &SpawnTool{
		manager: manager,
	}
}

// Name 返回工具名称。
func (t *SpawnTool) Name() string {
	return "spawn"
}

// Description 返回工具描述。
func (t *SpawnTool) Description() string {
	return "Spawn a subagent to handle a task in the background. Use this for complex or time-consuming tasks that can run independently. The subagent will complete the task and report back when done."
}

// Parameters 返回工具参数的 schema。
func (t *SpawnTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "The task for subagent to complete",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Optional short label for the task (for display)",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Optional target agent ID to delegate the task to",
			},
		},
		"required": []string{"task"},
	}
}

// SetAllowlistChecker 设置允许列表检查函数。
func (t *SpawnTool) SetAllowlistChecker(check func(targetAgentID string) bool) {
	t.allowlistCheck = check
}

// Execute 同步执行工具。
func (t *SpawnTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	return t.execute(ctx, args, nil)
}

// ExecuteAsync 实现 AsyncExecutor。回调作为调用参数传递给
// 子代理管理器——不会存储在 SpawnTool 实例上。
func (t *SpawnTool) ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
	return t.execute(ctx, args, cb)
}

// execute 是执行子代理生成的内部实现。
func (t *SpawnTool) execute(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
	task, ok := args["task"].(string)
	if !ok || strings.TrimSpace(task) == "" {
		return ErrorResult("task is required and must be a non-empty string")
	}

	label, _ := args["label"].(string)
	agentID, _ := args["agent_id"].(string)

	// 如果指定了目标代理则检查允许列表
	if agentID != "" && t.allowlistCheck != nil {
		if !t.allowlistCheck(agentID) {
			return ErrorResult(fmt.Sprintf("not allowed to spawn agent '%s'", agentID))
		}
	}

	if t.manager == nil {
		return ErrorResult("Subagent manager not configured")
	}

	// 从上下文读取 channel/chatID（由 registry 注入）。
	// 对于非对话调用者（如 CLI、测试）回退到 "cli"/"direct"，
	// 以保持与原始 NewSpawnTool 构造函数相同的默认值。
	channel := ToolChannel(ctx)
	if channel == "" {
		channel = "cli"
	}
	chatID := ToolChatID(ctx)
	if chatID == "" {
		chatID = "direct"
	}

	// 将回调传递给管理器以进行异步完成通知
	result, err := t.manager.Spawn(ctx, task, label, agentID, channel, chatID, cb)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to spawn subagent: %v", err))
	}

	// 返回 AsyncResult，因为任务在后台运行
	return AsyncResult(result)
}
