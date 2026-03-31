// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package providers

import (
	"fmt"
	"testing"

	"github.com/seagosoft/geekclaw/geekclaw/config"
)

func TestExtractProtocol(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		wantProtocol string
		wantModelID  string
	}{
		{
			name:         "openai with prefix",
			model:        "openai/gpt-4o",
			wantProtocol: "openai",
			wantModelID:  "gpt-4o",
		},
		{
			name:         "anthropic with prefix",
			model:        "anthropic/claude-sonnet-4.6",
			wantProtocol: "anthropic",
			wantModelID:  "claude-sonnet-4.6",
		},
		{
			name:         "no prefix - defaults to openai",
			model:        "gpt-4o",
			wantProtocol: "openai",
			wantModelID:  "gpt-4o",
		},
		{
			name:         "groq with prefix",
			model:        "groq/llama-3.1-70b",
			wantProtocol: "groq",
			wantModelID:  "llama-3.1-70b",
		},
		{
			name:         "empty string",
			model:        "",
			wantProtocol: "openai",
			wantModelID:  "",
		},
		{
			name:         "with whitespace",
			model:        "  openai/gpt-4  ",
			wantProtocol: "openai",
			wantModelID:  "gpt-4",
		},
		{
			name:         "multiple slashes",
			model:        "nvidia/meta/llama-3.1-8b",
			wantProtocol: "nvidia",
			wantModelID:  "meta/llama-3.1-8b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, modelID := ExtractProtocol(tt.model)
			if protocol != tt.wantProtocol {
				t.Errorf("ExtractProtocol(%q) protocol = %q, want %q", tt.model, protocol, tt.wantProtocol)
			}
			if modelID != tt.wantModelID {
				t.Errorf("ExtractProtocol(%q) modelID = %q, want %q", tt.model, modelID, tt.wantModelID)
			}
		})
	}
}

func TestGetDefaultAPIBase(t *testing.T) {
	tests := []struct {
		protocol string
		want     string
	}{
		{"openai", "https://api.openai.com/v1"},
		{"openrouter", "https://openrouter.ai/api/v1"},
		{"litellm", "http://localhost:4000/v1"},
		{"groq", "https://api.groq.com/openai/v1"},
		{"ollama", "http://localhost:11434/v1"},
		{"deepseek", "https://api.deepseek.com/v1"},
		{"vllm", "http://localhost:8000/v1"},
		{"anthropic", "https://api.anthropic.com/v1"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			if got := getDefaultAPIBase(tt.protocol); got != tt.want {
				t.Errorf("getDefaultAPIBase(%q) = %q, want %q", tt.protocol, got, tt.want)
			}
		})
	}
}

func TestResolvePluginModule(t *testing.T) {
	tests := []struct {
		name       string
		protocol   string
		authMethod string
		want       string
		wantErr    bool
	}{
		{"openai api key", "openai", "", "providers.contrib.openai_compat", false},
		{"openai oauth", "openai", "oauth", "providers.contrib.codex", false},
		{"openai token", "openai", "token", "providers.contrib.codex", false},
		{"groq", "groq", "", "providers.contrib.openai_compat", false},
		{"openrouter", "openrouter", "", "providers.contrib.openai_compat", false},
		{"deepseek", "deepseek", "", "providers.contrib.openai_compat", false},
		{"anthropic api key", "anthropic", "", "providers.contrib.openai_compat", false},
		{"anthropic oauth", "anthropic", "oauth", "providers.contrib.anthropic_native", false},
		{"claude-cli", "claude-cli", "", "providers.contrib.claude_cli", false},
		{"claudecli", "claudecli", "", "providers.contrib.claude_cli", false},
		{"claude-code", "claude-code", "", "providers.contrib.claude_cli", false},
		{"codex-cli", "codex-cli", "", "providers.contrib.codex_cli", false},
		{"codexcli", "codexcli", "", "providers.contrib.codex_cli", false},
		{"unknown", "foobar", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ModelConfig{AuthMethod: tt.authMethod}
			got, err := resolvePluginModule(tt.protocol, cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolvePluginModule(%q) error = %v, wantErr %v", tt.protocol, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("resolvePluginModule(%q) = %q, want %q", tt.protocol, got, tt.want)
			}
		})
	}
}

func TestBuildPluginConfig(t *testing.T) {
	cfg := &config.ModelConfig{
		APIKey:         "test-key",
		APIBase:        "https://api.example.com/v1",
		Proxy:          "http://proxy:8080",
		MaxTokensField: "max_completion_tokens",
		RequestTimeout: 60,
		PluginsDir:     "/opt/plugins",
		PluginConfig:   map[string]any{"custom": "value"},
	}

	pc, err := buildPluginConfig(cfg, "openai")
	if err != nil {
		t.Fatalf("buildPluginConfig() error = %v", err)
	}

	checks := map[string]any{
		"api_key":          "test-key",
		"api_base":         "https://api.example.com/v1",
		"proxy":            "http://proxy:8080",
		"max_tokens_field": "max_completion_tokens",
		"request_timeout":  60,
		"workspace":        "/opt/plugins",
		"custom":           "value",
	}

	for k, want := range checks {
		got, ok := pc[k]
		if !ok {
			t.Errorf("buildPluginConfig() missing key %q", k)
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("buildPluginConfig()[%q] = %v, want %v", k, got, want)
		}
	}
}

func TestBuildPluginConfig_DefaultAPIBase(t *testing.T) {
	cfg := &config.ModelConfig{
		APIKey: "test-key",
	}

	pc, err := buildPluginConfig(cfg, "groq")
	if err != nil {
		t.Fatalf("buildPluginConfig() error = %v", err)
	}

	if pc["api_base"] != "https://api.groq.com/openai/v1" {
		t.Errorf("api_base = %q, want groq default", pc["api_base"])
	}
}

func TestResolveAuthProvider(t *testing.T) {
	if got := resolveAuthProvider("openai"); got != "openai" {
		t.Errorf("resolveAuthProvider(openai) = %q, want openai", got)
	}
	if got := resolveAuthProvider("anthropic"); got != "anthropic" {
		t.Errorf("resolveAuthProvider(anthropic) = %q, want anthropic", got)
	}
	if got := resolveAuthProvider("groq"); got != "" {
		t.Errorf("resolveAuthProvider(groq) = %q, want empty", got)
	}
}

func TestCreateProviderFromConfig_NilConfig(t *testing.T) {
	_, _, err := CreateProviderFromConfig(nil)
	if err == nil {
		t.Fatal("CreateProviderFromConfig(nil) expected error")
	}
}

func TestCreateProviderFromConfig_EmptyModel(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-empty",
		Model:     "",
	}

	_, _, err := CreateProviderFromConfig(cfg)
	if err == nil {
		t.Fatal("CreateProviderFromConfig() expected error for empty model")
	}
}

func TestCreateProviderFromConfig_UnknownProtocol(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-unknown",
		Model:     "unknown-protocol/model",
		APIKey:    "test-key",
	}

	_, _, err := CreateProviderFromConfig(cfg)
	if err == nil {
		t.Fatal("CreateProviderFromConfig() expected error for unknown protocol")
	}
}

func TestCreateProviderFromConfig_ExternalRequiresPluginCommand(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-external",
		Model:     "external/my-model",
	}

	_, _, err := CreateProviderFromConfig(cfg)
	if err == nil {
		t.Fatal("CreateProviderFromConfig() expected error for external without plugin_command")
	}
}
