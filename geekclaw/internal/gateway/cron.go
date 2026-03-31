package gateway

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/agent"
	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/cron"
	"github.com/seagosoft/geekclaw/geekclaw/tools"
)

// setupCronTool 初始化定时任务服务并注册 CronTool 到 Agent 循环中。
func setupCronTool(
	agentLoop *agent.AgentLoop,
	msgBus *bus.MessageBus,
	pluginsDir string,
	restrict bool,
	execTimeout time.Duration,
	cfg *config.Config,
) *cron.CronService {
	cronStorePath := filepath.Join(filepath.Dir(pluginsDir), "logs", "jobs.json")

	// 创建定时任务服务
	cronService := cron.NewCronService(cronStorePath, nil)

	// 如果启用则创建并注册 CronTool
	var cronTool *tools.CronTool
	if cfg.Tools.IsToolEnabled("cron") {
		var err error
		cronTool, err = tools.NewCronTool(cronService, agentLoop, msgBus, pluginsDir, restrict, execTimeout, cfg)
		if err != nil {
			log.Fatalf("Critical error during CronTool initialization: %v", err)
		}

		agentLoop.RegisterTool(cronTool)
	}

	// 设置任务执行回调处理器
	if cronTool != nil {
		cronService.SetOnJob(func(job *cron.CronJob) (string, error) {
			result := cronTool.ExecuteJob(context.Background(), job)
			return result, nil
		})
	}

	return cronService
}
