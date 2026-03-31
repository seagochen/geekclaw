package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// ToolEntry 表示注册表中的一个工具条目。
type ToolEntry struct {
	Tool   Tool
	IsCore bool
	TTL    int
}

// ToolRegistry 是工具的注册表，管理工具的注册、查找和执行。
type ToolRegistry struct {
	tools   map[string]*ToolEntry
	mu      sync.RWMutex
	version atomic.Uint64 // 在 Register/RegisterHidden 时递增，用于缓存失效
}

// NewToolRegistry 创建一个新的工具注册表。
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*ToolEntry),
	}
}

// Register 注册一个核心工具。
func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		logger.WarnCF("tools", "Tool registration overwrites existing tool",
			map[string]any{"name": name})
	}
	r.tools[name] = &ToolEntry{
		Tool:   tool,
		IsCore: true,
		TTL:    0, // 核心工具不使用 TTL
	}
	r.version.Add(1)
	logger.DebugCF("tools", "Registered core tool", map[string]any{"name": name})
}

// RegisterHidden 保存隐藏工具（仅通过 TTL 可见）。
func (r *ToolRegistry) RegisterHidden(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		logger.WarnCF("tools", "Hidden tool registration overwrites existing tool",
			map[string]any{"name": name})
	}
	r.tools[name] = &ToolEntry{
		Tool:   tool,
		IsCore: false,
		TTL:    0,
	}
	r.version.Add(1)
	logger.DebugCF("tools", "Registered hidden tool", map[string]any{"name": name})
}

// PromoteTools 原子地为多个非核心工具设置 TTL。
// 这防止并发的 TickTTL 在提升之间递减。
func (r *ToolRegistry) PromoteTools(names []string, ttl int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	promoted := 0
	for _, name := range names {
		if entry, exists := r.tools[name]; exists {
			if !entry.IsCore {
				entry.TTL = ttl
				promoted++
			}
		}
	}
	logger.DebugCF(
		"tools",
		"PromoteTools completed",
		map[string]any{"requested": len(names), "promoted": promoted, "ttl": ttl},
	)
}

// TickTTL 仅对非核心工具减少 TTL。
func (r *ToolRegistry) TickTTL() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.tools {
		if !entry.IsCore && entry.TTL > 0 {
			entry.TTL--
		}
	}
}

// Version 返回当前注册表版本（原子操作）。
func (r *ToolRegistry) Version() uint64 {
	return r.version.Load()
}

// HiddenToolSnapshot 保存隐藏工具的一致快照以及
// 获取时的注册表版本。供 BM25SearchTool 缓存使用。
type HiddenToolSnapshot struct {
	Docs    []HiddenToolDoc
	Version uint64
}

// HiddenToolDoc 是用于搜索索引的隐藏工具的轻量级表示。
type HiddenToolDoc struct {
	Name        string
	Description string
}

// SnapshotHiddenTools 在单个读锁下返回所有非核心工具和当前注册表版本，
// 保证两个值之间的一致性。
func (r *ToolRegistry) SnapshotHiddenTools() HiddenToolSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	docs := make([]HiddenToolDoc, 0, len(r.tools))
	for name, entry := range r.tools {
		if !entry.IsCore {
			docs = append(docs, HiddenToolDoc{
				Name:        name,
				Description: entry.Tool.Description(),
			})
		}
	}
	return HiddenToolSnapshot{
		Docs:    docs,
		Version: r.version.Load(),
	}
}

// Get 根据名称获取工具。
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	if !ok {
		return nil, false
	}
	// TTL 过期的隐藏工具不可调用。
	if !entry.IsCore && entry.TTL <= 0 {
		return nil, false
	}
	return entry.Tool, true
}

// Execute 执行指定名称的工具。
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any) *ToolResult {
	return r.ExecuteWithContext(ctx, name, args, "", "", nil)
}

// ExecuteWithContext 使用 channel/chatID 上下文和可选的异步回调执行工具。
// 如果工具实现了 AsyncExecutor 且提供了非 nil 回调，
// 将调用 ExecuteAsync 而非 Execute——回调是参数，
// 不会作为可变状态存储在工具上。
func (r *ToolRegistry) ExecuteWithContext(
	ctx context.Context,
	name string,
	args map[string]any,
	channel, chatID string,
	asyncCallback AsyncCallback,
) *ToolResult {
	logger.InfoCF("tool", "Tool execution started",
		map[string]any{
			"tool": name,
			"args": args,
		})

	tool, ok := r.Get(name)
	if !ok {
		logger.ErrorCF("tool", "Tool not found",
			map[string]any{
				"tool": name,
			})
		return ErrorResult(fmt.Sprintf("tool %q not found", name)).WithError(fmt.Errorf("tool not found"))
	}

	// 将 channel/chatID 注入到 ctx 中，以便工具通过 ToolChannel(ctx)/ToolChatID(ctx) 读取。
	// 始终注入——工具自行验证所需内容。
	ctx = WithToolContext(ctx, channel, chatID)

	// 如果工具实现了 AsyncExecutor 且提供了回调，使用 ExecuteAsync。
	// 回调是调用参数，不是工具实例上的可变状态。
	var result *ToolResult
	start := time.Now()
	if asyncExec, ok := tool.(AsyncExecutor); ok && asyncCallback != nil {
		logger.DebugCF("tool", "Executing async tool via ExecuteAsync",
			map[string]any{
				"tool": name,
			})
		result = asyncExec.ExecuteAsync(ctx, args, asyncCallback)
	} else {
		result = tool.Execute(ctx, args)
	}
	duration := time.Since(start)

	// 根据结果类型记录日志
	if result.IsError {
		logger.ErrorCF("tool", "Tool execution failed",
			map[string]any{
				"tool":     name,
				"duration": duration.Milliseconds(),
				"error":    result.ForLLM,
			})
	} else if result.Async {
		logger.InfoCF("tool", "Tool started (async)",
			map[string]any{
				"tool":     name,
				"duration": duration.Milliseconds(),
			})
	} else {
		logger.InfoCF("tool", "Tool execution completed",
			map[string]any{
				"tool":          name,
				"duration_ms":   duration.Milliseconds(),
				"result_length": len(result.ForLLM),
			})
	}

	return result
}

// sortedToolNames 返回按排序顺序的工具名称，用于确定性迭代。
// 这对 KV 缓存稳定性至关重要：非确定性的 map 迭代会在每次调用时
// 产生不同的系统提示和工具定义，即使没有工具更改也会使
// LLM 的前缀缓存失效。
func (r *ToolRegistry) sortedToolNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetDefinitions 返回所有已注册工具的定义。
func (r *ToolRegistry) GetDefinitions() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	definitions := make([]map[string]any, 0, len(sorted))
	for _, name := range sorted {
		entry := r.tools[name]

		if !entry.IsCore && entry.TTL <= 0 {
			continue
		}

		definitions = append(definitions, ToolToSchema(r.tools[name].Tool))
	}
	return definitions
}

// ToProviderDefs 将工具定义转换为提供者兼容格式。
// 这是 LLM 提供者 API 期望的格式。
func (r *ToolRegistry) ToProviderDefs() []providers.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	definitions := make([]providers.ToolDefinition, 0, len(sorted))
	for _, name := range sorted {
		entry := r.tools[name]

		if !entry.IsCore && entry.TTL <= 0 {
			continue
		}

		schema := ToolToSchema(entry.Tool)

		// 通过类型检查安全提取嵌套值
		fn, ok := schema["function"].(map[string]any)
		if !ok {
			continue
		}

		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)

		definitions = append(definitions, providers.ToolDefinition{
			Type: "function",
			Function: providers.ToolFunctionDefinition{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}
	return definitions
}

// List 返回所有已注册工具名称的列表。
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sortedToolNames()
}

// Count 返回已注册工具的数量。
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// GetSummaries 返回所有已注册工具的人类可读摘要。
// 返回 "name - description" 字符串的切片。
func (r *ToolRegistry) GetSummaries() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	summaries := make([]string, 0, len(sorted))
	for _, name := range sorted {
		entry := r.tools[name]

		if !entry.IsCore && entry.TTL <= 0 {
			continue
		}

		summaries = append(summaries, fmt.Sprintf("- `%s` - %s", entry.Tool.Name(), entry.Tool.Description()))
	}
	return summaries
}
