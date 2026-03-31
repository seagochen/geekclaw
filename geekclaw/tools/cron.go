package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/cron"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
)

// JobExecutor 是通过代理执行定时任务的接口。
type JobExecutor interface {
	ProcessDirectWithChannel(ctx context.Context, content, sessionKey, channel, chatID string) (string, error)
}

// CronTool 为代理提供定时调度能力。
type CronTool struct {
	cronService *cron.CronService
	executor    JobExecutor
	msgBus      *bus.MessageBus
	execTool    *ExecTool
}

// NewCronTool 创建一个新的 CronTool。
// execTimeout: 0 表示无超时，>0 设置超时时长。
func NewCronTool(
	cronService *cron.CronService, executor JobExecutor, msgBus *bus.MessageBus, workspace string, restrict bool,
	execTimeout time.Duration, config *config.Config,
) (*CronTool, error) {
	execTool, err := NewExecToolWithConfig(workspace, restrict, config)
	if err != nil {
		return nil, fmt.Errorf("unable to configure exec tool: %w", err)
	}

	execTool.SetTimeout(execTimeout)
	return &CronTool{
		cronService: cronService,
		executor:    executor,
		msgBus:      msgBus,
		execTool:    execTool,
	}, nil
}

// Name 返回工具名称。
func (t *CronTool) Name() string {
	return "cron"
}

// Description 返回工具描述。
func (t *CronTool) Description() string {
	return "Schedule reminders, tasks, or system commands. IMPORTANT: When user asks to be reminded or scheduled, you MUST call this tool. Use 'at_seconds' for one-time reminders (e.g., 'remind me in 10 minutes' → at_seconds=600). Use 'every_seconds' ONLY for recurring tasks (e.g., 'every 2 hours' → every_seconds=7200). Use 'cron_expr' for complex recurring schedules. Use 'command' to execute shell commands directly."
}

// Parameters 返回工具参数的 schema。
func (t *CronTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"add", "list", "remove", "enable", "disable"},
				"description": "Action to perform. Use 'add' when user wants to schedule a reminder or task.",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The reminder/task message to display when triggered. If 'command' is used, this describes what the command does.",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Optional: Shell command to execute directly (e.g., 'df -h'). If set, the agent will run this command and report output instead of just showing the message. 'deliver' will be forced to false for commands.",
			},
			"command_confirm": map[string]any{
				"type":        "boolean",
				"description": "Required when using command=true. Must be true to explicitly confirm scheduling a shell command.",
			},
			"at_seconds": map[string]any{
				"type":        "integer",
				"description": "One-time reminder: seconds from now when to trigger (e.g., 600 for 10 minutes later). Use this for one-time reminders like 'remind me in 10 minutes'.",
			},
			"every_seconds": map[string]any{
				"type":        "integer",
				"description": "Recurring interval in seconds (e.g., 3600 for every hour). Use this ONLY for recurring tasks like 'every 2 hours' or 'daily reminder'.",
			},
			"cron_expr": map[string]any{
				"type":        "string",
				"description": "Cron expression for complex recurring schedules (e.g., '0 9 * * *' for daily at 9am). Use this for complex recurring schedules.",
			},
			"job_id": map[string]any{
				"type":        "string",
				"description": "Job ID (for remove/enable/disable)",
			},
			"deliver": map[string]any{
				"type":        "boolean",
				"description": "If true, send message directly to channel. If false, let agent process message (for complex tasks). Default: true",
			},
		},
		"required": []string{"action"},
	}
}

// Execute 使用给定参数运行工具。
func (t *CronTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, ok := args["action"].(string)
	if !ok {
		return ErrorResult("action is required")
	}

	switch action {
	case "add":
		return t.addJob(ctx, args)
	case "list":
		return t.listJobs()
	case "remove":
		return t.removeJob(args)
	case "enable":
		return t.enableJob(args, true)
	case "disable":
		return t.enableJob(args, false)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

// addJob 添加一个新的定时任务。
func (t *CronTool) addJob(ctx context.Context, args map[string]any) *ToolResult {
	channel := ToolChannel(ctx)
	chatID := ToolChatID(ctx)

	if channel == "" || chatID == "" {
		return ErrorResult("no session context (channel/chat_id not set). Use this tool in an active conversation.")
	}

	message, ok := args["message"].(string)
	if !ok || message == "" {
		return ErrorResult("message is required for add")
	}

	var schedule cron.CronSchedule

	// 检查 at_seconds（一次性）、every_seconds（循环）或 cron_expr
	atSeconds, hasAt := args["at_seconds"].(float64)
	everySeconds, hasEvery := args["every_seconds"].(float64)
	cronExpr, hasCron := args["cron_expr"].(string)

	// 修复：类型断言对零值也返回 true，需要额外的有效性检查
	// 这防止了 LLM 用默认值（0）填充未使用的可选参数而触发错误类型
	hasAt = hasAt && atSeconds > 0
	hasEvery = hasEvery && everySeconds > 0
	hasCron = hasCron && cronExpr != ""

	// 优先级：at_seconds > every_seconds > cron_expr
	if hasAt {
		atMS := time.Now().UnixMilli() + int64(atSeconds)*1000
		schedule = cron.CronSchedule{
			Kind: "at",
			AtMS: &atMS,
		}
	} else if hasEvery {
		everyMS := int64(everySeconds) * 1000
		schedule = cron.CronSchedule{
			Kind:    "every",
			EveryMS: &everyMS,
		}
	} else if hasCron {
		schedule = cron.CronSchedule{
			Kind: "cron",
			Expr: cronExpr,
		}
	} else {
		return ErrorResult("one of at_seconds, every_seconds, or cron_expr is required")
	}

	// 读取 deliver 参数，默认为 true
	deliver := true
	if d, ok := args["deliver"].(bool); ok {
		deliver = d
	}

	// GHSA-pv8c-p6jf-3fpp: 命令调度需要内部频道 + 显式确认。
	// 非命令提醒（纯消息）对所有频道开放。
	command, _ := args["command"].(string)
	commandConfirm, _ := args["command_confirm"].(bool)
	if command != "" {
		if !channels.IsInternalChannel(channel) {
			return ErrorResult("scheduling command execution is restricted to internal channels")
		}
		if !commandConfirm {
			return ErrorResult("command_confirm=true is required to schedule command execution")
		}
		deliver = false
	}

	// 截断消息作为任务名称（最多 30 个字符）
	messagePreview := utils.Truncate(message, 30)

	job, err := t.cronService.AddJob(
		messagePreview,
		schedule,
		message,
		deliver,
		channel,
		chatID,
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Error adding job: %v", err))
	}

	if command != "" {
		job.Payload.Command = command
		// 需要保存更新后的 payload
		t.cronService.UpdateJob(job)
	}

	return SilentResult(fmt.Sprintf("Cron job added: %s (id: %s)", job.Name, job.ID))
}

// listJobs 列出所有已调度的任务。
func (t *CronTool) listJobs() *ToolResult {
	jobs := t.cronService.ListJobs(false)

	if len(jobs) == 0 {
		return SilentResult("No scheduled jobs")
	}

	var result strings.Builder
	result.WriteString("Scheduled jobs:\n")
	for _, j := range jobs {
		var scheduleInfo string
		if j.Schedule.Kind == "every" && j.Schedule.EveryMS != nil {
			scheduleInfo = fmt.Sprintf("every %ds", *j.Schedule.EveryMS/1000)
		} else if j.Schedule.Kind == "cron" {
			scheduleInfo = j.Schedule.Expr
		} else if j.Schedule.Kind == "at" {
			scheduleInfo = "one-time"
		} else {
			scheduleInfo = "unknown"
		}
		result.WriteString(fmt.Sprintf("- %s (id: %s, %s)\n", j.Name, j.ID, scheduleInfo))
	}

	return SilentResult(result.String())
}

// removeJob 移除一个定时任务。
func (t *CronTool) removeJob(args map[string]any) *ToolResult {
	jobID, ok := args["job_id"].(string)
	if !ok || jobID == "" {
		return ErrorResult("job_id is required for remove")
	}

	if t.cronService.RemoveJob(jobID) {
		return SilentResult(fmt.Sprintf("Cron job removed: %s", jobID))
	}
	return ErrorResult(fmt.Sprintf("Job %s not found", jobID))
}

// enableJob 启用或禁用一个定时任务。
func (t *CronTool) enableJob(args map[string]any, enable bool) *ToolResult {
	jobID, ok := args["job_id"].(string)
	if !ok || jobID == "" {
		return ErrorResult("job_id is required for enable/disable")
	}

	job := t.cronService.EnableJob(jobID, enable)
	if job == nil {
		return ErrorResult(fmt.Sprintf("Job %s not found", jobID))
	}

	status := "enabled"
	if !enable {
		status = "disabled"
	}
	return SilentResult(fmt.Sprintf("Cron job '%s' %s", job.Name, status))
}

// ExecuteJob 通过代理执行一个定时任务。
func (t *CronTool) ExecuteJob(ctx context.Context, job *cron.CronJob) string {
	// 从任务 payload 获取 channel/chatID
	channel := job.Payload.Channel
	chatID := job.Payload.To

	// 未设置时的默认值
	if channel == "" {
		channel = "cli"
	}
	if chatID == "" {
		chatID = "direct"
	}

	// 如果存在命令则执行命令
	if job.Payload.Command != "" {
		args := map[string]any{
			"command":   job.Payload.Command,
			"__channel": channel,
			"__chat_id": chatID,
		}

		result := t.execTool.Execute(ctx, args)
		var output string
		if result.IsError {
			output = fmt.Sprintf("Error executing scheduled command: %s", result.ForLLM)
		} else {
			output = fmt.Sprintf("Scheduled command '%s' executed:\n%s", job.Payload.Command, result.ForLLM)
		}

		pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pubCancel()
		t.msgBus.PublishOutbound(pubCtx, bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: output,
		})
		return "ok"
	}

	// 如果 deliver=true，直接发送消息而不经过代理处理
	if job.Payload.Deliver {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pubCancel()
		t.msgBus.PublishOutbound(pubCtx, bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: job.Payload.Message,
		})
		return "ok"
	}

	// 对于 deliver=false，通过代理处理（用于复杂任务）
	sessionKey := fmt.Sprintf("cron-%s", job.ID)

	// 用任务的消息调用代理
	response, err := t.executor.ProcessDirectWithChannel(
		ctx,
		job.Payload.Message,
		sessionKey,
		channel,
		chatID,
	)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	// 响应由 AgentLoop 通过 MessageBus 自动发送
	_ = response // 将由 AgentLoop 发送
	return "ok"
}
