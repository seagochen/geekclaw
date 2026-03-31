// GeekClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/fileutil"
)

// MemoryStore 管理代理的持久化记忆。
// - 长期记忆：memory/MEMORY.md
// - 每日笔记：memory/YYYYMM/YYYYMMDD.md
type MemoryStore struct {
	pluginsDir string
	memoryDir  string
	memoryFile string
}

// NewMemoryStore 使用给定的插件目录路径创建一个新的 MemoryStore。
// 确保记忆目录存在。
func NewMemoryStore(pluginsDir string) *MemoryStore {
	memoryDir := filepath.Join(pluginsDir, "memory")
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")

	// 确保记忆目录存在
	os.MkdirAll(memoryDir, 0o755)

	return &MemoryStore{
		pluginsDir: pluginsDir,
		memoryDir:  memoryDir,
		memoryFile: memoryFile,
	}
}

// getTodayFile 返回今天每日笔记文件的路径（memory/YYYYMM/YYYYMMDD.md）。
func (ms *MemoryStore) getTodayFile() string {
	today := time.Now().Format("20060102") // YYYYMMDD
	monthDir := today[:6]                  // YYYYMM
	filePath := filepath.Join(ms.memoryDir, monthDir, today+".md")
	return filePath
}

// ReadLongTerm 读取长期记忆（MEMORY.md）。
// 如果文件不存在则返回空字符串。
func (ms *MemoryStore) ReadLongTerm() string {
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		return string(data)
	}
	return ""
}

// WriteLongTerm 将内容写入长期记忆文件（MEMORY.md）。
func (ms *MemoryStore) WriteLongTerm(content string) error {
	// 使用统一的原子写入工具，并显式同步以确保闪存存储的可靠性。
	// 使用 0o600（仅属主可读写）作为安全的默认权限。
	return fileutil.WriteFileAtomic(ms.memoryFile, []byte(content), 0o600)
}

// ReadToday 读取今天的每日笔记。
// 如果文件不存在则返回空字符串。
func (ms *MemoryStore) ReadToday() string {
	todayFile := ms.getTodayFile()
	if data, err := os.ReadFile(todayFile); err == nil {
		return string(data)
	}
	return ""
}

// AppendToday 将内容追加到今天的每日笔记。
// 如果文件不存在，则创建一个带有日期标题的新文件。
func (ms *MemoryStore) AppendToday(content string) error {
	todayFile := ms.getTodayFile()

	// 确保月份目录存在
	monthDir := filepath.Dir(todayFile)
	if err := os.MkdirAll(monthDir, 0o755); err != nil {
		return err
	}

	var existingContent string
	if data, err := os.ReadFile(todayFile); err == nil {
		existingContent = string(data)
	}

	var newContent string
	if existingContent == "" {
		// 为新的一天添加标题
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		newContent = header + content
	} else {
		// 追加到现有内容
		newContent = existingContent + "\n" + content
	}

	// 使用统一的原子写入工具，并显式同步以确保闪存存储的可靠性。
	return fileutil.WriteFileAtomic(todayFile, []byte(newContent), 0o600)
}

// GetRecentDailyNotes 返回最近 N 天的每日笔记。
// 各天内容以 "---" 分隔符连接。
func (ms *MemoryStore) GetRecentDailyNotes(days int) string {
	var sb strings.Builder
	first := true

	for i := range days {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102") // YYYYMMDD
		monthDir := dateStr[:6]            // YYYYMM
		filePath := filepath.Join(ms.memoryDir, monthDir, dateStr+".md")

		if data, err := os.ReadFile(filePath); err == nil {
			if !first {
				sb.WriteString("\n\n---\n\n")
			}
			sb.Write(data)
			first = false
		}
	}

	return sb.String()
}

// GetMemoryContext 返回格式化的记忆上下文，用于代理提示词。
// 包含长期记忆和近期每日笔记。
func (ms *MemoryStore) GetMemoryContext() string {
	longTerm := ms.ReadLongTerm()
	recentNotes := ms.GetRecentDailyNotes(3)

	if longTerm == "" && recentNotes == "" {
		return ""
	}

	var sb strings.Builder

	if longTerm != "" {
		sb.WriteString("## Long-term Memory\n\n")
		sb.WriteString(longTerm)
	}

	if recentNotes != "" {
		if longTerm != "" {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Recent Daily Notes\n\n")
		sb.WriteString(recentNotes)
	}

	return sb.String()
}
