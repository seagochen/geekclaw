package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
	"github.com/seagosoft/geekclaw/geekclaw/skills"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
)

// ContextBuilder 负责构建系统提示词，包括身份、技能、记忆等上下文信息。
type ContextBuilder struct {
	pluginsDir         string
	skillsLoader       *skills.SkillsLoader
	memory             *MemoryStore
	toolDiscoveryBM25  bool
	toolDiscoveryRegex bool

	// 系统提示词缓存，避免每次调用都重新构建。
	// 修复了 issue #607：重复处理整个上下文的问题。
	// 当插件目录源文件发生变更（mtime 检查）时缓存自动失效。
	systemPromptMutex  sync.RWMutex
	cachedSystemPrompt string
	cachedAt           time.Time // 缓存构建时所有跟踪路径的最大 mtime

	// existedAtCache 记录上次构建缓存时各源文件路径是否存在。
	// 这使得 sourceFilesChanged 能检测到新创建的文件（缓存时不存在，现在存在）
	// 或已删除的文件（缓存时存在，现在消失）—— 两种情况都应触发缓存重建。
	existedAtCache map[string]bool

	// skillFilesAtCache 快照缓存构建时的技能文件树及其 mtime。
	// 用于捕获嵌套文件的创建/删除/mtime 变更，
	// 这些变更可能不会更新顶层技能根目录的 mtime。
	skillFilesAtCache map[string]time.Time
}

// WithToolDiscovery 配置工具发现的搜索方法（BM25 和/或正则表达式）。
func (cb *ContextBuilder) WithToolDiscovery(useBM25, useRegex bool) *ContextBuilder {
	cb.toolDiscoveryBM25 = useBM25
	cb.toolDiscoveryRegex = useRegex
	return cb
}

// getGlobalConfigDir 返回全局配置目录路径。
func getGlobalConfigDir() string {
	if home := config.GeekclawHome(); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".geekclaw")
}

// NewContextBuilder 创建一个新的上下文构建器，使用指定的插件目录。
func NewContextBuilder(pluginsDir string) *ContextBuilder {
	// 内置技能：GEEKCLAW_HOME/plugins/skills 目录中的技能
	builtinSkillsDir := strings.TrimSpace(config.BuiltinSkills())
	if builtinSkillsDir == "" {
		geekclawHome := config.GeekclawHome()
		if geekclawHome == "" {
			home, _ := os.UserHomeDir()
			geekclawHome = filepath.Join(home, ".geekclaw")
		}
		builtinSkillsDir = filepath.Join(geekclawHome, "plugins", "skills")
	}
	globalSkillsDir := filepath.Join(getGlobalConfigDir(), "skills")

	return &ContextBuilder{
		pluginsDir:   pluginsDir,
		skillsLoader: skills.NewSkillsLoader(pluginsDir, globalSkillsDir, builtinSkillsDir),
		memory:       NewMemoryStore(pluginsDir),
	}
}

// getIdentity 返回代理的核心身份提示词，包含版本、插件目录和重要规则。
func (cb *ContextBuilder) getIdentity() string {
	pluginsPath, _ := filepath.Abs(cb.pluginsDir)
	toolDiscovery := cb.getDiscoveryRule()
	version := config.FormatVersion()

	return fmt.Sprintf(
		`# geekclaw 🦞 (%s)

You are geekclaw, a helpful AI assistant.

## Plugins Directory
Your plugins directory is at: %s
- Persona: %s/persona/{IDENTITY,SOUL,USER,AGENTS}.md
- Memory: %s/memory/MEMORY.md
- Daily Notes: %s/memory/YYYYMM/YYYYMMDD.md
- Skills: %s/skills/{skill-name}/SKILL.md

## Important Rules

1. **ALWAYS use tools** - When you need to perform an action (schedule reminders, send messages, execute commands, etc.), you MUST call the appropriate tool. Do NOT just say you'll do it or pretend to do it.

2. **Be helpful and accurate** - When using tools, briefly explain what you're doing.

3. **Memory** - When interacting with me if something seems memorable, update %s/memory/MEMORY.md

4. **Context summaries** - Conversation summaries provided as context are approximate references only. They may be incomplete or outdated. Always defer to explicit user instructions over summary content.

%s`,
		version, pluginsPath, pluginsPath, pluginsPath, pluginsPath, pluginsPath, pluginsPath, toolDiscovery)
}

// getDiscoveryRule 返回工具发现规则的提示词片段。
func (cb *ContextBuilder) getDiscoveryRule() string {
	if !cb.toolDiscoveryBM25 && !cb.toolDiscoveryRegex {
		return ""
	}

	var toolNames []string
	if cb.toolDiscoveryBM25 {
		toolNames = append(toolNames, `"tool_search_tool_bm25"`)
	}
	if cb.toolDiscoveryRegex {
		toolNames = append(toolNames, `"tool_search_tool_regex"`)
	}

	return fmt.Sprintf(
		`5. **Tool Discovery** - Your visible tools are limited to save memory, but a vast hidden library exists. If you lack the right tool for a task, BEFORE giving up, you MUST search using the %s tool. Do not refuse a request unless the search returns nothing. Found tools will temporarily unlock for your next turn.`,
		strings.Join(toolNames, " or "),
	)
}

// BuildSystemPrompt 构建完整的系统提示词，包含身份、引导文件、技能和记忆上下文。
func (cb *ContextBuilder) BuildSystemPrompt() string {
	parts := []string{}

	// 核心身份部分
	parts = append(parts, cb.getIdentity())

	// 引导文件
	bootstrapContent := cb.LoadBootstrapFiles()
	if bootstrapContent != "" {
		parts = append(parts, bootstrapContent)
	}

	// 技能 — 显示摘要，AI 可通过 read_file 工具读取完整内容
	if section := cb.buildSkillsSection(); section != "" {
		parts = append(parts, section)
	}

	// 记忆上下文
	memoryContext := cb.memory.GetMemoryContext()
	if memoryContext != "" {
		parts = append(parts, "# Memory\n\n"+memoryContext)
	}

	// 使用 "---" 分隔符连接各部分
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSystemPromptWithCache 如果缓存可用且源文件未变更则返回缓存的系统提示词，
// 否则重新构建并缓存。源文件变更通过 mtime 检查（低成本 stat 调用）来检测。
func (cb *ContextBuilder) BuildSystemPromptWithCache() string {
	// 先尝试读锁 — 缓存有效时的快速路径
	cb.systemPromptMutex.RLock()
	if cb.cachedSystemPrompt != "" && !cb.sourceFilesChangedLocked() {
		result := cb.cachedSystemPrompt
		cb.systemPromptMutex.RUnlock()
		return result
	}
	cb.systemPromptMutex.RUnlock()

	// 获取写锁进行构建
	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	// 二次检查：等待期间可能已有其他 goroutine 重新构建
	if cb.cachedSystemPrompt != "" && !cb.sourceFilesChangedLocked() {
		return cb.cachedSystemPrompt
	}

	// 在构建提示词之前快照基线（存在性 + 最大 mtime）。
	// 这样 cachedAt 反映的是构建前的状态：如果在 BuildSystemPrompt 执行期间
	// 某个文件被修改，其新 mtime 将大于 baseline.maxMtime，
	// 因此下次 sourceFilesChangedLocked 检查会正确触发重建。
	// 另一种方式（构建后快照）有缓存过时内容的风险，
	// 因为过新的基线会使过时内容不可见。
	baseline := cb.buildCacheBaseline()
	prompt := cb.BuildSystemPrompt()
	cb.cachedSystemPrompt = prompt
	cb.cachedAt = baseline.maxMtime
	cb.existedAtCache = baseline.existed
	cb.skillFilesAtCache = baseline.skillFiles

	logger.DebugCF("agent", "System prompt cached",
		map[string]any{
			"length": len(prompt),
		})

	return prompt
}

// InvalidateCache 清除缓存的系统提示词。
// 通常不需要，因为缓存通过 mtime 检查自动失效，
// 但在测试或显式重新加载命令时很有用。
func (cb *ContextBuilder) InvalidateCache() {
	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	cb.cachedSystemPrompt = ""
	cb.cachedAt = time.Time{}
	cb.existedAtCache = nil
	cb.skillFilesAtCache = nil

	logger.DebugCF("agent", "System prompt cache invalidated", nil)
}

// sourcePaths 返回用于缓存失效跟踪的非技能源文件路径
// （人设文件 + 记忆）。技能根目录单独处理，
// 因为它们需要目录级和递归文件级的双重检查。
func (cb *ContextBuilder) sourcePaths() []string {
	return []string{
		filepath.Join(cb.pluginsDir, "persona", "AGENTS.md"),
		filepath.Join(cb.pluginsDir, "persona", "SOUL.md"),
		filepath.Join(cb.pluginsDir, "persona", "USER.md"),
		filepath.Join(cb.pluginsDir, "persona", "IDENTITY.md"),
		filepath.Join(cb.pluginsDir, "memory", "MEMORY.md"),
	}
}

// cacheBaseline 保存文件存在性快照和所有跟踪路径中观察到的最新 mtime。
// 用作缓存的参考点。
type cacheBaseline struct {
	existed    map[string]bool
	skillFiles map[string]time.Time
	maxMtime   time.Time
}

// buildCacheBaseline 记录当前跟踪路径的存在状态，并计算所有跟踪文件
// 和技能目录内容中的最新 mtime。在构建缓存时于写锁下调用。
func (cb *ContextBuilder) buildCacheBaseline() cacheBaseline {
	skillRoots := cb.skillRoots()

	// 我们跟踪存在性的所有路径：源文件 + 所有技能根目录。
	allPaths := append(cb.sourcePaths(), skillRoots...)

	existed := make(map[string]bool, len(allPaths))
	skillFiles := make(map[string]time.Time)
	var maxMtime time.Time

	for _, p := range allPaths {
		info, err := os.Stat(p)
		existed[p] = err == nil
		if err == nil && info.ModTime().After(maxMtime) {
			maxMtime = info.ModTime()
		}
	}

	// 递归遍历所有技能根目录以快照技能文件及其 mtime。
	// 使用 os.Stat（而非 d.Info）以与 sourceFilesChanged 检查保持一致。
	for _, root := range skillRoots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr == nil && !d.IsDir() {
				if info, err := os.Stat(path); err == nil {
					skillFiles[path] = info.ModTime()
					if info.ModTime().After(maxMtime) {
						maxMtime = info.ModTime()
					}
				}
			}
			return nil
		})
	}

	// 如果尚无跟踪文件存在（空的插件目录），maxMtime 为零。
	// 使用一个很早的非零时间，以便：
	// 1. cachedAt.IsZero() 不会触发持续重建。
	// 2. 之后创建的任何真实文件的 mtime > cachedAt，
	//    因此会被 fileChangedSince 检测到（不像 time.Now()
	//    可能与 mtime <= Now 的文件产生竞态）。
	if maxMtime.IsZero() {
		maxMtime = time.Unix(1, 0)
	}

	return cacheBaseline{existed: existed, skillFiles: skillFiles, maxMtime: maxMtime}
}

// sourceFilesChangedLocked 检查自上次构建缓存以来，
// 是否有任何插件目录源文件被修改、创建或删除。
//
// 重要：调用者必须至少持有 systemPromptMutex 的读锁。
// Go 的 sync.RWMutex 不支持重入，因此此函数不能自行获取锁
// （当从已持有 RLock 或 Lock 的 BuildSystemPromptWithCache 调用时会死锁）。
func (cb *ContextBuilder) sourceFilesChangedLocked() bool {
	if cb.cachedAt.IsZero() {
		return true
	}

	// 检查跟踪的源文件（引导文件 + 记忆）。
	if slices.ContainsFunc(cb.sourcePaths(), cb.fileChangedSince) {
		return true
	}

	// --- 技能根目录（工作区/全局/内置）---
	//
	// 对于每个根目录：
	// 1. 创建/删除和根目录 mtime 变更通过 fileChangedSince 跟踪。
	// 2. 嵌套文件的创建/删除/mtime 变更通过技能文件快照跟踪。
	for _, root := range cb.skillRoots() {
		if cb.fileChangedSince(root) {
			return true
		}
	}
	if skillFilesChangedSince(cb.skillRoots(), cb.skillFilesAtCache) {
		return true
	}

	return false
}

// fileChangedSince 判断跟踪的源文件自缓存构建以来是否被修改、新建或删除。
//
// 四种情况：
//   - 缓存时存在，现在存在 -> 检查 mtime
//   - 缓存时存在，现在消失 -> 已变更（已删除）
//   - 缓存时不存在，现在存在 -> 已变更（新建）
//   - 缓存时不存在，现在不存在 -> 未变更
func (cb *ContextBuilder) fileChangedSince(path string) bool {
	// 防御性检查：如果 existedAtCache 从未初始化，视为已变更
	// 以便缓存重建，而不是静默提供过时数据。
	if cb.existedAtCache == nil {
		return true
	}

	existedBefore := cb.existedAtCache[path]
	info, err := os.Stat(path)
	existsNow := err == nil

	if existedBefore != existsNow {
		return true // 文件被创建或删除
	}
	if !existsNow {
		return false // 之前不存在，现在也不存在
	}
	return info.ModTime().After(cb.cachedAt)
}

// LoadBootstrapFiles 加载人设引导文件（AGENTS.md、SOUL.md、USER.md、IDENTITY.md）。
func (cb *ContextBuilder) LoadBootstrapFiles() string {
	bootstrapFiles := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"IDENTITY.md",
	}

	var sb strings.Builder
	for _, filename := range bootstrapFiles {
		filePath := filepath.Join(cb.pluginsDir, "persona", filename)
		if data, err := os.ReadFile(filePath); err == nil {
			fmt.Fprintf(&sb, "## %s\n\n%s\n\n", filename, data)
		}
	}

	return sb.String()
}

// buildDynamicContext 返回包含每次请求信息的简短动态上下文字符串。
// 此内容每次请求都会变化（时间、会话），因此不属于缓存提示词的一部分。
// LLM 端的 KV 缓存复用通过各提供商适配器的原生机制实现：
//   - Anthropic：静态 SystemParts 块上的按块 cache_control（ephemeral）
//   - OpenAI / Codex：基于前缀的 prompt_cache_key 缓存
//
// 参见：https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
// 参见：https://platform.openai.com/docs/guides/prompt-caching
func (cb *ContextBuilder) buildDynamicContext(channel, chatID string) string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	rt := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Current Time\n%s\n\n## Runtime\n%s", now, rt)

	if channel != "" && chatID != "" {
		fmt.Fprintf(&sb, "\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	return sb.String()
}

// BuildMessages 根据历史记录、摘要、当前消息和媒体内容构建完整的消息列表。
func (cb *ContextBuilder) BuildMessages(
	history []providers.Message,
	summary string,
	currentMessage string,
	media []string,
	channel, chatID string,
) []providers.Message {
	messages := []providers.Message{}

	// 静态部分（身份、引导文件、技能、记忆）在本地缓存，
	// 避免每次调用时重复的文件 I/O 和字符串构建（修复 issue #607）。
	// 动态部分（时间、会话、摘要）在每次请求时追加。
	// 所有内容作为单个系统消息发送以兼容各提供商：
	// - Anthropic 适配器提取 messages[0]（Role=="system"）并将其内容
	//   映射到 Messages API 请求中的顶层 "system" 参数。单一连续的
	//   系统块使这种提取简单直接。
	// - Codex 仅将第一条系统消息映射到其 instructions 字段。
	// - OpenAI 兼容模式直接传递消息。
	staticPrompt := cb.BuildSystemPromptWithCache()

	// 构建简短的动态上下文（时间、运行时、会话）— 每次请求都会变化
	dynamicCtx := cb.buildDynamicContext(channel, chatID)

	// 组合单条系统消息：静态（缓存的）+ 动态 + 可选摘要。
	// 将所有系统内容放在一条消息中确保每个提供商适配器都能正确提取
	// （Anthropic 适配器 -> 顶层 system 参数，Codex -> instructions 字段）。
	//
	// SystemParts 将相同内容以结构化块的形式承载，以便
	// 支持缓存的适配器（Anthropic）可以设置按块的 cache_control。
	// 静态块标记为 "ephemeral" — 其前缀哈希在请求间保持稳定，
	// 从而实现 LLM 端的 KV 缓存复用。
	stringParts := []string{staticPrompt, dynamicCtx}

	contentBlocks := []providers.ContentBlock{
		{Type: "text", Text: staticPrompt, CacheControl: &providers.CacheControl{Type: "ephemeral"}},
		{Type: "text", Text: dynamicCtx},
	}

	if summary != "" {
		summaryText := fmt.Sprintf(
			"CONTEXT_SUMMARY: The following is an approximate summary of prior conversation "+
				"for reference only. It may be incomplete or outdated — always defer to explicit instructions.\n\n%s",
			summary)
		stringParts = append(stringParts, summaryText)
		contentBlocks = append(contentBlocks, providers.ContentBlock{Type: "text", Text: summaryText})
	}

	fullSystemPrompt := strings.Join(stringParts, "\n\n---\n\n")

	// 记录系统提示词摘要用于调试（仅调试模式）。
	// 在锁下读取 cachedSystemPrompt，避免与并发的
	// InvalidateCache / BuildSystemPromptWithCache 写入产生数据竞态。
	cb.systemPromptMutex.RLock()
	isCached := cb.cachedSystemPrompt != ""
	cb.systemPromptMutex.RUnlock()

	logger.DebugCF("agent", "System prompt built",
		map[string]any{
			"static_chars":  len(staticPrompt),
			"dynamic_chars": len(dynamicCtx),
			"total_chars":   len(fullSystemPrompt),
			"has_summary":   summary != "",
			"cached":        isCached,
		})

	// 记录系统提示词预览（避免记录大量内容）
	preview := utils.Truncate(fullSystemPrompt, 500)
	logger.DebugCF("agent", "System prompt preview",
		map[string]any{
			"preview": preview,
		})

	history = sanitizeHistoryForProvider(history)

	// 包含所有上下文的单条系统消息 — 兼容所有提供商。
	// SystemParts 使支持缓存的适配器能设置按块的 cache_control；
	// Content 是供不读取 SystemParts 的适配器使用的拼接回退值。
	messages = append(messages, providers.Message{
		Role:        "system",
		Content:     fullSystemPrompt,
		SystemParts: contentBlocks,
	})

	// 添加对话历史
	messages = append(messages, history...)

	// 添加当前用户消息
	if strings.TrimSpace(currentMessage) != "" {
		msg := providers.Message{
			Role:    "user",
			Content: currentMessage,
		}
		if len(media) > 0 {
			msg.Media = media
		}
		messages = append(messages, msg)
	}

	return messages
}

// sanitizeHistoryForProvider 清理历史消息以确保符合提供商的格式要求。
func sanitizeHistoryForProvider(history []providers.Message) []providers.Message {
	if len(history) == 0 {
		return history
	}

	sanitized := make([]providers.Message, 0, len(history))
	for _, msg := range history {
		switch msg.Role {
		case "system":
			// 从历史中删除系统消息。BuildMessages 始终构建自己的
			// 单条系统消息（静态 + 动态 + 摘要）；额外的系统消息
			// 会破坏仅接受一条系统消息的提供商（Anthropic、Codex）。
			logger.DebugCF("agent", "Dropping system message from history", map[string]any{})
			continue

		case "tool":
			if len(sanitized) == 0 {
				logger.DebugCF("agent", "Dropping orphaned leading tool message", map[string]any{})
				continue
			}
			// 向后查找最近的助手消息，
			// 跳过前面的工具消息（多工具调用场景）。
			foundAssistant := false
			for i := len(sanitized) - 1; i >= 0; i-- {
				if sanitized[i].Role == "tool" {
					continue
				}
				if sanitized[i].Role == "assistant" && len(sanitized[i].ToolCalls) > 0 {
					foundAssistant = true
				}
				break
			}
			if !foundAssistant {
				logger.DebugCF("agent", "Dropping orphaned tool message", map[string]any{})
				continue
			}
			sanitized = append(sanitized, msg)

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				if len(sanitized) == 0 {
					logger.DebugCF("agent", "Dropping assistant tool-call turn at history start", map[string]any{})
					continue
				}
				prev := sanitized[len(sanitized)-1]
				if prev.Role != "user" && prev.Role != "tool" {
					logger.DebugCF(
						"agent",
						"Dropping assistant tool-call turn with invalid predecessor",
						map[string]any{"prev_role": prev.Role},
					)
					continue
				}
			}
			sanitized = append(sanitized, msg)

		default:
			sanitized = append(sanitized, msg)
		}
	}

	// 第二遍：确保每条包含 tool_calls 的助手消息都有对应的
	// 工具结果消息跟随。这是严格提供商（如 DeepSeek）所要求的：
	// "包含 'tool_calls' 的助手消息后必须跟随对每个 'tool_call_id' 响应的工具消息。"
	final := make([]providers.Message, 0, len(sanitized))
	for i := 0; i < len(sanitized); i++ {
		msg := sanitized[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// 收集预期的 tool_call ID
			expected := make(map[string]bool, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				expected[tc.ID] = false
			}

			// 检查后续消息是否有匹配的工具结果
			toolMsgCount := 0
			for j := i + 1; j < len(sanitized); j++ {
				if sanitized[j].Role != "tool" {
					break
				}
				toolMsgCount++
				if _, exists := expected[sanitized[j].ToolCallID]; exists {
					expected[sanitized[j].ToolCallID] = true
				}
			}

			// 如果有任何 tool_call_id 缺失，丢弃此助手消息及其部分工具消息
			allFound := true
			for toolCallID, found := range expected {
				if !found {
					allFound = false
					logger.DebugCF(
						"agent",
						"Dropping assistant message with incomplete tool results",
						map[string]any{
							"missing_tool_call_id": toolCallID,
							"expected_count":       len(expected),
							"found_count":          toolMsgCount,
						},
					)
					break
				}
			}

			if !allFound {
				// 跳过此助手消息及其工具消息
				i += toolMsgCount
				continue
			}
		}
		final = append(final, msg)
	}

	return final
}

// AddToolResult 向消息列表中追加工具执行结果消息。
func (cb *ContextBuilder) AddToolResult(
	messages []providers.Message,
	toolCallID, toolName, result string,
) []providers.Message {
	messages = append(messages, providers.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
	return messages
}

// AddAssistantMessage 向消息列表中追加助手消息。
func (cb *ContextBuilder) AddAssistantMessage(
	messages []providers.Message,
	content string,
	toolCalls []map[string]any,
) []providers.Message {
	msg := providers.Message{
		Role:    "assistant",
		Content: content,
	}
	// 无论是否有工具调用，始终添加助手消息
	messages = append(messages, msg)
	return messages
}
