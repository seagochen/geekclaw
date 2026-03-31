package tools

import "context"

// Tool 是所有工具必须实现的接口。
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Execute(ctx context.Context, args map[string]any) *ToolResult
}

// --- 请求级别的工具上下文 (channel / chatID) ---
//
// 通过 context.Value 传递，使并发的工具调用各自获得
// 独立的不可变副本——单例工具实例上没有可变状态。
//
// 键是未导出的指针类型变量——保证无冲突，
// 只能通过下面的辅助函数访问。

type toolCtxKey struct{ name string }

var (
	ctxKeyChannel = &toolCtxKey{"channel"}
	ctxKeyChatID  = &toolCtxKey{"chatID"}
)

// WithToolContext 返回一个携带 channel 和 chatID 的子上下文。
func WithToolContext(ctx context.Context, channel, chatID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyChannel, channel)
	ctx = context.WithValue(ctx, ctxKeyChatID, chatID)
	return ctx
}

// ToolChannel 从 ctx 中提取 channel，如果未设置则返回空字符串。
func ToolChannel(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyChannel).(string)
	return v
}

// ToolChatID 从 ctx 中提取 chatID，如果未设置则返回空字符串。
func ToolChatID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyChatID).(string)
	return v
}

// AsyncCallback 是异步工具用于通知完成的函数类型。
// 当异步工具完成工作时，它会用结果调用此回调。
//
// ctx 参数允许在代理关闭时取消回调。
// result 参数包含工具的执行结果。
type AsyncCallback func(ctx context.Context, result *ToolResult)

// AsyncExecutor 是工具可以实现的可选接口，用于支持
// 带完成回调的异步执行。
//
// 与旧的 AsyncTool 模式（SetCallback + Execute）不同，AsyncExecutor
// 将回调作为 ExecuteAsync 的参数接收。这消除了
// 并发调用可能在共享工具实例上互相覆盖回调的数据竞争。
//
// 适用于：
//   - 不应阻塞代理循环的长时间运行操作
//   - 独立完成的子代理生成
//   - 需要稍后报告结果的后台任务
//
// 示例：
//
//	func (t *SpawnTool) ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
//	    go func() {
//	        result := t.runSubagent(ctx, args)
//	        if cb != nil { cb(ctx, result) }
//	    }()
//	    return AsyncResult("Subagent spawned, will report back")
//	}
type AsyncExecutor interface {
	Tool
	// ExecuteAsync 异步运行工具。回调 cb 将在异步操作
	// 完成时被调用（可能从另一个 goroutine）。
	// 调用者（registry）保证 cb 非 nil。
	ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult
}

// ToolToSchema 将工具转换为 JSON Schema 格式的 map。
func ToolToSchema(tool Tool) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		},
	}
}
