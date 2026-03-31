package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/providers"
)

// setupWorkspace 创建包含标准目录和可选文件的临时工作空间。
// 返回 tmpDir 路径；调用方应 defer os.RemoveAll(tmpDir)。
func setupWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "geekclaw-test-*")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(tmpDir, "memory"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "skills"), 0o755)
	for name, content := range files {
		dir := filepath.Dir(filepath.Join(tmpDir, name))
		os.MkdirAll(dir, 0o755)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return tmpDir
}

// TestSingleSystemMessage 验证 BuildMessages 无论 summary/history 如何变化，
// 始终恰好产生一条 system 消息。
// 修复：多条 system 消息会导致 Anthropic（顶级 system 参数）和
// Codex（仅读取最后一条 system 消息作为指令）出错。
func TestSingleSystemMessage(t *testing.T) {
	tmpDir := setupWorkspace(t, map[string]string{
		"persona/IDENTITY.md": "# Identity\nTest agent.",
	})
	defer os.RemoveAll(tmpDir)

	cb := NewContextBuilder(tmpDir)

	tests := []struct {
		name    string
		history []providers.Message
		summary string
		message string
	}{
		{
			name:    "no summary, no history",
			summary: "",
			message: "hello",
		},
		{
			name:    "with summary",
			summary: "Previous conversation discussed X",
			message: "hello",
		},
		{
			name: "with history and summary",
			history: []providers.Message{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
			},
			summary: strings.Repeat("Long summary text. ", 50),
			message: "new message",
		},
		{
			name: "system message in history is filtered",
			history: []providers.Message{
				{Role: "system", Content: "stale system prompt from previous session"},
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
			},
			summary: "",
			message: "new message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := cb.BuildMessages(tt.history, tt.summary, tt.message, nil, "test", "chat1")

			systemCount := 0
			for _, m := range msgs {
				if m.Role == "system" {
					systemCount++
				}
			}
			if systemCount != 1 {
				t.Errorf("expected exactly 1 system message, got %d", systemCount)
			}
			if msgs[0].Role != "system" {
				t.Errorf("first message should be system, got %s", msgs[0].Role)
			}
			if msgs[len(msgs)-1].Role != "user" {
				t.Errorf("last message should be user, got %s", msgs[len(msgs)-1].Role)
			}

			// system 消息必须包含身份信息（静态）和当前时间（动态）
			sys := msgs[0].Content
			if !strings.Contains(sys, "geekclaw") {
				t.Error("system message missing identity")
			}
			if !strings.Contains(sys, "Current Time") {
				t.Error("system message missing dynamic time context")
			}

			// 摘要处理
			if tt.summary != "" {
				if !strings.Contains(sys, "CONTEXT_SUMMARY:") {
					t.Error("summary present but CONTEXT_SUMMARY prefix missing")
				}
				if !strings.Contains(sys, tt.summary[:20]) {
					t.Error("summary content not found in system message")
				}
			} else {
				if strings.Contains(sys, "CONTEXT_SUMMARY:") {
					t.Error("CONTEXT_SUMMARY should not appear without summary")
				}
			}
		})
	}
}

// TestMtimeAutoInvalidation 验证缓存可通过 mtime 检测源文件变更，
// 无需显式调用 InvalidateCache()。
// 修复：原始实现没有自动失效机制 —— 对 bootstrap 文件、
// memory 或 skills 的修改在进程重启前不可见。
func TestMtimeAutoInvalidation(t *testing.T) {
	tests := []struct {
		name       string
		file       string // relative path inside workspace
		contentV1  string
		contentV2  string
		checkField string // substring to verify in rebuilt prompt
	}{
		{
			name:       "bootstrap file change",
			file:       "persona/IDENTITY.md",
			contentV1:  "# Original Identity",
			contentV2:  "# Updated Identity",
			checkField: "Updated Identity",
		},
		{
			name:       "memory file change",
			file:       "memory/MEMORY.md",
			contentV1:  "# Memory\nUser likes Go.",
			contentV2:  "# Memory\nUser likes Rust.",
			checkField: "User likes Rust",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := setupWorkspace(t, map[string]string{tt.file: tt.contentV1})
			defer os.RemoveAll(tmpDir)

			cb := NewContextBuilder(tmpDir)

			sp1 := cb.BuildSystemPromptWithCache()

			// 覆写文件并设置未来 mtime 以确保检测到变更。
			// 使用 2s 偏移量以保障文件系统 mtime 精度（部分文件系统
			// 精度为 1s 甚至更低，在 CI 容器中尤为明显）。
			fullPath := filepath.Join(tmpDir, tt.file)
			os.WriteFile(fullPath, []byte(tt.contentV2), 0o644)
			future := time.Now().Add(2 * time.Second)
			os.Chtimes(fullPath, future, future)

			// 验证 sourceFilesChangedLocked 检测到 mtime 变更
			cb.systemPromptMutex.RLock()
			changed := cb.sourceFilesChangedLocked()
			cb.systemPromptMutex.RUnlock()
			if !changed {
				t.Fatalf("sourceFilesChangedLocked() should detect %s change", tt.file)
			}

			// 应无需显式调用 InvalidateCache() 即自动重建
			sp2 := cb.BuildSystemPromptWithCache()
			if sp1 == sp2 {
				t.Errorf("cache not rebuilt after %s change", tt.file)
			}
			if !strings.Contains(sp2, tt.checkField) {
				t.Errorf("rebuilt prompt missing expected content %q", tt.checkField)
			}
		})
	}

	// skills 目录 mtime 变更
	t.Run("skills dir change", func(t *testing.T) {
		tmpDir := setupWorkspace(t, nil)
		defer os.RemoveAll(tmpDir)

		cb := NewContextBuilder(tmpDir)
		_ = cb.BuildSystemPromptWithCache() // populate cache

		// 触碰 skills 目录（模拟新技能安装）
		skillsDir := filepath.Join(tmpDir, "skills")
		future := time.Now().Add(2 * time.Second)
		os.Chtimes(skillsDir, future, future)

		// 验证 sourceFilesChangedLocked 检测到变更（缓存将被重建）
		// 通过检查内部状态确认：第二次调用应触发重建。
		cb.systemPromptMutex.RLock()
		changed := cb.sourceFilesChangedLocked()
		cb.systemPromptMutex.RUnlock()
		if !changed {
			t.Error("sourceFilesChangedLocked() should detect skills dir mtime change")
		}
	})
}

// TestExplicitInvalidateCache 验证 InvalidateCache() 即使源文件未变更
// 也能强制重建（适用于测试和重载命令）。
func TestExplicitInvalidateCache(t *testing.T) {
	tmpDir := setupWorkspace(t, map[string]string{
		"persona/IDENTITY.md": "# Test Identity",
	})
	defer os.RemoveAll(tmpDir)

	cb := NewContextBuilder(tmpDir)

	sp1 := cb.BuildSystemPromptWithCache()
	cb.InvalidateCache()
	sp2 := cb.BuildSystemPromptWithCache()

	if sp1 != sp2 {
		t.Error("prompt should be identical after invalidate+rebuild when files unchanged")
	}

	// 验证 cachedAt 已被重置
	cb.InvalidateCache()
	cb.systemPromptMutex.RLock()
	if !cb.cachedAt.IsZero() {
		t.Error("cachedAt should be zero after InvalidateCache()")
	}
	cb.systemPromptMutex.RUnlock()
}

// TestCacheStability 验证在文件未变更时静态提示在重复调用间保持稳定
// （issue #607 的回归测试）。
func TestCacheStability(t *testing.T) {
	tmpDir := setupWorkspace(t, map[string]string{
		"persona/IDENTITY.md": "# Identity\nContent",
		"persona/SOUL.md":     "# Soul\nContent",
	})
	defer os.RemoveAll(tmpDir)

	cb := NewContextBuilder(tmpDir)

	results := make([]string, 5)
	for i := range results {
		results[i] = cb.BuildSystemPromptWithCache()
	}
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("cached prompt changed between call 0 and %d", i)
		}
	}

	// 静态提示不得包含每次请求专属数据
	if strings.Contains(results[0], "Current Time") {
		t.Error("static cached prompt should not contain time (added dynamically)")
	}
}

// TestNewFileCreationInvalidatesCache 验证创建一个缓存构建时不存在的源文件
// 会触发缓存重建。
// 捕获旧版 modifiedSince（stat 错误时返回 false）会遗漏的
// "从无到有" 边缘情况。
func TestNewFileCreationInvalidatesCache(t *testing.T) {
	tests := []struct {
		name       string
		file       string // relative path inside workspace
		content    string
		checkField string // substring to verify in rebuilt prompt
	}{
		{
			name:       "new bootstrap file",
			file:       "persona/SOUL.md",
			content:    "# Soul\nBe kind and helpful.",
			checkField: "Be kind and helpful",
		},
		{
			name:       "new memory file",
			file:       "memory/MEMORY.md",
			content:    "# Memory\nUser prefers dark mode.",
			checkField: "User prefers dark mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 从空工作空间开始（无 bootstrap/memory 文件）
			tmpDir := setupWorkspace(t, nil)
			defer os.RemoveAll(tmpDir)

			cb := NewContextBuilder(tmpDir)

			// 填充缓存 —— 文件尚不存在
			sp1 := cb.BuildSystemPromptWithCache()
			if strings.Contains(sp1, tt.checkField) {
				t.Fatalf("prompt should not contain %q before file is created", tt.checkField)
			}

			// 在缓存构建之后创建文件
			fullPath := filepath.Join(tmpDir, tt.file)
			os.MkdirAll(filepath.Dir(fullPath), 0o755)
			if err := os.WriteFile(fullPath, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			// 设置未来 mtime 以保证检测到变更
			future := time.Now().Add(2 * time.Second)
			os.Chtimes(fullPath, future, future)

			// 缓存应自动失效，因为文件从不存在变为存在
			sp2 := cb.BuildSystemPromptWithCache()
			if !strings.Contains(sp2, tt.checkField) {
				t.Errorf("cache not invalidated on new file creation: expected %q in prompt", tt.checkField)
			}
		})
	}
}

// TestSkillFileContentChange 验证修改技能文件内容（而非仅改变目录结构）
// 会使缓存失效。
// 此场景中仅靠目录 mtime 不够 —— 在大多数文件系统上，
// 修改目录内的文件不会更新父目录的 mtime。
func TestSkillFileContentChange(t *testing.T) {
	skillMD := `---
name: test-skill
description: "A test skill"
---
# Test Skill v1
Original content.`

	tmpDir := setupWorkspace(t, map[string]string{
		"skills/test-skill/SKILL.md": skillMD,
	})
	defer os.RemoveAll(tmpDir)

	cb := NewContextBuilder(tmpDir)

	// 填充缓存
	sp1 := cb.BuildSystemPromptWithCache()
	_ = sp1 // 缓存已预热

	// 修改技能文件内容（不触碰 skills/ 目录）
	updatedSkillMD := `---
name: test-skill
description: "An updated test skill"
---
# Test Skill v2
Updated content.`

	skillPath := filepath.Join(tmpDir, "skills", "test-skill", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(updatedSkillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	// 仅对技能文件设置未来 mtime（不修改目录）
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(skillPath, future, future)

	// 验证 sourceFilesChangedLocked 检测到内容变更
	cb.systemPromptMutex.RLock()
	changed := cb.sourceFilesChangedLocked()
	cb.systemPromptMutex.RUnlock()
	if !changed {
		t.Error("sourceFilesChangedLocked() should detect skill file content change")
	}

	// 验证缓存确实以新内容重建
	sp2 := cb.BuildSystemPromptWithCache()
	if sp1 == sp2 && strings.Contains(sp1, "test-skill") {
		// 若技能出现在提示中但提示未变化，则缓存未失效。
		t.Error("cache should be invalidated when skill file content changes")
	}
}

// TestGlobalSkillFileContentChange 验证修改全局技能
// (~/.geekclaw/skills) 会使缓存的系统提示失效。
func TestGlobalSkillFileContentChange(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tmpDir := setupWorkspace(t, nil)
	defer os.RemoveAll(tmpDir)

	globalSkillPath := filepath.Join(tmpHome, ".geekclaw", "skills", "global-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(globalSkillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	v1 := `---
name: global-skill
description: global-v1
---
# Global Skill v1`
	if err := os.WriteFile(globalSkillPath, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	cb := NewContextBuilder(tmpDir)
	sp1 := cb.BuildSystemPromptWithCache()
	if !strings.Contains(sp1, "global-v1") {
		t.Fatal("expected initial prompt to contain global skill description")
	}

	v2 := `---
name: global-skill
description: global-v2
---
# Global Skill v2`
	if err := os.WriteFile(globalSkillPath, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(globalSkillPath, future, future); err != nil {
		t.Fatalf("failed to update mtime for %s: %v", globalSkillPath, err)
	}

	cb.systemPromptMutex.RLock()
	changed := cb.sourceFilesChangedLocked()
	cb.systemPromptMutex.RUnlock()
	if !changed {
		t.Fatal("sourceFilesChangedLocked() should detect global skill file content change")
	}

	sp2 := cb.BuildSystemPromptWithCache()
	if !strings.Contains(sp2, "global-v2") {
		t.Error("rebuilt prompt should contain updated global skill description")
	}
	if sp1 == sp2 {
		t.Error("cache should be invalidated when global skill file content changes")
	}
}

// TestBuiltinSkillFileContentChange 验证修改内置技能
// 会使缓存的系统提示失效。
func TestBuiltinSkillFileContentChange(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tmpDir := setupWorkspace(t, nil)
	defer os.RemoveAll(tmpDir)

	builtinRoot := t.TempDir()
	t.Setenv("GEEKCLAW_BUILTIN_SKILLS", builtinRoot)

	builtinSkillPath := filepath.Join(builtinRoot, "builtin-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(builtinSkillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	v1 := `---
name: builtin-skill
description: builtin-v1
---
# Builtin Skill v1`
	if err := os.WriteFile(builtinSkillPath, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	cb := NewContextBuilder(tmpDir)
	sp1 := cb.BuildSystemPromptWithCache()
	if !strings.Contains(sp1, "builtin-v1") {
		t.Fatal("expected initial prompt to contain builtin skill description")
	}

	v2 := `---
name: builtin-skill
description: builtin-v2
---
# Builtin Skill v2`
	if err := os.WriteFile(builtinSkillPath, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(builtinSkillPath, future, future); err != nil {
		t.Fatalf("failed to update mtime for %s: %v", builtinSkillPath, err)
	}

	cb.systemPromptMutex.RLock()
	changed := cb.sourceFilesChangedLocked()
	cb.systemPromptMutex.RUnlock()
	if !changed {
		t.Fatal("sourceFilesChangedLocked() should detect builtin skill file content change")
	}

	sp2 := cb.BuildSystemPromptWithCache()
	if !strings.Contains(sp2, "builtin-v2") {
		t.Error("rebuilt prompt should contain updated builtin skill description")
	}
	if sp1 == sp2 {
		t.Error("cache should be invalidated when builtin skill file content changes")
	}
}

// TestSkillFileDeletionInvalidatesCache 验证删除嵌套技能文件
// 会使缓存的系统提示失效。
func TestSkillFileDeletionInvalidatesCache(t *testing.T) {
	tmpDir := setupWorkspace(t, map[string]string{
		"skills/delete-me/SKILL.md": `---
name: delete-me
description: delete-me-v1
---
# Delete Me`,
	})
	defer os.RemoveAll(tmpDir)

	cb := NewContextBuilder(tmpDir)
	sp1 := cb.BuildSystemPromptWithCache()
	if !strings.Contains(sp1, "delete-me-v1") {
		t.Fatal("expected initial prompt to contain skill description")
	}

	skillPath := filepath.Join(tmpDir, "skills", "delete-me", "SKILL.md")
	if err := os.Remove(skillPath); err != nil {
		t.Fatal(err)
	}

	cb.systemPromptMutex.RLock()
	changed := cb.sourceFilesChangedLocked()
	cb.systemPromptMutex.RUnlock()
	if !changed {
		t.Fatal("sourceFilesChangedLocked() should detect deleted skill file")
	}

	sp2 := cb.BuildSystemPromptWithCache()
	if strings.Contains(sp2, "delete-me-v1") {
		t.Error("rebuilt prompt should not contain deleted skill description")
	}
	if sp1 == sp2 {
		t.Error("cache should be invalidated when skill file is deleted")
	}
}

// TestConcurrentBuildSystemPromptWithCache 验证多个 goroutine 可安全并发
// 调用 BuildSystemPromptWithCache，不产生空结果、panic 或数据竞争。
// 运行方式：go test -race ./pkg/agent/ -run TestConcurrentBuildSystemPromptWithCache
func TestConcurrentBuildSystemPromptWithCache(t *testing.T) {
	tmpDir := setupWorkspace(t, map[string]string{
		"persona/IDENTITY.md":  "# Identity\nConcurrency test agent.",
		"persona/SOUL.md":      "# Soul\nBe helpful.",
		"memory/MEMORY.md":     "# Memory\nUser prefers Go.",
		"skills/demo/SKILL.md": "---\nname: demo\ndescription: \"demo skill\"\n---\n# Demo",
	})
	defer os.RemoveAll(tmpDir)

	cb := NewContextBuilder(tmpDir)

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	errs := make(chan string, goroutines*iterations)

	for g := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range iterations {
				result := cb.BuildSystemPromptWithCache()
				if result == "" {
					errs <- "empty prompt returned"
					return
				}
				if !strings.Contains(result, "geekclaw") {
					errs <- "prompt missing identity"
					return
				}

				// 同时并发测试 BuildMessages
				msgs := cb.BuildMessages(nil, "", "hello", nil, "test", "chat")
				if len(msgs) < 2 {
					errs <- "BuildMessages returned fewer than 2 messages"
					return
				}
				if msgs[0].Role != "system" {
					errs <- "first message not system"
					return
				}

				// 偶尔失效以测试写路径
				if i%10 == 0 {
					cb.InvalidateCache()
				}
			}
		}(g)
	}

	wg.Wait()
	close(errs)

	for errMsg := range errs {
		t.Errorf("concurrent access error: %s", errMsg)
	}
}

// BenchmarkBuildMessagesWithCache 测量缓存性能。

// TestEmptyWorkspaceBaselineDetectsNewFiles 验证在空工作空间（无任何被追踪文件）
// 构建缓存后，事后创建文件仍能触发缓存失效。
// 验证 maxMtime 的 time.Unix(1, 0) 回退值：任何真实文件的 mtime 都晚于 epoch，
// 因此 fileChangedSince 能正确检测"不存在 -> 存在"的转变，
// 且无需人工修改 Chtimes 即可通过 mtime 比较。
func TestEmptyWorkspaceBaselineDetectsNewFiles(t *testing.T) {
	// 空工作空间：无 bootstrap 文件、无 memory、无 skills 内容。
	tmpDir := setupWorkspace(t, nil)
	defer os.RemoveAll(tmpDir)

	cb := NewContextBuilder(tmpDir)

	// 构建缓存 —— 所有被追踪文件均不存在，maxMtime 回退至 epoch。
	sp1 := cb.BuildSystemPromptWithCache()

	// 以自然 mtime 创建 bootstrap 文件（不操作 Chtimes）。
	// 文件的 mtime 应为当前系统时间，严格晚于 time.Unix(1, 0)。
	soulPath := filepath.Join(tmpDir, "persona", "SOUL.md")
	os.MkdirAll(filepath.Dir(soulPath), 0o755)
	if err := os.WriteFile(soulPath, []byte("# Soul\nNewly created."), 0o644); err != nil {
		t.Fatal(err)
	}

	// 缓存应通过 existedAtCache 检测到新文件（不存在 -> 存在）。
	cb.systemPromptMutex.RLock()
	changed := cb.sourceFilesChangedLocked()
	cb.systemPromptMutex.RUnlock()
	if !changed {
		t.Fatal("sourceFilesChangedLocked should detect newly created file on empty workspace")
	}

	sp2 := cb.BuildSystemPromptWithCache()
	if !strings.Contains(sp2, "Newly created") {
		t.Error("rebuilt prompt should contain new file content")
	}
	if sp1 == sp2 {
		t.Error("cache should have been invalidated after file creation")
	}
}

// BenchmarkBuildMessagesWithCache 测量缓存性能。
func BenchmarkBuildMessagesWithCache(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "geekclaw-bench-*")
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, "memory"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "skills"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "persona"), 0o755)
	for _, name := range []string{"IDENTITY.md", "SOUL.md", "USER.md"} {
		os.WriteFile(filepath.Join(tmpDir, "persona", name), []byte(strings.Repeat("Content.\n", 10)), 0o644)
	}

	cb := NewContextBuilder(tmpDir)
	history := []providers.Message{
		{Role: "user", Content: "previous message"},
		{Role: "assistant", Content: "previous response"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.BuildMessages(history, "summary", "new message", nil, "cli", "test")
	}
}
