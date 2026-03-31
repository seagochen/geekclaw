package providers

import (
	"testing"

	"github.com/seagosoft/geekclaw/geekclaw/auth"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/providers/external"
)

// TestCreateProvider_OpenRouter 验证 CreateProvider 为 OpenRouter 协议创建外部插件。
// 此测试需要 Python 环境和插件，跳过以避免 CI 失败。
func TestCreateProvider_OpenRouter(t *testing.T) {
	t.Skip("integration test: requires Python plugin environment")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Model = "test-openrouter"
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "test-openrouter",
			Model:     "openrouter/auto",
			APIKey:    "sk-or-test",
			APIBase:   "https://openrouter.ai/api/v1",
		},
	}

	provider, _, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}

	if _, ok := provider.(*external.ExternalLLMProvider); !ok {
		t.Fatalf("provider type = %T, want *external.ExternalLLMProvider", provider)
	}
}

// TestCreateProvider_ClaudeCli 验证 CreateProvider 为 claude-cli 协议创建外部插件。
func TestCreateProvider_ClaudeCli(t *testing.T) {
	t.Skip("integration test: requires Python plugin environment")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Model = "test-claude-cli"
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName:  "test-claude-cli",
			Model:      "claude-cli/claude-sonnet",
			PluginsDir: "/tmp/workspace",
		},
	}

	provider, _, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}

	if _, ok := provider.(*external.ExternalLLMProvider); !ok {
		t.Fatalf("provider type = %T, want *external.ExternalLLMProvider", provider)
	}
}

// TestCreateProvider_AnthropicOAuth 验证 CreateProvider 为 Anthropic OAuth 创建外部插件。
func TestCreateProvider_AnthropicOAuth(t *testing.T) {
	t.Skip("integration test: requires Python plugin environment")
	originalGetCredential := getCredential
	t.Cleanup(func() { getCredential = originalGetCredential })

	getCredential = func(provider string) (*auth.AuthCredential, error) {
		if provider != "anthropic" {
			t.Fatalf("provider = %q, want anthropic", provider)
		}
		return &auth.AuthCredential{
			AccessToken: "anthropic-token",
		}, nil
	}

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Model = "test-claude-oauth"
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName:  "test-claude-oauth",
			Model:      "anthropic/claude-sonnet-4.6",
			AuthMethod: "oauth",
		},
	}

	provider, _, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}

	if _, ok := provider.(*external.ExternalLLMProvider); !ok {
		t.Fatalf("provider type = %T, want *external.ExternalLLMProvider", provider)
	}
}
