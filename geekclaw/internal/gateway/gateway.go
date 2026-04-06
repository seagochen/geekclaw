package gateway

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/agent"
	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	_ "github.com/seagosoft/geekclaw/geekclaw/channels/external"
	"github.com/seagosoft/geekclaw/geekclaw/health"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/media"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
	"github.com/seagosoft/geekclaw/geekclaw/voice"
	voiceext "github.com/seagosoft/geekclaw/geekclaw/voice/external"
)

// gatewayCmd 启动网关服务，初始化所有子系统并运行主循环。
func gatewayCmd(debug bool) error {
	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("🔍 Debug mode enabled")
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// 确保日志目录存在，用于运行时状态文件
	if err := os.MkdirAll(cfg.LogsPath(), 0o755); err != nil {
		return fmt.Errorf("error creating logs directory: %w", err)
	}

	provider, modelID, err := providers.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	// 使用提供商创建时解析的模型 ID
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// 输出代理启动信息
	fmt.Println("\n📦 Agent Status:")
	startupInfo := agentLoop.GetStartupInfo()
	toolsInfo := startupInfo["tools"].(map[string]any)
	skillsInfo := startupInfo["skills"].(map[string]any)
	fmt.Printf("  • Tools: %d loaded\n", toolsInfo["count"])
	fmt.Printf("  • Skills: %d/%d available\n",
		skillsInfo["available"],
		skillsInfo["total"])

	// 同时记录到文件
	logger.InfoCF("agent", "Agent initialized",
		map[string]any{
			"tools_count":      toolsInfo["count"],
			"skills_total":     skillsInfo["total"],
			"skills_available": skillsInfo["available"],
		})

	// 设置定时任务工具和服务
	execTimeout := time.Duration(cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute
	cronService := setupCronTool(
		agentLoop,
		msgBus,
		cfg.WorkspacePath(),
		cfg.Agents.Defaults.RestrictToWorkspace,
		execTimeout,
		cfg,
	)

	// heartbeat 和 devices 现在通过外部插件提供

	// 创建媒体存储，用于带 TTL 清理的文件生命周期管理
	mediaStore := media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled:  cfg.Tools.MediaCleanup.Enabled,
		MaxAge:   time.Duration(cfg.Tools.MediaCleanup.MaxAge) * time.Minute,
		Interval: time.Duration(cfg.Tools.MediaCleanup.Interval) * time.Minute,
	})
	mediaStore.Start()

	channelManager, err := channels.NewManager(cfg, msgBus, mediaStore)
	if err != nil {
		mediaStore.Stop()
		return fmt.Errorf("error creating channel manager: %w", err)
	}

	// 将频道管理器和媒体存储注入到代理循环中
	agentLoop.SetChannelManager(channelManager)
	agentLoop.SetMediaStore(mediaStore)

	// 连接语音转录功能——外部插件优先于内置插件。
	var activeTranscriber voice.Transcriber
	var voicePlugins []*voiceext.ExternalTranscriber
	for name, pcfg := range cfg.Voice.Plugins {
		if !pcfg.Enabled {
			continue
		}
		plugin := voiceext.NewExternalTranscriber(name, voiceext.PluginConfig{
			Enabled: pcfg.Enabled,
			Command: pcfg.Command,
			Args:    pcfg.Args,
			Env:     pcfg.Env,
			Config:  pcfg.Config,
		})
		if err := plugin.Start(context.Background()); err != nil {
			logger.ErrorCF("voice", "Failed to start transcriber plugin", map[string]any{
				"plugin": name,
				"error":  err.Error(),
			})
			continue
		}
		voicePlugins = append(voicePlugins, plugin)
		activeTranscriber = plugin
		break // 使用第一个成功启动的插件
	}
	if activeTranscriber != nil {
		agentLoop.SetTranscriber(activeTranscriber)
		logger.InfoCF("voice", "Transcription enabled (agent-level)", map[string]any{"provider": activeTranscriber.Name()})
	}

	enabledChannels := channelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("✓ Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("⚠ Warning: No channels enabled")
	}

	fmt.Printf("✓ Gateway started on %s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cronService.Start(); err != nil {
		fmt.Printf("Error starting cron service: %v\n", err)
	}
	fmt.Println("✓ Cron service started")

	// 设置共享 HTTP 服务器，包含健康检查端点和 webhook 处理器
	healthServer := health.NewServer(cfg.Gateway.Host, cfg.Gateway.Port)
	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	channelManager.SetupHTTPServer(addr, healthServer)

	if err := channelManager.StartAll(ctx); err != nil {
		fmt.Printf("Error starting channels: %v\n", err)
		return err
	}

	fmt.Printf("✓ Health endpoints available at http://%s:%d/health and /ready\n", cfg.Gateway.Host, cfg.Gateway.Port)

	go agentLoop.Run(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("\nShutting down...")

	// 关闭顺序至关重要：先停止消息源，再停止处理器，最后关闭总线。
	// 错误的顺序会导致 channel 向已关闭的 bus 发送消息引发 panic。

	// 1. 取消上下文，通知所有组件开始停止
	cancel()

	// 2. 使用带超时的新上下文进行优雅关闭
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	// 3. 先停止频道（消息源），确保不再有新消息进入总线
	channelManager.StopAll(shutdownCtx)

	// 4. 停止代理循环（消息消费者）
	agentLoop.Stop()
	agentLoop.Close()

	// 5. 停止定时任务（可能产生入站消息）
	cronService.Stop()

	// 6. 最后关闭总线 — 此时所有生产者和消费者都已停止
	msgBus.Close()

	// 7. 清理其余资源
	if cp, ok := provider.(providers.StatefulProvider); ok {
		cp.Close()
	}
	for _, vp := range voicePlugins {
		vp.Stop()
	}
	mediaStore.Stop()

	fmt.Println("✓ Gateway stopped")

	return nil
}
