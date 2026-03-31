package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/agent"
	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// NewAgentCommand 创建 agent 子命令，支持交互式和单消息模式与 AI 代理对话。
func NewAgentCommand() *cobra.Command {
	var (
		message    string
		sessionKey string
		model      string
		debug      bool
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Interact with the agent directly",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return agentCmd(message, sessionKey, model, debug)
		},
	}

	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Send a single message (non-interactive mode)")
	cmd.Flags().StringVarP(&sessionKey, "session", "s", "cli:default", "Session key")
	cmd.Flags().StringVarP(&model, "model", "", "", "Model to use")

	return cmd
}

// agentCmd 执行 agent 命令的核心逻辑，初始化配置、提供者和代理循环。
func agentCmd(message, sessionKey, model string, debug bool) error {
	if sessionKey == "" {
		sessionKey = "cli:default"
	}

	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("🔍 Debug mode enabled")
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	if model != "" {
		cfg.Agents.Defaults.ModelName = model
	}

	provider, modelID, err := providers.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	// 使用提供者创建时解析得到的模型 ID
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)
	defer agentLoop.Close()

	// 打印代理启动信息（仅交互模式下显示）
	startupInfo := agentLoop.GetStartupInfo()
	logger.InfoCF("agent", "Agent initialized",
		map[string]any{
			"tools_count":      startupInfo["tools"].(map[string]any)["count"],
			"skills_total":     startupInfo["skills"].(map[string]any)["total"],
			"skills_available": startupInfo["skills"].(map[string]any)["available"],
		})

	if message != "" {
		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, message, sessionKey)
		if err != nil {
			return fmt.Errorf("error processing message: %w", err)
		}
		fmt.Printf("\n%s %s\n", internal.Logo, response)
		return nil
	}

	fmt.Printf("%s Interactive mode (Ctrl+C to exit)\n", internal.Logo)
	fmt.Println("  /help    - show detailed help")
	fmt.Println("  /usage   - show model info and token usage")
	fmt.Println("  /cmd     - switch to command mode")
	fmt.Println("  /pico    - switch to chat mode")
	fmt.Println("  /hipico  - AI assistance in command mode")
	fmt.Println("  /byepico - end AI assistance")
	fmt.Println()
	interactiveMode(agentLoop, sessionKey)

	return nil
}
