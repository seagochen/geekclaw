// Package media 管理媒体文件的生命周期。
package media

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// MediaMeta 保存已存储媒体文件的元数据。
type MediaMeta struct {
	Filename    string
	ContentType string
	Source      string // "telegram"、"discord"、"tool:image-gen" 等
}

// MediaStore 管理与处理作用域关联的媒体文件生命周期。
type MediaStore interface {
	// Store 在给定作用域下注册已有的本地文件。
	// 返回引用标识符（例如 "media://<id>"）。
	// Store 不移动或复制文件；只记录映射关系。
	Store(localPath string, meta MediaMeta, scope string) (ref string, err error)

	// Resolve 返回给定引用对应的本地文件路径。
	Resolve(ref string) (localPath string, err error)

	// ResolveWithMeta 返回给定引用对应的本地文件路径和元数据。
	ResolveWithMeta(ref string) (localPath string, meta MediaMeta, err error)

	// ReleaseAll 删除给定作用域下注册的所有文件
	// 并移除映射条目。文件不存在的错误将被忽略。
	ReleaseAll(scope string) error
}

// mediaEntry 保存已存储媒体文件的路径和元数据。
type mediaEntry struct {
	path     string
	meta     MediaMeta
	storedAt time.Time
}

// MediaCleanerConfig 配置后台 TTL 清理。
type MediaCleanerConfig struct {
	Enabled  bool
	MaxAge   time.Duration
	Interval time.Duration
}

// FileMediaStore 是 MediaStore 的纯内存实现。
// 文件应已存在于磁盘上（例如 /tmp/geekclaw_media/）。
type FileMediaStore struct {
	mu          sync.RWMutex
	refs        map[string]mediaEntry
	scopeToRefs map[string]map[string]struct{}
	refToScope  map[string]string

	cleanerCfg MediaCleanerConfig
	stop       chan struct{}
	startOnce  sync.Once
	stopOnce   sync.Once
	nowFunc    func() time.Time // 用于测试
}

// NewFileMediaStore 创建一个不带后台清理的新 FileMediaStore。
func NewFileMediaStore() *FileMediaStore {
	return &FileMediaStore{
		refs:        make(map[string]mediaEntry),
		scopeToRefs: make(map[string]map[string]struct{}),
		refToScope:  make(map[string]string),
		nowFunc:     time.Now,
	}
}

// NewFileMediaStoreWithCleanup 创建一个带 TTL 后台清理的 FileMediaStore。
func NewFileMediaStoreWithCleanup(cfg MediaCleanerConfig) *FileMediaStore {
	return &FileMediaStore{
		refs:        make(map[string]mediaEntry),
		scopeToRefs: make(map[string]map[string]struct{}),
		refToScope:  make(map[string]string),
		cleanerCfg:  cfg,
		stop:        make(chan struct{}),
		nowFunc:     time.Now,
	}
}

// Store 在给定作用域下注册本地文件。文件必须存在。
func (s *FileMediaStore) Store(localPath string, meta MediaMeta, scope string) (string, error) {
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("media store: %s: %w", localPath, err)
	}

	ref := "media://" + uuid.New().String()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.refs[ref] = mediaEntry{path: localPath, meta: meta, storedAt: s.nowFunc()}
	if s.scopeToRefs[scope] == nil {
		s.scopeToRefs[scope] = make(map[string]struct{})
	}
	s.scopeToRefs[scope][ref] = struct{}{}
	s.refToScope[ref] = scope

	return ref, nil
}

// Resolve 返回给定引用对应的本地路径。
func (s *FileMediaStore) Resolve(ref string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.refs[ref]
	if !ok {
		return "", fmt.Errorf("media store: unknown ref: %s", ref)
	}
	return entry.path, nil
}

// ResolveWithMeta 返回给定引用对应的本地路径和元数据。
func (s *FileMediaStore) ResolveWithMeta(ref string) (string, MediaMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.refs[ref]
	if !ok {
		return "", MediaMeta{}, fmt.Errorf("media store: unknown ref: %s", ref)
	}
	return entry.path, entry.meta, nil
}

// ReleaseAll 移除给定作用域下的所有文件并清理映射。
// 第一阶段（持有锁）：从 map 中移除条目。
// 第二阶段（无锁）：从磁盘删除文件。
func (s *FileMediaStore) ReleaseAll(scope string) error {
	// 第一阶段：在持有锁的情况下收集路径并从 map 中移除
	var paths []string

	s.mu.Lock()
	refs, ok := s.scopeToRefs[scope]
	if !ok {
		s.mu.Unlock()
		return nil
	}

	for ref := range refs {
		if entry, exists := s.refs[ref]; exists {
			paths = append(paths, entry.path)
		}
		delete(s.refs, ref)
		delete(s.refToScope, ref)
	}
	delete(s.scopeToRefs, scope)
	s.mu.Unlock()

	// 第二阶段：在不持有锁的情况下删除文件
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			logger.WarnCF("media", "release: failed to remove file", map[string]any{
				"path":  p,
				"error": err.Error(),
			})
		}
	}

	return nil
}

// CleanExpired 移除所有超过 MaxAge 的条目。
// 第一阶段（持有锁）：识别过期条目并从 map 中移除。
// 第二阶段（无锁）：从磁盘删除文件以减少锁竞争。
func (s *FileMediaStore) CleanExpired() int {
	if s.cleanerCfg.MaxAge <= 0 {
		return 0
	}

	// 第一阶段：在持有锁的情况下收集过期条目
	type expiredEntry struct {
		ref  string
		path string
	}

	s.mu.Lock()
	cutoff := s.nowFunc().Add(-s.cleanerCfg.MaxAge)
	var expired []expiredEntry

	for ref, entry := range s.refs {
		if entry.storedAt.Before(cutoff) {
			expired = append(expired, expiredEntry{ref: ref, path: entry.path})

			if scope, ok := s.refToScope[ref]; ok {
				if scopeRefs, ok := s.scopeToRefs[scope]; ok {
					delete(scopeRefs, ref)
					if len(scopeRefs) == 0 {
						delete(s.scopeToRefs, scope)
					}
				}
			}

			delete(s.refs, ref)
			delete(s.refToScope, ref)
		}
	}
	s.mu.Unlock()

	// 第二阶段：在不持有锁的情况下删除文件
	for _, e := range expired {
		if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
			logger.WarnCF("media", "cleanup: failed to remove file", map[string]any{
				"path":  e.path,
				"error": err.Error(),
			})
		}
	}

	return len(expired)
}

// Start 启动后台清理协程（如果清理已启用）。
// 可安全多次调用；只有第一次调用会启动协程。
func (s *FileMediaStore) Start() {
	if !s.cleanerCfg.Enabled || s.stop == nil {
		return
	}
	if s.cleanerCfg.Interval <= 0 || s.cleanerCfg.MaxAge <= 0 {
		logger.WarnCF("media", "cleanup: skipped due to invalid config", map[string]any{
			"interval": s.cleanerCfg.Interval.String(),
			"max_age":  s.cleanerCfg.MaxAge.String(),
		})
		return
	}

	s.startOnce.Do(func() {
		logger.InfoCF("media", "cleanup enabled", map[string]any{
			"interval": s.cleanerCfg.Interval.String(),
			"max_age":  s.cleanerCfg.MaxAge.String(),
		})

		go func() {
			ticker := time.NewTicker(s.cleanerCfg.Interval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if n := s.CleanExpired(); n > 0 {
						logger.InfoCF("media", "cleanup: removed expired entries", map[string]any{
							"count": n,
						})
					}
				case <-s.stop:
					return
				}
			}
		}()
	})
}

// Stop 终止后台清理协程。
// 可安全多次调用；只有第一次调用会关闭通道。
func (s *FileMediaStore) Stop() {
	if s.stop == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stop)
	})
}
