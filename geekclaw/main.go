// GeekClaw - 超轻量级个人 AI 代理
// 灵感来源并基于 nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/internal/agent"
	"github.com/seagosoft/geekclaw/geekclaw/internal/cron"
	"github.com/seagosoft/geekclaw/geekclaw/internal/gateway"
"github.com/seagosoft/geekclaw/geekclaw/internal/skills"
	"github.com/seagosoft/geekclaw/geekclaw/internal/status"
	"github.com/seagosoft/geekclaw/geekclaw/internal/version"
	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// NewGeekclawCommand 创建并返回 GeekClaw 的根命令，注册所有子命令。
func NewGeekclawCommand() *cobra.Command {
	short := fmt.Sprintf("%s geekclaw - Personal AI Assistant v%s\n\n", internal.Logo, config.GetVersion())

	cmd := &cobra.Command{
		Use:     "geekclaw",
		Short:   short,
		Example: "geekclaw version",
	}

	cmd.AddCommand(
		agent.NewAgentCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		skills.NewSkillsCommand(),
		version.NewVersionCommand(),
	)

	return cmd
}

const (
	colorBlue = "\033[1;38;2;62;93;185m"
	colorRed  = "\033[1;38;2;213;70;70m"
	banner    = "\r\n" +
		colorBlue + " ██████╗ ███████╗███████╗██╗  ██╗" + colorRed + " ██████╗██╗      █████╗ ██╗    ██╗\n" +
		colorBlue + "██╔════╝ ██╔════╝██╔════╝██║ ██╔╝" + colorRed + "██╔════╝██║     ██╔══██╗██║    ██║\n" +
		colorBlue + "██║  ███╗█████╗  █████╗  █████╔╝ " + colorRed + "██║     ██║     ███████║██║ █╗ ██║\n" +
		colorBlue + "██║   ██║██╔══╝  ██╔══╝  ██╔═██╗ " + colorRed + "██║     ██║     ██╔══██║██║███╗██║\n" +
		colorBlue + "╚██████╔╝███████╗███████╗██║  ██╗" + colorRed + "╚██████╗███████╗██║  ██║╚███╔███╔╝\n" +
		colorBlue + " ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝" + colorRed + " ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝\n " +
		"\033[0m\r\n"
)

func main() {
	if os.Getenv("GEEKCLAW_NO_BANNER") == "" {
		fmt.Printf("%s", banner)
	}
	cmd := NewGeekclawCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
