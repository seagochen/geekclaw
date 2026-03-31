package agent

import (
	"sync"

	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
	"github.com/seagosoft/geekclaw/geekclaw/routing"
	"github.com/seagosoft/geekclaw/geekclaw/tools"
)

// AgentRegistry 管理多个代理实例并将消息路由到相应的代理。
type AgentRegistry struct {
	agents   map[string]*AgentInstance
	resolver *routing.RouteResolver
	mu       sync.RWMutex
}

// NewAgentRegistry 根据配置创建注册表，实例化所有代理。
func NewAgentRegistry(
	cfg *config.Config,
	provider providers.LLMProvider,
) *AgentRegistry {
	registry := &AgentRegistry{
		agents:   make(map[string]*AgentInstance),
		resolver: routing.NewRouteResolver(cfg),
	}

	agentConfigs := cfg.Agents.List
	if len(agentConfigs) == 0 {
		implicitAgent := &config.AgentConfig{
			ID:      "main",
			Default: true,
		}
		instance := NewAgentInstance(implicitAgent, &cfg.Agents.Defaults, cfg, provider)
		registry.agents["main"] = instance
		logger.InfoCF("agent", "Created implicit main agent (no agents.list configured)", nil)
	} else {
		for i := range agentConfigs {
			ac := &agentConfigs[i]
			id := routing.NormalizeAgentID(ac.ID)
			instance := NewAgentInstance(ac, &cfg.Agents.Defaults, cfg, provider)
			registry.agents[id] = instance
			logger.InfoCF("agent", "Registered agent",
				map[string]any{
					"agent_id":    id,
					"name":        ac.Name,
					"workspace":   instance.Workspace,
				"plugins_dir": instance.PluginsDir,
					"model":       instance.Model,
				})
		}
	}

	return registry
}

// GetAgent 返回指定 ID 的代理实例。
func (r *AgentRegistry) GetAgent(agentID string) (*AgentInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := routing.NormalizeAgentID(agentID)
	agent, ok := r.agents[id]
	return agent, ok
}

// ResolveRoute 确定由哪个代理处理该消息。
func (r *AgentRegistry) ResolveRoute(input routing.RouteInput) routing.ResolvedRoute {
	return r.resolver.ResolveRoute(input)
}

// ListAgentIDs 返回所有已注册的代理 ID。
func (r *AgentRegistry) ListAgentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// CanSpawnSubagent 检查 parentAgentID 是否被允许生成 targetAgentID。
func (r *AgentRegistry) CanSpawnSubagent(parentAgentID, targetAgentID string) bool {
	parent, ok := r.GetAgent(parentAgentID)
	if !ok {
		return false
	}
	if parent.Subagents == nil || parent.Subagents.AllowAgents == nil {
		return false
	}
	targetNorm := routing.NormalizeAgentID(targetAgentID)
	for _, allowed := range parent.Subagents.AllowAgents {
		if allowed == "*" {
			return true
		}
		if routing.NormalizeAgentID(allowed) == targetNorm {
			return true
		}
	}
	return false
}

// ForEachTool 对所有代理中指定名称的工具调用 fn。
// 用于在注册表构建后传播依赖（如 MediaStore）到工具。
func (r *AgentRegistry) ForEachTool(name string, fn func(tools.Tool)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, agent := range r.agents {
		if t, ok := agent.Tools.Get(name); ok {
			fn(t)
		}
	}
}

// Close 释放所有已注册代理持有的资源。
func (r *AgentRegistry) Close() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, agent := range r.agents {
		if err := agent.Close(); err != nil {
			logger.WarnCF("agent", "Failed to close agent",
				map[string]any{"agent_id": agent.ID, "error": err.Error()})
		}
	}
}

// GetDefaultAgent 返回默认代理实例。
func (r *AgentRegistry) GetDefaultAgent() *AgentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if agent, ok := r.agents["main"]; ok {
		return agent
	}
	for _, agent := range r.agents {
		return agent
	}
	return nil
}
