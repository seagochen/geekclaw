package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// SubagentTask 表示一个子代理任务。
type SubagentTask struct {
	ID            string
	Task          string
	Label         string
	AgentID       string
	OriginChannel string
	OriginChatID  string
	Status        string
	Result        string
	Created       int64
}

// SubagentManager 管理子代理任务的创建和执行。
type SubagentManager struct {
	tasks          map[string]*SubagentTask
	mu             sync.RWMutex
	provider       providers.LLMProvider
	defaultModel   string
	workspace      string
	tools          *ToolRegistry
	maxIterations  int
	maxTokens      int
	temperature    float64
	hasMaxTokens   bool
	hasTemperature bool
	nextID         int
}

// NewSubagentManager 创建一个新的子代理管理器。
func NewSubagentManager(
	provider providers.LLMProvider,
	defaultModel, workspace string,
) *SubagentManager {
	return &SubagentManager{
		tasks:         make(map[string]*SubagentTask),
		provider:      provider,
		defaultModel:  defaultModel,
		workspace:     workspace,
		tools:         NewToolRegistry(),
		maxIterations: 10,
		nextID:        1,
	}
}

// SetLLMOptions 设置子代理 LLM 调用的最大 token 数和温度。
func (sm *SubagentManager) SetLLMOptions(maxTokens int, temperature float64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxTokens = maxTokens
	sm.hasMaxTokens = true
	sm.temperature = temperature
	sm.hasTemperature = true
}

// SetTools 设置子代理执行的工具注册表。
// 如果未设置，子代理将使用提供的工具。
func (sm *SubagentManager) SetTools(tools *ToolRegistry) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools = tools
}

// RegisterTool 注册一个用于子代理执行的工具。
func (sm *SubagentManager) RegisterTool(tool Tool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools.Register(tool)
}

// Spawn 生成一个新的子代理任务。
func (sm *SubagentManager) Spawn(
	ctx context.Context,
	task, label, agentID, originChannel, originChatID string,
	callback AsyncCallback,
) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	taskID := fmt.Sprintf("subagent-%d", sm.nextID)
	sm.nextID++

	subagentTask := &SubagentTask{
		ID:            taskID,
		Task:          task,
		Label:         label,
		AgentID:       agentID,
		OriginChannel: originChannel,
		OriginChatID:  originChatID,
		Status:        "running",
		Created:       time.Now().UnixMilli(),
	}
	sm.tasks[taskID] = subagentTask

	// 在后台启动任务，支持上下文取消
	go sm.runTask(ctx, subagentTask, callback)

	if label != "" {
		return fmt.Sprintf("Spawned subagent '%s' for task: %s", label, task), nil
	}
	return fmt.Sprintf("Spawned subagent for task: %s", task), nil
}

// runTask 执行子代理任务。
func (sm *SubagentManager) runTask(ctx context.Context, task *SubagentTask, callback AsyncCallback) {
	task.Status = "running"
	task.Created = time.Now().UnixMilli()

	// 为子代理构建系统提示
	systemPrompt := `You are a subagent. Complete the given task independently and report the result.
You have access to tools - use them as needed to complete your task.
After completing the task, provide a clear summary of what was done.`

	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: task.Task,
		},
	}

	// 在开始前检查上下文是否已取消
	select {
	case <-ctx.Done():
		sm.mu.Lock()
		task.Status = "canceled"
		task.Result = "Task canceled before execution"
		sm.mu.Unlock()
		return
	default:
	}

	// 使用工具访问权限运行工具循环
	sm.mu.RLock()
	tools := sm.tools
	maxIter := sm.maxIterations
	maxTokens := sm.maxTokens
	temperature := sm.temperature
	hasMaxTokens := sm.hasMaxTokens
	hasTemperature := sm.hasTemperature
	sm.mu.RUnlock()

	var llmOptions map[string]any
	if hasMaxTokens || hasTemperature {
		llmOptions = map[string]any{}
		if hasMaxTokens {
			llmOptions["max_tokens"] = maxTokens
		}
		if hasTemperature {
			llmOptions["temperature"] = temperature
		}
	}

	loopResult, err := RunToolLoop(ctx, ToolLoopConfig{
		Provider:      sm.provider,
		Model:         sm.defaultModel,
		Tools:         tools,
		MaxIterations: maxIter,
		LLMOptions:    llmOptions,
	}, messages, task.OriginChannel, task.OriginChatID)

	sm.mu.Lock()
	var result *ToolResult
	defer func() {
		sm.mu.Unlock()
		// 如果提供了回调且结果已设置，则调用回调
		if callback != nil && result != nil {
			callback(ctx, result)
		}
	}()

	if err != nil {
		task.Status = "failed"
		task.Result = fmt.Sprintf("Error: %v", err)
		// 检查是否被取消
		if ctx.Err() != nil {
			task.Status = "canceled"
			task.Result = "Task canceled during execution"
		}
		result = ErrorResult(task.Result).WithError(err)
	} else {
		task.Status = "completed"
		task.Result = loopResult.Content
		result = &ToolResult{
			ForLLM: fmt.Sprintf(
				"Subagent '%s' completed (iterations: %d): %s",
				task.Label,
				loopResult.Iterations,
				loopResult.Content,
			),
			ForUser: loopResult.Content,
			Silent:  false,
			IsError: false,
			Async:   false,
		}
	}
}

// GetTask 根据任务 ID 获取任务。
func (sm *SubagentManager) GetTask(taskID string) (*SubagentTask, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	task, ok := sm.tasks[taskID]
	return task, ok
}

// ListTasks 列出所有子代理任务。
func (sm *SubagentManager) ListTasks() []*SubagentTask {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	tasks := make([]*SubagentTask, 0, len(sm.tasks))
	for _, task := range sm.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// SubagentTool 同步执行子代理任务并返回结果。
// 与异步运行的 SpawnTool 不同，SubagentTool 等待完成
// 并直接在 ToolResult 中返回结果。
type SubagentTool struct {
	manager *SubagentManager
}

// NewSubagentTool 创建一个新的 SubagentTool。
func NewSubagentTool(manager *SubagentManager) *SubagentTool {
	return &SubagentTool{
		manager: manager,
	}
}

// Name 返回工具名称。
func (t *SubagentTool) Name() string {
	return "subagent"
}

// Description 返回工具描述。
func (t *SubagentTool) Description() string {
	return "Execute a subagent task synchronously and return the result. Use this for delegating specific tasks to an independent agent instance. Returns execution summary to user and full details to LLM."
}

// Parameters 返回工具参数的 schema。
func (t *SubagentTool) Parameters() map[string]any {
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
		},
		"required": []string{"task"},
	}
}

// Execute 同步执行子代理任务。
func (t *SubagentTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	task, ok := args["task"].(string)
	if !ok {
		return ErrorResult("task is required").WithError(fmt.Errorf("task parameter is required"))
	}

	label, _ := args["label"].(string)

	if t.manager == nil {
		return ErrorResult("Subagent manager not configured").WithError(fmt.Errorf("manager is nil"))
	}

	// 为子代理构建消息
	messages := []providers.Message{
		{
			Role:    "system",
			Content: "You are a subagent. Complete the given task independently and provide a clear, concise result.",
		},
		{
			Role:    "user",
			Content: task,
		},
	}

	// 使用 RunToolLoop 执行（与异步 SpawnTool 相同）
	sm := t.manager
	sm.mu.RLock()
	tools := sm.tools
	maxIter := sm.maxIterations
	maxTokens := sm.maxTokens
	temperature := sm.temperature
	hasMaxTokens := sm.hasMaxTokens
	hasTemperature := sm.hasTemperature
	sm.mu.RUnlock()

	var llmOptions map[string]any
	if hasMaxTokens || hasTemperature {
		llmOptions = map[string]any{}
		if hasMaxTokens {
			llmOptions["max_tokens"] = maxTokens
		}
		if hasTemperature {
			llmOptions["temperature"] = temperature
		}
	}

	// 对于非对话调用者（如 CLI、测试）回退到 "cli"/"direct"，
	// 以保持与原始 NewSubagentTool 构造函数相同的默认值。
	channel := ToolChannel(ctx)
	if channel == "" {
		channel = "cli"
	}
	chatID := ToolChatID(ctx)
	if chatID == "" {
		chatID = "direct"
	}

	loopResult, err := RunToolLoop(ctx, ToolLoopConfig{
		Provider:      sm.provider,
		Model:         sm.defaultModel,
		Tools:         tools,
		MaxIterations: maxIter,
		LLMOptions:    llmOptions,
	}, messages, channel, chatID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Subagent execution failed: %v", err)).WithError(err)
	}

	// ForUser：给用户的简短摘要（过长时截断）
	userContent := loopResult.Content
	maxUserLen := 500
	if len(userContent) > maxUserLen {
		userContent = userContent[:maxUserLen] + "..."
	}

	// ForLLM：完整的执行详情
	labelStr := label
	if labelStr == "" {
		labelStr = "(unnamed)"
	}
	llmContent := fmt.Sprintf("Subagent task completed:\nLabel: %s\nIterations: %d\nResult: %s",
		labelStr, loopResult.Iterations, loopResult.Content)

	return &ToolResult{
		ForLLM:  llmContent,
		ForUser: userContent,
		Silent:  false,
		IsError: false,
		Async:   false,
	}
}
