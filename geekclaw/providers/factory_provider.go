// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/providers/external"
)

// ExtractProtocol 从模型字符串中提取协议前缀和模型标识符。
// 如果没有指定前缀，默认为 "openai"。
// 示例：
//   - "openai/gpt-4o" -> ("openai", "gpt-4o")
//   - "anthropic/claude-sonnet-4.6" -> ("anthropic", "claude-sonnet-4.6")
//   - "gpt-4o" -> ("openai", "gpt-4o")  // 默认协议
func ExtractProtocol(model string) (protocol, modelID string) {
	model = strings.TrimSpace(model)
	protocol, modelID, found := strings.Cut(model, "/")
	if !found {
		return "openai", model
	}
	return protocol, modelID
}

// CreateProviderFromConfig 根据 ModelConfig 创建提供者。
// 所有提供者均通过外部 Python 插件实现，Go 核心只负责路由和配置传递。
// 支持的协议：openai、litellm、anthropic、claude-cli、codex-cli、external 等
// 返回提供者、模型 ID（不含协议前缀）以及可能的错误。
func CreateProviderFromConfig(cfg *config.ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}

	if cfg.Model == "" {
		return nil, "", fmt.Errorf("model is required")
	}

	protocol, modelID := ExtractProtocol(cfg.Model)

	// 用户显式指定了 plugin_command 的外部插件
	if protocol == "external" {
		if cfg.PluginCommand == "" {
			return nil, "", fmt.Errorf("plugin_command is required for external protocol (model: %s)", cfg.Model)
		}
		return startExternalPlugin(modelID, cfg.PluginCommand, cfg.PluginArgs, cfg.PluginEnv, cfg.PluginConfig)
	}

	// 自动路由：根据协议映射到内置 Python 插件
	pluginModule, err := resolvePluginModule(protocol, cfg)
	if err != nil {
		return nil, "", err
	}

	pluginConfig, err := buildPluginConfig(cfg, protocol)
	if err != nil {
		return nil, "", err
	}

	env := cfg.PluginEnv
	if env == nil {
		env = map[string]string{}
	}
	// 确保 Python 插件可以找到 SDK 和插件模块
	if _, ok := env["PYTHONPATH"]; !ok {
		if cfg.PluginsDir != "" {
			env["PYTHONPATH"] = cfg.PluginsDir
		}
	}

	return startExternalPlugin(
		modelID,
		"python3",
		[]string{"-m", pluginModule},
		env,
		pluginConfig,
	)
}

// resolvePluginModule 根据协议前缀返回对应的 Python 插件模块路径。
func resolvePluginModule(protocol string, cfg *config.ModelConfig) (string, error) {
	switch protocol {
	case "openai":
		return "providers.contrib.openai_compat", nil

	case "litellm", "openrouter", "groq", "zhipu", "gemini", "nvidia",
		"ollama", "moonshot", "shengsuanyun", "deepseek", "cerebras",
		"vivgrid", "volcengine", "vllm", "qwen", "mistral", "avian",
		"minimax":
		return "providers.contrib.openai_compat", nil

	case "anthropic":
		return "providers.contrib.openai_compat", nil

	case "claude-cli", "claudecli", "claude-code", "claudecode":
		return "providers.contrib.claude_cli", nil

	case "codex-cli", "codexcli":
		return "providers.contrib.codex_cli", nil

	default:
		return "", fmt.Errorf("unknown protocol %q", protocol)
	}
}

// buildPluginConfig 将 ModelConfig 字段转换为插件配置 map。
func buildPluginConfig(cfg *config.ModelConfig, protocol string) (map[string]any, error) {
	pc := make(map[string]any)

	// 基本 HTTP 提供者字段
	if cfg.APIKey != "" {
		pc["api_key"] = cfg.APIKey
	}
	apiBase := cfg.APIBase
	if apiBase == "" {
		apiBase = getDefaultAPIBase(protocol)
	}
	if apiBase != "" {
		pc["api_base"] = apiBase
	}
	if cfg.Proxy != "" {
		pc["proxy"] = cfg.Proxy
	}
	if cfg.MaxTokensField != "" {
		pc["max_tokens_field"] = cfg.MaxTokensField
	}
	if cfg.RequestTimeout > 0 {
		pc["request_timeout"] = cfg.RequestTimeout
	}

	// CLI 提供者字段
	if cfg.PluginsDir != "" {
		pc["workspace"] = cfg.PluginsDir
	}

	// 合并用户指定的 plugin_config（优先级最高）
	for k, v := range cfg.PluginConfig {
		pc[k] = v
	}

	return pc, nil
}

// startExternalPlugin 启动一个外部 LLM 插件进程。
func startExternalPlugin(
	modelID, command string, args []string, env map[string]string, pluginConfig map[string]any,
) (LLMProvider, string, error) {
	plugin := external.NewExternalLLMProvider(modelID, external.PluginConfig{
		Enabled: true,
		Command: command,
		Args:    args,
		Env:     env,
		Config:  pluginConfig,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := plugin.Start(ctx); err != nil {
		return nil, "", fmt.Errorf("start provider plugin %q: %w", modelID, err)
	}

	logger.InfoCF("providers", "Provider loaded via plugin", map[string]any{
		"model":   modelID,
		"command": command,
		"args":    args,
	})

	return plugin, modelID, nil
}

// getDefaultAPIBase 返回给定协议的默认 API 基础 URL。
func getDefaultAPIBase(protocol string) string {
	switch protocol {
	case "openai":
		return "https://api.openai.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "litellm":
		return "http://localhost:4000/v1"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "zhipu":
		return "https://open.bigmodel.cn/api/paas/v4"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta"
	case "nvidia":
		return "https://integrate.api.nvidia.com/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	case "moonshot":
		return "https://api.moonshot.cn/v1"
	case "shengsuanyun":
		return "https://router.shengsuanyun.com/api/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "cerebras":
		return "https://api.cerebras.ai/v1"
	case "vivgrid":
		return "https://api.vivgrid.com/v1"
	case "volcengine":
		return "https://ark.cn-beijing.volces.com/api/v3"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "vllm":
		return "http://localhost:8000/v1"
	case "mistral":
		return "https://api.mistral.ai/v1"
	case "avian":
		return "https://api.avian.io/v1"
	case "minimax":
		return "https://api.minimaxi.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	default:
		return ""
	}
}
