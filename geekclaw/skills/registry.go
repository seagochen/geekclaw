package skills

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultMaxConcurrentSearches = 2 // 默认最大并发搜索数
)

// SearchResult 表示技能注册中心搜索的单个结果。
type SearchResult struct {
	Score        float64 `json:"score"`         // 相关性分数
	Slug         string  `json:"slug"`          // 技能标识符
	DisplayName  string  `json:"display_name"`  // 显示名称
	Summary      string  `json:"summary"`       // 摘要描述
	Version      string  `json:"version"`       // 版本号
	RegistryName string  `json:"registry_name"` // 来源注册中心名称
}

// SkillMeta 保存来自注册中心的技能元数据。
type SkillMeta struct {
	Slug             string `json:"slug"`               // 技能标识符
	DisplayName      string `json:"display_name"`       // 显示名称
	Summary          string `json:"summary"`            // 摘要描述
	LatestVersion    string `json:"latest_version"`     // 最新版本
	IsMalwareBlocked bool   `json:"is_malware_blocked"` // 是否因恶意软件被阻止
	IsSuspicious     bool   `json:"is_suspicious"`      // 是否被标记为可疑
	RegistryName     string `json:"registry_name"`      // 来源注册中心名称
}

// InstallResult 由 DownloadAndInstall 返回，携带元数据
// 供调用方用于审核决策和用户消息。
type InstallResult struct {
	Version          string // 安装的版本
	IsMalwareBlocked bool   // 是否因恶意软件被阻止
	IsSuspicious     bool   // 是否被标记为可疑
	Summary          string // 技能摘要
}

// SkillRegistry 是所有技能注册中心必须实现的接口。
// 每个注册中心代表不同的技能来源（例如 clawhub.ai）。
type SkillRegistry interface {
	// Name 返回此注册中心的唯一名称（例如 "clawhub"）。
	Name() string
	// Search 在注册中心中搜索匹配查询的技能。
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
	// GetSkillMeta 根据 slug 检索特定技能的元数据。
	GetSkillMeta(ctx context.Context, slug string) (*SkillMeta, error)
	// DownloadAndInstall 获取元数据、解析版本、下载并将技能安装到 targetDir。
	// 返回 InstallResult，其中包含供调用方用于审核和用户消息的元数据。
	DownloadAndInstall(ctx context.Context, slug, version, targetDir string) (*InstallResult, error)
}

// RegistryConfig 保存所有技能注册中心的配置。
// 这是 NewRegistryManagerFromConfig 的输入。
type RegistryConfig struct {
	ClawHub               ClawHubConfig // ClawHub 注册中心配置
	MaxConcurrentSearches int           // 最大并发搜索数
}

// ClawHubConfig 配置 ClawHub 注册中心。
type ClawHubConfig struct {
	Enabled         bool   // 是否启用
	BaseURL         string // 基础 URL
	AuthToken       string // 认证令牌
	SearchPath      string // 搜索路径，例如 "/api/v1/search"
	SkillsPath      string // 技能路径，例如 "/api/v1/skills"
	DownloadPath    string // 下载路径，例如 "/api/v1/download"
	Timeout         int    // 超时时间（秒），0 表示默认值（30 秒）
	MaxZipSize      int    // 最大 ZIP 大小（字节），0 表示默认值（50MB）
	MaxResponseSize int    // 最大响应大小（字节），0 表示默认值（2MB）
}

// RegistryManager 协调多个技能注册中心。
// 它将搜索请求扇出到各注册中心，并将安装请求路由到正确的注册中心。
type RegistryManager struct {
	registries    []SkillRegistry
	maxConcurrent int
	mu            sync.RWMutex
}

// NewRegistryManager 创建一个空的 RegistryManager。
func NewRegistryManager() *RegistryManager {
	return &RegistryManager{
		registries:    make([]SkillRegistry, 0),
		maxConcurrent: defaultMaxConcurrentSearches,
	}
}

// NewRegistryManagerFromConfig 根据配置构建 RegistryManager，
// 仅实例化已启用的注册中心。
func NewRegistryManagerFromConfig(cfg RegistryConfig) *RegistryManager {
	rm := NewRegistryManager()
	if cfg.MaxConcurrentSearches > 0 {
		rm.maxConcurrent = cfg.MaxConcurrentSearches
	}
	if cfg.ClawHub.Enabled {
		rm.AddRegistry(NewClawHubRegistry(cfg.ClawHub))
	}
	return rm
}

// AddRegistry 向管理器添加一个注册中心。
func (rm *RegistryManager) AddRegistry(r SkillRegistry) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.registries = append(rm.registries, r)
}

// GetRegistry 根据名称返回注册中心，如果未找到则返回 nil。
func (rm *RegistryManager) GetRegistry(name string) SkillRegistry {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	for _, r := range rm.registries {
		if r.Name() == name {
			return r
		}
	}
	return nil
}

// SearchAll 将查询并发扇出到所有注册中心，
// 并将结果按分数降序合并。
func (rm *RegistryManager) SearchAll(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	rm.mu.RLock()
	regs := make([]SkillRegistry, len(rm.registries))
	copy(regs, rm.registries)
	rm.mu.RUnlock()

	if len(regs) == 0 {
		return nil, fmt.Errorf("no registries configured")
	}

	type regResult struct {
		results []SearchResult
		err     error
	}

	// 信号量：限制并发数。
	sem := make(chan struct{}, rm.maxConcurrent)
	resultsCh := make(chan regResult, len(regs))

	var wg sync.WaitGroup
	for _, reg := range regs {
		wg.Add(1)
		go func(r SkillRegistry) {
			defer wg.Done()

			// 获取信号量槽位。
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultsCh <- regResult{err: ctx.Err()}
				return
			}

			searchCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()

			results, err := r.Search(searchCtx, query, limit)
			if err != nil {
				slog.Warn("registry search failed", "registry", r.Name(), "error", err)
				resultsCh <- regResult{err: err}
				return
			}
			resultsCh <- regResult{results: results}
		}(reg)
	}

	// 所有 goroutine 完成后关闭结果通道。
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var merged []SearchResult
	var lastErr error

	var anyRegistrySucceeded bool
	for rr := range resultsCh {
		if rr.err != nil {
			lastErr = rr.err
			continue
		}
		anyRegistrySucceeded = true
		merged = append(merged, rr.results...)
	}

	// 如果所有注册中心都失败了，返回最后一个错误。
	if !anyRegistrySucceeded && lastErr != nil {
		return nil, fmt.Errorf("all registries failed: %w", lastErr)
	}

	// 按分数降序排序。
	sortByScoreDesc(merged)

	// 限制结果数量。
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}

	return merged, nil
}

// sortByScoreDesc 按 Score 降序排列 SearchResult（插入排序——适用于小切片）。
func sortByScoreDesc(results []SearchResult) {
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Score < key.Score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}
