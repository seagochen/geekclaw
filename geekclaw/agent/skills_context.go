// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// errWalkStop 是用于提前停止 filepath.WalkDir 的哨兵错误。
// 使用专用错误（而非 fs.SkipAll）使提前退出的意图更明确，
// 并避免当回调的 err 参数非 nil 时返回 nil 会触发的 nilerr 检查器警告。
var errWalkStop = errors.New("walk stop")

// skillRoots 返回所有可能影响 BuildSkillsSummary 输出的
// 技能根目录（插件/全局/内置）。
func (cb *ContextBuilder) skillRoots() []string {
	if cb.skillsLoader == nil {
		return []string{filepath.Join(cb.pluginsDir, "skills")}
	}

	roots := cb.skillsLoader.SkillRoots()
	if len(roots) == 0 {
		return []string{filepath.Join(cb.pluginsDir, "skills")}
	}
	return roots
}

// buildSkillsSection 返回系统提示词中的技能部分，
// 如果没有可用技能则返回空字符串。
func (cb *ContextBuilder) buildSkillsSection() string {
	skillsSummary := cb.skillsLoader.BuildSkillsSummary()
	if skillsSummary == "" {
		return ""
	}
	return fmt.Sprintf("# Skills\n\nThe following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.\n\n%s", skillsSummary)
}

// skillFilesChangedSince 将当前递归技能文件树与缓存时的快照进行比较。
// 任何创建/删除/mtime 偏差都会使缓存失效。
func skillFilesChangedSince(skillRoots []string, filesAtCache map[string]time.Time) bool {
	// 防御性检查：如果快照从未初始化，强制重建。
	if filesAtCache == nil {
		return true
	}

	// 检查缓存的文件是否仍然存在且 mtime 相同。
	for path, cachedMtime := range filesAtCache {
		info, err := os.Stat(path)
		if err != nil {
			// 之前跟踪的文件消失（或变得不可访问）：
			// 无论哪种情况，缓存的技能摘要可能已过时。
			return true
		}
		if !info.ModTime().Equal(cachedMtime) {
			return true
		}
	}

	// 检查是否有新文件出现在任何技能根目录下。
	changed := false
	for _, root := range skillRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				// 将意外的遍历错误视为已变更，以避免缓存过时。
				if !os.IsNotExist(walkErr) {
					changed = true
					return errWalkStop
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if _, ok := filesAtCache[path]; !ok {
				changed = true
				return errWalkStop
			}
			return nil
		})

		if changed {
			return true
		}
		if err != nil && !errors.Is(err, errWalkStop) && !os.IsNotExist(err) {
			logger.DebugCF("agent", "skills walk error", map[string]any{"error": err.Error()})
			return true
		}
	}

	return false
}

// GetSkillsInfo 返回已加载技能的信息。
func (cb *ContextBuilder) GetSkillsInfo() map[string]any {
	allSkills := cb.skillsLoader.ListSkills()
	skillNames := make([]string, 0, len(allSkills))
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}
	return map[string]any{
		"total":     len(allSkills),
		"available": len(allSkills),
		"names":     skillNames,
	}
}
