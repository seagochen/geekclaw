package commands

import (
	"context"

	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/interactive"
)

// Runtime 为命令处理器提供运行时依赖。它由代理循环按请求构建，
// 以便按请求的状态（如会话作用域）与长期存活的回调（如 GetModelInfo）共存。
type Runtime struct {
	Config             *config.Config
	GetModelInfo       func() (name, provider string)
	ListAgentIDs       func() []string
	ListDefinitions    func() []Definition
	GetEnabledChannels func() []string
	SwitchModel        func(value string) (oldModel string, err error)
	SwitchChannel      func(value string) error

	// 会话模式控制（会话作用域回调）
	GetSessionMode func() string // 返回 "pico" 或 "cmd"
	SetModeCmd     func() string // 切换到 cmd 模式，返回提示字符串
	SetModePico    func() string // 切换到 pico 模式，返回状态字符串

	// 工作目录（会话作用域）
	GetWorkDir   func() string // 当前会话的工作目录
	GetWorkspace func() string // 代理工作空间根目录

	// 文件编辑——委托给循环的 handleEditCommand，并进行适当的路径解析
	EditFile func(content string) string

	// Token 和模型使用统计
	GetTokenUsage func() (promptTokens, completionTokens, requests int64)

	// Shell 命令执行——委托给循环的 executeCmdMode，并进行适当的路径/会话处理
	ExecCmd func(ctx context.Context, command string) (string, error)

	// 从 cmd 模式发起的一次性 AI 查询（/hipico）
	RunOneShot func(ctx context.Context, message string) (string, error)

	// 会话历史管理
	ClearHistory   func() error // 清除历史记录（旧接口，保留用于兼容）
	ClearSession   func() error // 清除所有历史记录和摘要并保存
	CompactSession func() error // 同步运行摘要压缩

	// 任务管理，用于停止 AI 处理
	StopLatestTask          func() (stopped bool, taskInfo string) // 停止最近的任务
	StopLatestTaskInSession func() (stopped bool, taskInfo string) // 停止当前会话中最近的任务
	ListActiveTasks         func() []string                        // 返回活跃任务信息字符串列表

	// 交互模式管理
	GetInteractiveMode    func() interactive.Mode                // 返回当前模式
	SetInteractiveMode    func(mode interactive.Mode) interactive.Mode // 设置模式，返回旧模式
	GetPendingConfirmation func() *struct {                                               // 返回待确认信息
		ID      string
		Message string
		Options []struct {
			ID string
		}
	}
	RespondToConfirmation func(response string) error // 响应待确认操作
	CancelConfirmation    func() error                // 取消待确认操作
}
