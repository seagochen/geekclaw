package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
)

const (
	// MaxRegexPatternLength 正则表达式模式的最大长度。
	MaxRegexPatternLength = 200
)

// RegexSearchTool 使用正则表达式模式搜索隐藏工具。
type RegexSearchTool struct {
	registry         *ToolRegistry
	ttl              int
	maxSearchResults int
}

// NewRegexSearchTool 创建一个新的 RegexSearchTool。
func NewRegexSearchTool(r *ToolRegistry, ttl int, maxSearchResults int) *RegexSearchTool {
	return &RegexSearchTool{registry: r, ttl: ttl, maxSearchResults: maxSearchResults}
}

// Name 返回工具名称。
func (t *RegexSearchTool) Name() string {
	return "tool_search_tool_regex"
}

// Description 返回工具描述。
func (t *RegexSearchTool) Description() string {
	return "Search available hidden tools on-demand using a regex pattern. Returns JSON schemas of discovered tools."
}

// Parameters 返回工具参数的 schema。
func (t *RegexSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to match tool name or description",
			},
		},
		"required": []string{"pattern"},
	}
}

// Execute 执行正则搜索操作。
func (t *RegexSearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	pattern, ok := args["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		// 空字符串正则 (?i) 会匹配每个隐藏工具，
		// 向上下文中倾倒大量负载并消耗 token。
		return ErrorResult("Missing or invalid 'pattern' argument. Must be a non-empty string.")
	}

	if len(pattern) > MaxRegexPatternLength {
		logger.WarnCF("discovery", "Regex pattern rejected (too long)", map[string]any{"len": len(pattern)})
		return ErrorResult(fmt.Sprintf("Pattern too long: max %d characters allowed", MaxRegexPatternLength))
	}

	logger.DebugCF("discovery", "Regex search", map[string]any{"pattern": pattern})

	res, err := t.registry.SearchRegex(pattern, t.maxSearchResults)
	if err != nil {
		logger.WarnCF("discovery", "Invalid regex pattern", map[string]any{"pattern": pattern, "error": err.Error()})
		return ErrorResult(fmt.Sprintf("Invalid regex pattern syntax: %v. Please fix your regex and try again.", err))
	}

	logger.InfoCF("discovery", "Regex search completed", map[string]any{"pattern": pattern, "results": len(res)})
	return formatDiscoveryResponse(t.registry, res, t.ttl)
}

// BM25SearchTool 使用自然语言查询搜索隐藏工具。
type BM25SearchTool struct {
	registry         *ToolRegistry
	ttl              int
	maxSearchResults int

	// 缓存：仅在注册表版本更改时重建。
	cacheMu      sync.Mutex
	cachedEngine *bm25CachedEngine
	cacheVersion uint64
}

// NewBM25SearchTool 创建一个新的 BM25SearchTool。
func NewBM25SearchTool(r *ToolRegistry, ttl int, maxSearchResults int) *BM25SearchTool {
	return &BM25SearchTool{registry: r, ttl: ttl, maxSearchResults: maxSearchResults}
}

// Name 返回工具名称。
func (t *BM25SearchTool) Name() string {
	return "tool_search_tool_bm25"
}

// Description 返回工具描述。
func (t *BM25SearchTool) Description() string {
	return "Search available hidden tools on-demand using natural language query describing the action you need to perform. Returns JSON schemas of discovered tools."
}

// Parameters 返回工具参数的 schema。
func (t *BM25SearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
		},
		"required": []string{"query"},
	}
}

// Execute 执行 BM25 搜索操作。
func (t *BM25SearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		// 空字符串查询会匹配每个隐藏工具，
		// 向上下文中倾倒大量负载并消耗 token。
		return ErrorResult("Missing or invalid 'query' argument. Must be a non-empty string.")
	}

	logger.DebugCF("discovery", "BM25 search", map[string]any{"query": query})

	cached := t.getOrBuildEngine()
	if cached == nil {
		logger.DebugCF("discovery", "BM25 search: no hidden tools available", nil)
		return SilentResult("No tools found matching the query.")
	}

	ranked := cached.engine.Search(query, t.maxSearchResults)
	if len(ranked) == 0 {
		logger.DebugCF("discovery", "BM25 search: no matches", map[string]any{"query": query})
		return SilentResult("No tools found matching the query.")
	}

	results := make([]ToolSearchResult, len(ranked))
	for i, r := range ranked {
		results[i] = ToolSearchResult{
			Name:        r.Document.Name,
			Description: r.Document.Description,
		}
	}

	logger.InfoCF("discovery", "BM25 search completed", map[string]any{"query": query, "results": len(results)})
	return formatDiscoveryResponse(t.registry, results, t.ttl)
}

// ToolSearchResult 表示返回给 LLM 的搜索结果。
// Parameters 从 JSON 响应中省略以节省上下文 token；
// LLM 将在提升后通过 ToProviderDefs 看到完整的 schema。
type ToolSearchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SearchRegex 使用正则表达式在隐藏工具中搜索。
func (r *ToolRegistry) SearchRegex(pattern string, maxSearchResults int) ([]ToolSearchResult, error) {
	if maxSearchResults <= 0 {
		return nil, nil
	}

	regex, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex pattern %q: %w", pattern, err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []ToolSearchResult

	// 按排序顺序迭代以确保跨调用的确定性结果。
	for _, name := range r.sortedToolNames() {
		entry := r.tools[name]
		// 仅在隐藏工具中搜索（核心工具已经可见）
		if !entry.IsCore {
			// 直接调用接口方法！无需反射/反序列化。
			desc := entry.Tool.Description()

			if regex.MatchString(name) || regex.MatchString(desc) {
				results = append(results, ToolSearchResult{
					Name:        name,
					Description: desc,
				})
				if len(results) >= maxSearchResults {
					break // 达到上限后停止搜索！节省 CPU。
				}
			}
		}
	}

	return results, nil
}

// formatDiscoveryResponse 格式化发现响应并提升匹配的工具。
func formatDiscoveryResponse(registry *ToolRegistry, results []ToolSearchResult, ttl int) *ToolResult {
	if len(results) == 0 {
		return SilentResult("No tools found matching the query.")
	}

	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	registry.PromoteTools(names, ttl)
	logger.InfoCF("discovery", "Promoted tools", map[string]any{"tools": names, "ttl": ttl})

	b, err := json.Marshal(results)
	if err != nil {
		return ErrorResult("Failed to format search results: " + err.Error())
	}

	msg := fmt.Sprintf(
		"Found %d tools:\n%s\n\nSUCCESS: These tools have been temporarily UNLOCKED as native tools! In your next response, you can call them directly just like any normal tool",
		len(results),
		string(b),
	)

	return SilentResult(msg)
}

// searchDoc 是用作 BM25 语料库文档的轻量级内部类型。
type searchDoc struct {
	Name        string
	Description string
}

// bm25CachedEngine 包装一个 BM25Engine 及其语料库快照。
type bm25CachedEngine struct {
	engine *utils.BM25Engine[searchDoc]
}

// snapshotToSearchDocs 将 HiddenToolSnapshot 转换为 BM25 searchDoc 切片。
func snapshotToSearchDocs(snap HiddenToolSnapshot) []searchDoc {
	docs := make([]searchDoc, len(snap.Docs))
	for i, d := range snap.Docs {
		docs[i] = searchDoc{Name: d.Name, Description: d.Description}
	}
	return docs
}

// buildBM25Engine 从 searchDocs 切片创建 BM25Engine。
func buildBM25Engine(docs []searchDoc) *utils.BM25Engine[searchDoc] {
	return utils.NewBM25Engine(
		docs,
		func(doc searchDoc) string {
			return doc.Name + " " + doc.Description
		},
	)
}

// getOrBuildEngine 返回缓存的 BM25 引擎，仅在注册表版本更改（注册了新工具）时重建。
func (t *BM25SearchTool) getOrBuildEngine() *bm25CachedEngine {
	// 快速路径：不加锁的乐观检查。
	if t.cachedEngine != nil && t.cacheVersion == t.registry.Version() {
		return t.cachedEngine
	}

	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()

	// 快照 + 版本在单个注册表 RLock 下读取，
	// 保证一致性（无 TOCTOU）。
	snap := t.registry.SnapshotHiddenTools()

	// 重新检查：等待 cacheMu 时另一个 goroutine 可能已经重建。
	if t.cachedEngine != nil && t.cacheVersion == snap.Version {
		return t.cachedEngine
	}

	docs := snapshotToSearchDocs(snap)
	if len(docs) == 0 {
		t.cachedEngine = nil
		t.cacheVersion = snap.Version
		return nil
	}

	cached := &bm25CachedEngine{engine: buildBM25Engine(docs)}
	t.cachedEngine = cached
	t.cacheVersion = snap.Version
	logger.DebugCF("discovery", "BM25 engine rebuilt", map[string]any{"docs": len(docs), "version": snap.Version})
	return cached
}

// SearchBM25 使用 BM25 通过 utils.BM25Engine 对隐藏工具进行查询排名。
// 此非缓存变体每次调用都会重建引擎。供测试
// 和没有 BM25SearchTool 实例的代码使用。
func (r *ToolRegistry) SearchBM25(query string, maxSearchResults int) []ToolSearchResult {
	snap := r.SnapshotHiddenTools()
	docs := snapshotToSearchDocs(snap)
	if len(docs) == 0 {
		return nil
	}

	ranked := buildBM25Engine(docs).Search(query, maxSearchResults)
	if len(ranked) == 0 {
		return nil
	}

	out := make([]ToolSearchResult, len(ranked))
	for i, r := range ranked {
		out[i] = ToolSearchResult{
			Name:        r.Document.Name,
			Description: r.Document.Description,
		}
	}
	return out
}
