// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package config

import (
	"fmt"
	"sync/atomic"
)

// rrCounter 是用于模型间轮询负载均衡的全局计数器。
var rrCounter atomic.Uint64

// Validate 检查 ModelConfig 是否包含所有必填字段。
func (c *ModelConfig) Validate() error {
	if c.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

// GetAPIKey 从旧版 providers 配置中返回 API 密钥。
// 已弃用：请使用 model_list 代替。
func (c *Config) GetAPIKey() string {
	if c.Providers.OpenRouter.APIKey != "" {
		return c.Providers.OpenRouter.APIKey
	}
	if c.Providers.Anthropic.APIKey != "" {
		return c.Providers.Anthropic.APIKey
	}
	if c.Providers.OpenAI.APIKey != "" {
		return c.Providers.OpenAI.APIKey
	}
	if c.Providers.Gemini.APIKey != "" {
		return c.Providers.Gemini.APIKey
	}
	if c.Providers.Zhipu.APIKey != "" {
		return c.Providers.Zhipu.APIKey
	}
	if c.Providers.Groq.APIKey != "" {
		return c.Providers.Groq.APIKey
	}
	if c.Providers.VLLM.APIKey != "" {
		return c.Providers.VLLM.APIKey
	}
	if c.Providers.ShengSuanYun.APIKey != "" {
		return c.Providers.ShengSuanYun.APIKey
	}
	if c.Providers.Cerebras.APIKey != "" {
		return c.Providers.Cerebras.APIKey
	}
	return ""
}

// GetAPIBase 从旧版 providers 配置中返回 API 基础 URL。
// 已弃用：请使用 model_list 代替。
func (c *Config) GetAPIBase() string {
	if c.Providers.OpenRouter.APIKey != "" {
		if c.Providers.OpenRouter.APIBase != "" {
			return c.Providers.OpenRouter.APIBase
		}
		return "https://openrouter.ai/api/v1"
	}
	if c.Providers.Zhipu.APIKey != "" {
		return c.Providers.Zhipu.APIBase
	}
	if c.Providers.VLLM.APIKey != "" && c.Providers.VLLM.APIBase != "" {
		return c.Providers.VLLM.APIBase
	}
	return ""
}

// GetModelConfig 返回指定模型名称的 ModelConfig。
// 如果存在多个同名配置，使用轮询选择以实现负载均衡。
// 如果未找到模型则返回错误。
func (c *Config) GetModelConfig(modelName string) (*ModelConfig, error) {
	matches := c.findMatches(modelName)
	if len(matches) == 0 {
		return nil, fmt.Errorf("model %q not found in model_list or providers", modelName)
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}

	// 多个配置——使用轮询进行负载均衡
	idx := rrCounter.Add(1) % uint64(len(matches))
	return &matches[idx], nil
}

// findMatches 查找所有具有指定 model_name 的 ModelConfig 条目。
func (c *Config) findMatches(modelName string) []ModelConfig {
	var matches []ModelConfig
	for i := range c.ModelList {
		if c.ModelList[i].ModelName == modelName {
			matches = append(matches, c.ModelList[i])
		}
	}
	return matches
}

// HasProvidersConfig 检查旧版 providers 配置中是否有任何提供者已配置。
func (c *Config) HasProvidersConfig() bool {
	return !c.Providers.IsEmpty()
}

// ValidateModelList 验证 model_list 中所有 ModelConfig 条目。
// 检查每个模型配置是否有效。
// 注意：允许多个具有相同 model_name 的条目以实现负载均衡。
func (c *Config) ValidateModelList() error {
	for i := range c.ModelList {
		if err := c.ModelList[i].Validate(); err != nil {
			return fmt.Errorf("model_list[%d]: %w", i, err)
		}
	}
	return nil
}
