// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package providers

import (
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// CreateProvider 根据配置创建提供者。
// 使用 model_list 配置（新格式）来创建提供者。
// 旧的 providers 配置会在加载时自动转换为 model_list。
// 返回提供者、使用的模型 ID 以及可能的错误。
func CreateProvider(cfg *config.Config) (LLMProvider, string, error) {
	model := cfg.Agents.Defaults.GetModelName()

	// 确保 model_list 从 providers 配置中填充（如需要）
	// 处理两种情况：
	// 1. ModelList 为空 - 转换所有提供者
	// 2. ModelList 有部分条目但未包含所有提供者 - 合并缺失的条目
	if cfg.HasProvidersConfig() {
		providerModels := config.ConvertProvidersToModelList(cfg)
		existingModelNames := make(map[string]bool)
		for _, m := range cfg.ModelList {
			existingModelNames[m.ModelName] = true
		}
		for _, pm := range providerModels {
			if !existingModelNames[pm.ModelName] {
				cfg.ModelList = append(cfg.ModelList, pm)
			}
		}
	}

	// 此时必须有 model_list
	if len(cfg.ModelList) == 0 {
		return nil, "", fmt.Errorf("no providers configured. Please add entries to model_list in your config")
	}

	// 从 model_list 获取模型配置
	modelCfg, err := cfg.GetModelConfig(model)
	if err != nil {
		return nil, "", fmt.Errorf("model %q not found in model_list: %w", model, err)
	}

	// 如果模型配置中未设置插件目录，则注入全局插件目录
	if modelCfg.PluginsDir == "" {
		modelCfg.PluginsDir = cfg.PluginsPath()
	}

	// 使用工厂方法创建提供者
	provider, modelID, err := CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create provider for model %q: %w", model, err)
	}

	return provider, modelID, nil
}
