package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
)

// MessageBusServices 返回 MessageBus 的反向调用服务处理函数。
//
// 注册的方法：
//   - host.bus.publish_outbound — 向频道发送消息
func MessageBusServices(mb *bus.MessageBus) map[string]ServiceHandler {
	return map[string]ServiceHandler{
		"host.bus.publish_outbound": func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				Channel          string `json:"channel"`
				ChatID           string `json:"chat_id"`
				Content          string `json:"content"`
				ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			if p.Channel == "" || p.Content == "" {
				return nil, fmt.Errorf("channel and content are required")
			}
			err := mb.PublishOutbound(ctx, bus.OutboundMessage{
				Channel:          p.Channel,
				ChatID:           p.ChatID,
				Content:          p.Content,
				ReplyToMessageID: p.ReplyToMessageID,
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		},
	}
}

// SkillsSearchFunc 是技能搜索的函数签名（避免直接依赖 skills 包）。
type SkillsSearchFunc func(ctx context.Context, query string, limit int) ([]map[string]any, error)

// SkillsInstallFunc 是技能安装的函数签名。
type SkillsInstallFunc func(ctx context.Context, slug, version, registry string, force bool) (map[string]any, error)

// SkillsServices 返回技能注册中心的反向调用服务处理函数。
//
// 注册的方法：
//   - host.skills.search — 搜索可安装的技能
//   - host.skills.install — 安装技能
func SkillsServices(searchFn SkillsSearchFunc, installFn SkillsInstallFunc) map[string]ServiceHandler {
	services := make(map[string]ServiceHandler)

	if searchFn != nil {
		services["host.skills.search"] = func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			if p.Query == "" {
				return nil, fmt.Errorf("query is required")
			}
			if p.Limit <= 0 {
				p.Limit = 5
			}
			return searchFn(ctx, p.Query, p.Limit)
		}
	}

	if installFn != nil {
		services["host.skills.install"] = func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				Slug     string `json:"slug"`
				Version  string `json:"version"`
				Registry string `json:"registry"`
				Force    bool   `json:"force"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			if p.Slug == "" || p.Registry == "" {
				return nil, fmt.Errorf("slug and registry are required")
			}
			return installFn(ctx, p.Slug, p.Version, p.Registry, p.Force)
		}
	}

	return services
}

// CronServiceAdapter 是 CronService 操作的接口（避免直接依赖 cron 包）。
type CronServiceAdapter interface {
	AddJob(ctx context.Context, name, channel, chatID string, schedule, payload map[string]any) (map[string]any, error)
	ListJobs(ctx context.Context, includeDisabled bool) ([]map[string]any, error)
	RemoveJob(ctx context.Context, jobID string) error
	EnableJob(ctx context.Context, jobID string, enabled bool) error
}

// CronServices 返回 CronService 的反向调用服务处理函数。
//
// 注册的方法：
//   - host.cron.add — 添加定时任务
//   - host.cron.list — 列出定时任务
//   - host.cron.remove — 删除定时任务
//   - host.cron.enable — 启用/禁用定时任务
func CronServices(adapter CronServiceAdapter) map[string]ServiceHandler {
	return map[string]ServiceHandler{
		"host.cron.add": func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				Name     string         `json:"name"`
				Channel  string         `json:"channel"`
				ChatID   string         `json:"chat_id"`
				Schedule map[string]any `json:"schedule"`
				Payload  map[string]any `json:"payload"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return adapter.AddJob(ctx, p.Name, p.Channel, p.ChatID, p.Schedule, p.Payload)
		},
		"host.cron.list": func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				IncludeDisabled bool `json:"include_disabled"`
			}
			if params != nil {
				_ = json.Unmarshal(params, &p)
			}
			return adapter.ListJobs(ctx, p.IncludeDisabled)
		},
		"host.cron.remove": func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return nil, adapter.RemoveJob(ctx, p.JobID)
		},
		"host.cron.enable": func(ctx context.Context, params json.RawMessage) (any, error) {
			var p struct {
				JobID   string `json:"job_id"`
				Enabled bool   `json:"enabled"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return nil, adapter.EnableJob(ctx, p.JobID, p.Enabled)
		},
	}
}

// ToolRegistryAdapter 是工具注册中心操作的接口。
type ToolRegistryAdapter interface {
	SearchHiddenTools(query string, limit int) ([]map[string]any, error)
	PromoteTools(names []string, ttl int)
}

// ToolRegistryServices 返回 ToolRegistry 的反向调用服务处理函数。
//
// 注册的方法：
//   - host.tools.search — 搜索隐藏工具
//   - host.tools.promote — 提升工具 TTL
func ToolRegistryServices(adapter ToolRegistryAdapter) map[string]ServiceHandler {
	return map[string]ServiceHandler{
		"host.tools.search": func(_ context.Context, params json.RawMessage) (any, error) {
			var p struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return adapter.SearchHiddenTools(p.Query, p.Limit)
		},
		"host.tools.promote": func(_ context.Context, params json.RawMessage) (any, error) {
			var p struct {
				Names []string `json:"names"`
				TTL   int      `json:"ttl"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			adapter.PromoteTools(p.Names, p.TTL)
			return map[string]any{"ok": true}, nil
		},
	}
}

// MergeServices 合并多个服务 map 为一个。
// 后面的 map 中的同名方法会覆盖前面的。
func MergeServices(maps ...map[string]ServiceHandler) map[string]ServiceHandler {
	merged := make(map[string]ServiceHandler)
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}
