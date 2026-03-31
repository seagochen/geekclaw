// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package config

import (
	"os"
	"path/filepath"
)

// DefaultConfig 返回 GeekClaw 的默认配置。
func DefaultConfig() *Config {
	// 确定插件目录的基础路径。
	// 优先级：$GEEKCLAW_HOME > ~/.geekclaw
	var homePath string
	if geekclawHome := GeekclawHome(); geekclawHome != "" {
		homePath = geekclawHome
	} else {
		userHome, _ := os.UserHomeDir()
		homePath = filepath.Join(userHome, ".geekclaw")
	}
	workspacePath := filepath.Join(homePath, "workspace")
	pluginsPath := filepath.Join(homePath, "plugins")

	return &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Workspace:                 workspacePath,
				RestrictToWorkspace:       true,
				PluginsDir:                pluginsPath,
				Provider:                  "",
				Model:                     "",
				MaxTokens:                 32768,
				Temperature:               nil, // nil 表示使用提供者默认值
				MaxToolIterations:         50,
				SummarizeMessageThreshold: 20,
				SummarizeTokenPercent:     75,
			},
		},
		Bindings: []AgentBinding{},
		Session: SessionConfig{
			DMScope: "per-channel-peer",
		},
		Channels: ChannelsConfig{},
		Providers: ProvidersConfig{
			OpenAI: OpenAIProviderConfig{WebSearch: true},
		},
		// ModelList 默认为空。用户应通过 config.yaml 添加自己的 AI 提供者。
		// 参见 config.example.yaml 获取示例。
		ModelList: []ModelConfig{},
		Gateway: GatewayConfig{
			Host: "127.0.0.1",
			Port: 18790,
		},
		Tools: ToolsConfig{
			MediaCleanup: MediaCleanupConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				MaxAge:   30,
				Interval: 5,
			},
			Web: WebToolsConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				Proxy:           "",
				FetchLimitBytes: 10 * 1024 * 1024, // 默认 10MB
			},
			Cron: CronToolsConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				ExecTimeoutMinutes: 5,
			},
			Exec: ExecConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
			AllowRemote: true,
				TimeoutSeconds:     60,
			},
			Skills: SkillsToolsConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				Registries: SkillsRegistriesConfig{
					ClawHub: ClawHubRegistryConfig{
						Enabled: true,
						BaseURL: "https://clawhub.ai",
					},
				},
				MaxConcurrentSearches: 2,
			},
			SendFile: ToolConfig{
				Enabled: true,
			},
			MCP: MCPConfig{
				ToolConfig: ToolConfig{
					Enabled: false,
				},
				Discovery: ToolDiscoveryConfig{
					Enabled:          false,
					TTL:              5,
					MaxSearchResults: 5,
					UseBM25:          true,
					UseRegex:         false,
				},
				Servers: map[string]MCPServerConfig{},
			},
			AppendFile: ToolConfig{
				Enabled: true,
			},
			EditFile: ToolConfig{
				Enabled: true,
			},
			FindSkills: ToolConfig{
				Enabled: true,
			},
			InstallSkill: ToolConfig{
				Enabled: true,
			},
			ListDir: ToolConfig{
				Enabled: true,
			},
			Message: ToolConfig{
				Enabled: true,
			},
			ReadFile: ReadFileToolConfig{
				Enabled:         true,
				MaxReadFileSize: 64 * 1024, // 64KB
			},
			Spawn: ToolConfig{
				Enabled: true,
			},
			Subagent: ToolConfig{
				Enabled: true,
			},
			WebFetch: ToolConfig{
				Enabled: true,
			},
			WriteFile: ToolConfig{
				Enabled: true,
			},
		},
		Voice: VoiceConfig{
			EchoTranscription: false,
		},
		BuildInfo: BuildInfo{
			Version:   Version,
			GitCommit: GitCommit,
			BuildTime: BuildTime,
			GoVersion: GoVersion,
		},
	}
}
