// GeekClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/commands"
	cmdext "github.com/seagosoft/geekclaw/geekclaw/commands/external"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	searchext "github.com/seagosoft/geekclaw/geekclaw/tools/external"
	"github.com/seagosoft/geekclaw/geekclaw/interactive"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/mcp"
	"github.com/seagosoft/geekclaw/geekclaw/media"
	"github.com/seagosoft/geekclaw/geekclaw/providers"
	"github.com/seagosoft/geekclaw/geekclaw/routing"
	"github.com/seagosoft/geekclaw/geekclaw/fileutil"
	"github.com/seagosoft/geekclaw/geekclaw/tasks"
	"github.com/seagosoft/geekclaw/geekclaw/tools"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
	"github.com/seagosoft/geekclaw/geekclaw/voice"
)

// AgentLoop 是代理的主循环，负责消息处理、路由和工具执行。
type AgentLoop struct {
	bus             *bus.MessageBus
	cfg             *config.Config
	registry        *AgentRegistry
	stateFile       string
	running         atomic.Bool
	summarizing     TypedMap[string, bool]
	fallback        *providers.FallbackChain
	channelManager  *channels.Manager
	mediaStore      media.MediaStore
	transcriber     voice.Transcriber
	cmdRegistry     *commands.Registry
	cmdPlugins      []*cmdext.ExternalCommandPlugin     // 外部命令插件
	searchPlugins   []*searchext.ExternalSearchProvider // 外部搜索插件
	toolPlugins     []*searchext.ExternalToolPlugin     // 外部工具插件
	sessionModes    TypedMap[string, sessionMode]
	sessionWorkDirs TypedMap[string, string]
	taskQueue       *tasks.Queue        // 管理活跃 AI 任务，支持取消操作
	interactiveMgr  *interactive.Manager // 管理交互式确认
}

// processOptions 配置消息处理方式。
type processOptions struct {
	SessionKey      string         // 历史/上下文的会话标识符
	Channel         string         // 工具执行的目标频道
	ChatID          string         // 工具执行的目标聊天 ID
	UserMessage     string         // 用户消息内容（可能包含前缀）
	Media           []string       // 入站消息中的 media:// 引用
	DefaultResponse string         // LLM 返回空内容时的默认响应
	EnableSummary   bool           // 是否触发摘要
	SendResponse    bool           // 是否通过总线发送响应
	NoHistory       bool           // 如果为 true，不加载会话历史（用于心跳）
	WorkingDir      string         // 当前工作目录覆盖（用于 cmd 模式下的 hipico）
	Sender          bus.SenderInfo // 发送者身份，用于按用户权限检查
}

const (
	defaultResponse           = "I've completed processing but have no response to give. Increase `max_tool_iterations` in config.yaml."
	sessionKeyAgentPrefix     = "agent:"
	metadataKeyAccountID      = "account_id"
	metadataKeyGuildID        = "guild_id"
	metadataKeyTeamID         = "team_id"
	metadataKeyParentPeerKind = "parent_peer_kind"
	metadataKeyParentPeerID   = "parent_peer_id"
)

// NewAgentLoop 创建并初始化代理主循环。
func NewAgentLoop(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	provider providers.LLMProvider,
) *AgentLoop {
	registry := NewAgentRegistry(cfg, provider)

	// 并行启动外部插件（搜索、工具），减少启动时间
	var (
		searchPlugins       []*searchext.ExternalSearchProvider
		activeSearchPlugin  *searchext.ExternalSearchProvider
		toolPlugins         []*searchext.ExternalToolPlugin
		pluginMu            sync.Mutex
		pluginWg            sync.WaitGroup
	)

	// 并行启动搜索插件
	for name, pluginCfg := range cfg.Tools.Web.Plugins {
		if !pluginCfg.Enabled {
			continue
		}
		pluginWg.Add(1)
		go func(name string, pluginCfg config.WebSearchPluginConfig) {
			defer pluginWg.Done()
			plugin := searchext.NewExternalSearchProvider(name, searchext.PluginConfig{
				Enabled: pluginCfg.Enabled,
				Command: pluginCfg.Command,
				Args:    pluginCfg.Args,
				Env:     pluginCfg.Env,
				Config:  pluginCfg.Config,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := plugin.Start(ctx); err != nil {
				logger.WarnCF("search", "Failed to start search plugin", map[string]any{
					"plugin": name,
					"error":  err.Error(),
				})
				return
			}
			pluginMu.Lock()
			searchPlugins = append(searchPlugins, plugin)
			if activeSearchPlugin == nil {
				activeSearchPlugin = plugin
				logger.InfoCF("search", "Using external search plugin", map[string]any{
					"plugin":   name,
					"provider": plugin.Name(),
				})
			}
			pluginMu.Unlock()
		}(name, pluginCfg)
	}

	// 并行启动工具插件
	for name, pluginCfg := range cfg.Tools.Plugins {
		if !pluginCfg.Enabled {
			continue
		}
		pluginWg.Add(1)
		go func(name string, pluginCfg config.ToolPluginConfig) {
			defer pluginWg.Done()
			plugin := searchext.NewExternalToolPlugin(name, searchext.PluginConfig{
				Enabled: pluginCfg.Enabled,
				Command: pluginCfg.Command,
				Args:    pluginCfg.Args,
				Env:     pluginCfg.Env,
				Config:  pluginCfg.Config,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := plugin.Start(ctx); err != nil {
				logger.WarnCF("tools", "Failed to start tool plugin", map[string]any{
					"plugin": name,
					"error":  err.Error(),
				})
				return
			}
			pluginMu.Lock()
			toolPlugins = append(toolPlugins, plugin)
			pluginMu.Unlock()
		}(name, pluginCfg)
	}

	// 等待所有插件启动完成
	pluginWg.Wait()

	// 查找活跃搜索插件的 max_results
	var searchPluginMaxResults int
	if activeSearchPlugin != nil {
		for _, pluginCfg := range cfg.Tools.Web.Plugins {
			if pluginCfg.Enabled && pluginCfg.MaxResults > 0 {
				searchPluginMaxResults = pluginCfg.MaxResults
				break
			}
		}
	}

	// 向所有代理注册共享工具
	registerSharedTools(cfg, msgBus, registry, provider, activeSearchPlugin, searchPluginMaxResults, toolPlugins)

	// 设置共享回退链
	cooldown := providers.NewCooldownTracker()
	fallbackChain := providers.NewFallbackChain(cooldown)

	// 状态文件用于记录最近活跃的频道/聊天
	stateFile := filepath.Join(cfg.LogsPath(), "state.json")

	cmdReg := commands.NewRegistry(commands.BuiltinDefinitions())

	// 并行启动外部命令插件
	var cmdPlugins []*cmdext.ExternalCommandPlugin
	var cmdPluginsMu sync.Mutex
	var cmdWg sync.WaitGroup
	type cmdPluginResult struct {
		plugin *cmdext.ExternalCommandPlugin
		name   string
	}
	var cmdResults []cmdPluginResult

	for name, pluginCfg := range cfg.Commands.Plugins {
		if !pluginCfg.Enabled {
			continue
		}
		cmdWg.Add(1)
		go func(name string, pluginCfg config.CommandPluginConfig) {
			defer cmdWg.Done()
			plugin := cmdext.NewExternalCommandPlugin(name, cmdext.PluginConfig{
				Enabled: pluginCfg.Enabled,
				Command: pluginCfg.Command,
				Args:    pluginCfg.Args,
				Env:     pluginCfg.Env,
				Config:  pluginCfg.Config,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := plugin.Start(ctx); err != nil {
				logger.WarnCF("commands", "Failed to start command plugin", map[string]any{
					"plugin": name,
					"error":  err.Error(),
				})
				return
			}
			cmdPluginsMu.Lock()
			cmdResults = append(cmdResults, cmdPluginResult{plugin: plugin, name: name})
			cmdPluginsMu.Unlock()
		}(name, pluginCfg)
	}
	cmdWg.Wait()

	// 按顺序注册命令（避免并发写 cmdReg）
	for _, r := range cmdResults {
		plugin := r.plugin

		if conflicts := cmdReg.MergeDefinitions(plugin.Commands()); len(conflicts) > 0 {
			for _, c := range conflicts {
				logger.WarnCF("commands", "Plugin command conflict", map[string]any{
					"plugin": r.name,
					"error":  c.Error(),
				})
			}
		}
		cmdPlugins = append(cmdPlugins, plugin)
	}

	al := &AgentLoop{
		bus:            msgBus,
		cfg:            cfg,
		registry:       registry,
		stateFile:      stateFile,
		fallback:       fallbackChain,
		cmdRegistry:    cmdReg,
		cmdPlugins:     cmdPlugins,
		searchPlugins:  searchPlugins,
		toolPlugins:    toolPlugins,
		taskQueue:      tasks.NewQueue(),
		interactiveMgr: interactive.NewManager(),
	}

	return al
}

// registerSharedTools 注册所有代理共享的工具（web、message、spawn）。
func registerSharedTools(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	registry *AgentRegistry,
	provider providers.LLMProvider,
	externalSearch tools.SearchProvider,
	externalSearchMaxResults int,
	toolPlugins []*searchext.ExternalToolPlugin,
) {
	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		if cfg.Tools.IsToolEnabled("web") {
			searchTool, err := tools.NewWebSearchTool(tools.WebSearchToolOptions{
				Proxy:              cfg.Tools.Web.Proxy,
				ExternalProvider:   externalSearch,
				ExternalMaxResults: externalSearchMaxResults,
			})
			if err != nil {
				logger.ErrorCF("agent", "Failed to create web search tool", map[string]any{"error": err.Error()})
			} else if searchTool != nil {
				agent.Tools.Register(searchTool)
			}
		}
		// web_fetch 现在通过外部工具插件提供（plugins/tools/contrib/web_fetch.py）

		// 外部工具插件 — 注册每个插件声明的所有工具
		for _, tp := range toolPlugins {
			for _, t := range tp.Tools() {
				agent.Tools.Register(t)
			}
		}

		// message 工具现在通过外部工具插件提供（plugins/tools/contrib/message.py）
		// 插件通过 host.bus.publish_outbound 反向 RPC 发送消息

		// 文件发送工具（通过 MediaStore 出站媒体 — 存储由 SetMediaStore 后续注入）
		if cfg.Tools.IsToolEnabled("send_file") {
			sendFileTool := tools.NewSendFileTool(
				agent.Workspace,
				cfg.Agents.Defaults.RestrictToWorkspace,
				cfg.Agents.Defaults.GetMaxMediaSize(),
				nil,
			)
			agent.Tools.Register(sendFileTool)
		}

		// skills 工具现在通过外部工具插件提供（plugins/tools/contrib/skills.py）
		// 插件通过 host.skills.search / host.skills.install 反向 RPC 操作注册中心

		// 生成工具，带有白名单检查器
		if cfg.Tools.IsToolEnabled("spawn") {
			if cfg.Tools.IsToolEnabled("subagent") {
				subagentManager := tools.NewSubagentManager(provider, agent.Model, agent.Workspace)
				subagentManager.SetLLMOptions(agent.MaxTokens, agent.Temperature)
				spawnTool := tools.NewSpawnTool(subagentManager)
				currentAgentID := agentID
				spawnTool.SetAllowlistChecker(func(targetAgentID string) bool {
					return registry.CanSpawnSubagent(currentAgentID, targetAgentID)
				})
				agent.Tools.Register(spawnTool)
			} else {
				logger.WarnCF("agent", "spawn tool requires subagent to be enabled", nil)
			}
		}
	}
}

// Run 启动代理主循环，处理入站消息。
func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	// 为所有代理初始化 MCP 服务器
	if al.cfg.Tools.IsToolEnabled("mcp") {
		mcpManager := mcp.NewManager()
		// 确保退出时清理 MCP 连接，无论初始化是否成功
		// 修复部分成功后失败时的资源泄漏问题
		defer func() {
			if err := mcpManager.Close(); err != nil {
				logger.ErrorCF("agent", "Failed to close MCP manager",
					map[string]any{
						"error": err.Error(),
					})
			}
		}()

		defaultAgent := al.registry.GetDefaultAgent()
		var workspacePath string
		if defaultAgent != nil && defaultAgent.Workspace != "" {
			workspacePath = defaultAgent.Workspace
		} else {
			workspacePath = al.cfg.WorkspacePath()
		}

		if err := mcpManager.LoadFromMCPConfig(ctx, al.cfg.Tools.MCP, workspacePath); err != nil {
			logger.WarnCF("agent", "Failed to load MCP servers, MCP tools will not be available",
				map[string]any{
					"error": err.Error(),
				})
		} else {
			// 为所有代理注册 MCP 工具
			servers := mcpManager.GetServers()
			uniqueTools := 0
			totalRegistrations := 0
			agentIDs := al.registry.ListAgentIDs()
			agentCount := len(agentIDs)

			for serverName, conn := range servers {
				uniqueTools += len(conn.Tools)
				for _, tool := range conn.Tools {
					for _, agentID := range agentIDs {
						agent, ok := al.registry.GetAgent(agentID)
						if !ok {
							continue
						}

						mcpTool := tools.NewMCPTool(mcpManager, serverName, tool)

						if al.cfg.Tools.MCP.Discovery.Enabled {
							agent.Tools.RegisterHidden(mcpTool)
						} else {
							agent.Tools.Register(mcpTool)
						}

						totalRegistrations++
						logger.DebugCF("agent", "Registered MCP tool",
							map[string]any{
								"agent_id": agentID,
								"server":   serverName,
								"tool":     tool.Name,
								"name":     mcpTool.Name(),
							})
					}
				}
			}
			logger.InfoCF("agent", "MCP tools registered successfully",
				map[string]any{
					"server_count":        len(servers),
					"unique_tools":        uniqueTools,
					"total_registrations": totalRegistrations,
					"agent_count":         agentCount,
				})

			// 仅在配置启用时初始化工具发现
			if al.cfg.Tools.MCP.Enabled && al.cfg.Tools.MCP.Discovery.Enabled {
				useBM25 := al.cfg.Tools.MCP.Discovery.UseBM25
				useRegex := al.cfg.Tools.MCP.Discovery.UseRegex

				// 快速失败：如果启用了发现但没有搜索方法被开启
				if !useBM25 && !useRegex {
					return fmt.Errorf(
						"tool discovery is enabled but neither 'use_bm25' nor 'use_regex' is set to true in the configuration",
					)
				}

				ttl := al.cfg.Tools.MCP.Discovery.TTL
				if ttl <= 0 {
					ttl = 5 // 默认值
				}

				maxSearchResults := al.cfg.Tools.MCP.Discovery.MaxSearchResults
				if maxSearchResults <= 0 {
					maxSearchResults = 5 // 默认值
				}

				logger.InfoCF("agent", "Initializing tool discovery", map[string]any{
					"bm25": useBM25, "regex": useRegex, "ttl": ttl, "max_results": maxSearchResults,
				})

				for _, agentID := range agentIDs {
					agent, ok := al.registry.GetAgent(agentID)
					if !ok {
						continue
					}

					if useRegex {
						agent.Tools.Register(tools.NewRegexSearchTool(agent.Tools, ttl, maxSearchResults))
					}
					if useBM25 {
						agent.Tools.Register(tools.NewBM25SearchTool(agent.Tools, ttl, maxSearchResults))
					}
				}
			}
		}
	}

	// 使用按会话分发的并发处理：不同会话并行，同一会话串行。
	type sessionWorker struct {
		ch   chan bus.InboundMessage
		done chan struct{}
	}
	var (
		workersMu sync.Mutex
		workers   = make(map[string]*sessionWorker)
	)

	processMsg := func(msg bus.InboundMessage) {
		defer func() {
			if al.mediaStore != nil && msg.MediaScope != "" {
				if releaseErr := al.mediaStore.ReleaseAll(msg.MediaScope); releaseErr != nil {
					logger.WarnCF("agent", "Failed to release media", map[string]any{
						"scope": msg.MediaScope,
						"error": releaseErr.Error(),
					})
				}
			}
		}()

		response, err := al.processMessage(ctx, msg)
		if err != nil {
			response = fmt.Sprintf("Error processing message: %v", err)
		}

		if response != "" {
			al.bus.PublishOutbound(ctx, bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				Content: response,
			})
			logger.InfoCF("agent", "Published outbound response",
				map[string]any{
					"channel":     msg.Channel,
					"chat_id":     msg.ChatID,
					"content_len": len(response),
				})
		}
	}

	// 获取消息的会话键：优先使用 SessionKey，否则用 channel:chatID
	sessionKeyFor := func(msg bus.InboundMessage) string {
		if msg.SessionKey != "" {
			return msg.SessionKey
		}
		return msg.Channel + ":" + msg.ChatID
	}

	for al.running.Load() {
		select {
		case <-ctx.Done():
			// 等待所有 worker 完成
			workersMu.Lock()
			for _, w := range workers {
				close(w.ch)
			}
			for _, w := range workers {
				<-w.done
			}
			workersMu.Unlock()
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			key := sessionKeyFor(msg)

			workersMu.Lock()
			w, exists := workers[key]
			if !exists {
				w = &sessionWorker{
					ch:   make(chan bus.InboundMessage, 16),
					done: make(chan struct{}),
				}
				workers[key] = w
				go func(key string, w *sessionWorker) {
					defer close(w.done)
					for m := range w.ch {
						processMsg(m)
					}
					// worker 空闲后自行清理
					workersMu.Lock()
					delete(workers, key)
					workersMu.Unlock()
				}(key, w)
			}
			workersMu.Unlock()

			// 非阻塞发送，如果 worker 队列满则直接处理
			select {
			case w.ch <- msg:
			default:
				go processMsg(msg)
			}
		}
	}

	// 停止时清理所有 worker
	workersMu.Lock()
	for _, w := range workers {
		close(w.ch)
	}
	for _, w := range workers {
		<-w.done
	}
	workersMu.Unlock()

	return nil
}

// Stop 停止代理主循环。
func (al *AgentLoop) Stop() {
	al.running.Store(false)
}

// Close 释放代理会话存储持有的资源并停止插件。在 Stop 之后调用。
func (al *AgentLoop) Close() {
	for _, p := range al.cmdPlugins {
		p.Stop()
	}
	for _, p := range al.searchPlugins {
		p.Stop()
	}
	for _, p := range al.toolPlugins {
		p.Stop()
	}
	al.registry.Close()
}

// RegisterTool 向所有代理注册工具。
func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	for _, agentID := range al.registry.ListAgentIDs() {
		if agent, ok := al.registry.GetAgent(agentID); ok {
			agent.Tools.Register(tool)
		}
	}
}

// SetChannelManager 设置频道管理器。
func (al *AgentLoop) SetChannelManager(cm *channels.Manager) {
	al.channelManager = cm
}

// SetMediaStore 注入 MediaStore 用于媒体生命周期管理。
func (al *AgentLoop) SetMediaStore(s media.MediaStore) {
	al.mediaStore = s

	// 将存储传播到所有代理的 send_file 工具。
	al.registry.ForEachTool("send_file", func(t tools.Tool) {
		if sf, ok := t.(*tools.SendFileTool); ok {
			sf.SetMediaStore(s)
		}
	})
}

// SetTranscriber 注入语音转录器，用于代理级音频转录。
func (al *AgentLoop) SetTranscriber(t voice.Transcriber) {
	al.transcriber = t
}

var audioAnnotationRe = regexp.MustCompile(`\[(voice|audio)(?::[^\]]*)?\]`)

// transcribeAudioInMessage 解析音频媒体引用，进行转录，
// 并将消息内容中的音频注解替换为转录文本。
// 返回（可能已修改的）消息和是否进行了音频转录的标志。
func (al *AgentLoop) transcribeAudioInMessage(ctx context.Context, msg bus.InboundMessage) (bus.InboundMessage, bool) {
	if al.transcriber == nil || al.mediaStore == nil || len(msg.Media) == 0 {
		return msg, false
	}

	// 按顺序转录每个音频媒体引用。
	var transcriptions []string
	for _, ref := range msg.Media {
		path, meta, err := al.mediaStore.ResolveWithMeta(ref)
		if err != nil {
			logger.WarnCF("voice", "Failed to resolve media ref", map[string]any{"ref": ref, "error": err})
			continue
		}
		if !utils.IsAudioFile(meta.Filename, meta.ContentType) {
			continue
		}
		result, err := al.transcriber.Transcribe(ctx, path)
		if err != nil {
			logger.WarnCF("voice", "Transcription failed", map[string]any{"ref": ref, "error": err})
			transcriptions = append(transcriptions, "")
			continue
		}
		transcriptions = append(transcriptions, result.Text)
	}

	if len(transcriptions) == 0 {
		return msg, false
	}

	al.sendTranscriptionFeedback(ctx, msg.Channel, msg.ChatID, msg.MessageID, transcriptions)

	// 按顺序将音频注解替换为转录内容。
	idx := 0
	newContent := audioAnnotationRe.ReplaceAllStringFunc(msg.Content, func(match string) string {
		if idx >= len(transcriptions) {
			return match
		}
		text := transcriptions[idx]
		idx++
		return "[voice: " + text + "]"
	})

	// 将未匹配到注解的剩余转录追加到内容末尾。
	for ; idx < len(transcriptions); idx++ {
		newContent += "\n[voice: " + transcriptions[idx] + "]"
	}

	msg.Content = newContent
	return msg, true
}

// sendTranscriptionFeedback 在启用选项时向用户发送音频转录结果反馈。
// 使用 Manager.SendMessage 同步执行（限流、分割、重试），
// 以保证与后续占位符的顺序。
func (al *AgentLoop) sendTranscriptionFeedback(
	ctx context.Context,
	channel, chatID, messageID string,
	validTexts []string,
) {
	if !al.cfg.Voice.EchoTranscription {
		return
	}
	if al.channelManager == nil {
		return
	}

	var nonEmpty []string
	for _, t := range validTexts {
		if t != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}

	var feedbackMsg string
	if len(nonEmpty) > 0 {
		feedbackMsg = "Transcript: " + strings.Join(nonEmpty, "\n")
	} else {
		feedbackMsg = "No voice detected in the audio"
	}

	err := al.channelManager.SendMessage(ctx, bus.OutboundMessage{
		Channel:          channel,
		ChatID:           chatID,
		Content:          feedbackMsg,
		ReplyToMessageID: messageID,
	})
	if err != nil {
		logger.WarnCF("voice", "Failed to send transcription feedback", map[string]any{"error": err.Error()})
	}
}

// RecordLastChannel 记录此插件目录的最后活跃频道。
// 使用原子状态保存机制，防止崩溃时数据丢失。
func (al *AgentLoop) RecordLastChannel(channel string) error {
	return al.saveState("last_channel", channel)
}

// RecordLastChatID 记录此插件目录的最后活跃聊天 ID。
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	return al.saveState("last_chat_id", chatID)
}

// saveState 原子地更新状态文件中的指定字段。
func (al *AgentLoop) saveState(key, value string) error {
	if al.stateFile == "" {
		return nil
	}
	data, _ := os.ReadFile(al.stateFile)
	var s map[string]any
	if json.Unmarshal(data, &s) != nil || s == nil {
		s = map[string]any{}
	}
	s[key] = value
	s["timestamp"] = time.Now()
	out, _ := json.MarshalIndent(s, "", "  ")
	return fileutil.WriteFileAtomic(al.stateFile, out, 0o600)
}

// ProcessDirect 直接处理消息，使用 CLI 频道。
func (al *AgentLoop) ProcessDirect(
	ctx context.Context,
	content, sessionKey string,
) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

// ProcessDirectWithWorkDir 使用显式工作目录上下文处理消息。
// 工作目录被注入系统提示词中，使 AI 相对于该目录解析文件路径。
func (al *AgentLoop) ProcessDirectWithWorkDir(
	ctx context.Context,
	content, sessionKey, workDir string,
) (string, error) {
	msg := bus.InboundMessage{
		Channel:    "cli",
		SenderID:   "cron",
		ChatID:     "direct",
		Content:    content,
		SessionKey: sessionKey,
		Metadata:   map[string]string{"work_dir": workDir},
	}
	return al.processMessage(ctx, msg)
}

// ProcessDirectWithChannel 使用指定频道和聊天 ID 直接处理消息。
func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// processMessage 处理入站消息，包括路由、命令处理和 LLM 调用。
func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// 向日志添加消息预览（错误消息显示完整内容）
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // 错误消息显示完整内容
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF(
		"agent",
		fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		},
	)

	var hadAudio bool
	msg, hadAudio = al.transcribeAudioInMessage(ctx, msg)

	// 对于音频消息，频道延迟了占位符。
	// 现在转录（和可选反馈）已完成，发送占位符。
	if hadAudio && al.channelManager != nil {
		al.channelManager.SendPlaceholder(ctx, msg.Channel, msg.ChatID)
	}

	// 将系统消息路由到 processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	route, agent, routeErr := al.resolveMessageRoute(msg)

	// 预先计算会话键，使会话作用域命令（/cmd、/pico、/clear...）
	// 获得与代理循环相同的键。路由失败时回退到 msg.SessionKey，
	// 这没问题 — 依赖上下文的命令会对 nil agent 进行防护。
	sessionKey := resolveScopeKey(route, msg.SessionKey)

	// 在要求成功路由之前检查命令。
	// 全局命令（/help、/show、/switch）即使路由失败也能工作；
	// 依赖上下文的命令检查自己的 Runtime 字段，
	// 并在所需能力为 nil 时报告"不可用"。
	if response, handled := al.handleCommand(ctx, msg, agent, sessionKey); handled {
		return response, nil
	}

	// 检查交互模式中是否有待处理的确认
	if conf := al.interactiveMgr.GetPendingConfirmation(sessionKey); conf != nil {
		// 用户正在回应待处理的确认
		err := al.interactiveMgr.RespondToConfirmation(conf.ID, msg.Content)
		if err != nil {
			return fmt.Sprintf("❌ Error: %v\n\nPending confirmation:\n%s", err, conf.Message), nil
		}
		return fmt.Sprintf("✅ Response recorded: %s\n\nProcessing...", msg.Content), nil
	}

	if routeErr != nil {
		return "", routeErr
	}

	logger.InfoCF("agent", "Routed message",
		map[string]any{
			"agent_id":      agent.ID,
			"scope_key":     sessionKey,
			"session_key":   sessionKey,
			"matched_by":    route.MatchedBy,
			"route_agent":   route.AgentID,
			"route_channel": route.Channel,
		})

	// 在任务队列中注册任务以支持取消
	taskID := sessionKey + ":" + time.Now().Format("20060102-150405.000")
	taskCtx, _ := al.taskQueue.Start(ctx, taskID, sessionKey)
	defer al.taskQueue.Finish(taskID) // 确保完成时清理

	logger.InfoCF("agent", "Task started",
		map[string]any{
			"task_id":     taskID,
			"session_key": sessionKey,
			"queue_size":  al.taskQueue.Count(),
		})

	opts := processOptions{
		SessionKey:      sessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		Media:           msg.Media,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
		WorkingDir:      msg.Metadata["work_dir"],
		Sender:          msg.Sender,
	}

	// 在命令模式下，将裸文本改写为 /exec，使所有 shell 执行
	// 流经注册表路径 — 无需第二个分发分支。
	content := strings.TrimSpace(msg.Content)
	if al.getSessionMode(sessionKey) == modeCmd && !commands.HasCommandPrefix(content) && content != "" {
		msg.Content = "/exec " + content
	}

	// 依赖上下文的命令检查自己的 Runtime 字段，
	// 并在所需能力为 nil 时报告"不可用"。
	if response, handled := al.handleCommand(taskCtx, msg, agent, sessionKey); handled {
		return response, nil
	}

	return al.runAgentLoop(taskCtx, agent, opts)
}

// resolveMessageRoute 解析消息的路由目标代理。
func (al *AgentLoop) resolveMessageRoute(msg bus.InboundMessage) (routing.ResolvedRoute, *AgentInstance, error) {
	route := al.registry.ResolveRoute(routing.RouteInput{
		Channel:    msg.Channel,
		AccountID:  inboundMetadata(msg, metadataKeyAccountID),
		Peer:       extractPeer(msg),
		ParentPeer: extractParentPeer(msg),
		GuildID:    inboundMetadata(msg, metadataKeyGuildID),
		TeamID:     inboundMetadata(msg, metadataKeyTeamID),
	})

	agent, ok := al.registry.GetAgent(route.AgentID)
	if !ok {
		agent = al.registry.GetDefaultAgent()
	}
	if agent == nil {
		return routing.ResolvedRoute{}, nil, fmt.Errorf("no agent available for route (agent_id=%s)", route.AgentID)
	}

	return route, agent, nil
}

// resolveScopeKey 解析会话作用域键。
func resolveScopeKey(route routing.ResolvedRoute, msgSessionKey string) string {
	if msgSessionKey != "" && strings.HasPrefix(msgSessionKey, sessionKeyAgentPrefix) {
		return msgSessionKey
	}
	return route.SessionKey
}

// processSystemMessage 处理系统消息，将异步工具结果路由回用户。
func (al *AgentLoop) processSystemMessage(
	ctx context.Context,
	msg bus.InboundMessage,
) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf(
			"processSystemMessage called with non-system message channel: %s",
			msg.Channel,
		)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// 从 chat_id 解析源频道（格式："channel:chat_id"）
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// 从消息内容中提取子代理结果
	// 格式："Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // 仅提取结果部分
	}

	// 跳过内部频道 - 仅记录日志，不发送给用户
	if channels.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// 系统消息使用默认代理
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for system message")
	}

	// 使用源会话以获取上下文
	sessionKey := routing.BuildAgentMainSessionKey(agent.ID)

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}

// runAgentLoop 是核心消息处理逻辑。
func (al *AgentLoop) runAgentLoop(
	ctx context.Context,
	agent *AgentInstance,
	opts processOptions,
) (string, error) {
	// 0a. 如果配置了会话超时，限制整体执行时间
	if agent.SessionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, agent.SessionTimeout)
		defer cancel()
	}

	// 0. 记录最后活跃频道用于心跳通知（跳过内部频道和 cli）
	if opts.Channel != "" && opts.ChatID != "" {
		if !channels.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF(
					"agent",
					"Failed to record last channel",
					map[string]any{"error": err.Error()},
				)
			}
		}
	}

	// 1. 构建消息（心跳跳过历史）
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = agent.Sessions.GetHistory(opts.SessionKey)
		summary = agent.Sessions.GetSummary(opts.SessionKey)
	}
	messages := agent.ContextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		opts.Media,
		opts.Channel,
		opts.ChatID,
	)

	// 将 media:// 引用解析为 base64 数据 URL（流式处理）
	maxMediaSize := al.cfg.Agents.Defaults.GetMaxMediaSize()
	messages = resolveMediaRefs(messages, al.mediaStore, maxMediaSize)

	// 如果设置了当前工作目录则注入到系统提示词中
	if opts.WorkingDir != "" && len(messages) > 0 && messages[0].Role == "system" {
		messages[0].Content += fmt.Sprintf(
			"\n\n## Current Working Directory\nThe user is currently working in: %s\n"+
				"When the user refers to files or directories, resolve them relative to this path, not the plugins directory root.",
			opts.WorkingDir,
		)
	}

	// 2. 将用户消息保存到会话
	agent.Sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 3. 运行 LLM 迭代循环
	finalContent, iteration, err := al.runLLMIteration(ctx, agent, messages, opts)
	if err != nil {
		return "", err
	}

	// 如果最后一个工具有 ForUser 内容且已发送，可能不需要发送最终响应
	// 这由工具的 Silent 标志和 ForUser 内容控制

	// 4. 处理空响应
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 5. 将最终助手消息保存到会话
	agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	agent.Sessions.Save(opts.SessionKey)

	// 6. 可选：摘要
	if opts.EnableSummary {
		al.maybeSummarize(agent, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 7. 可选：通过总线发送响应
	if opts.SendResponse {
		al.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	// 8. 记录响应
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]any{
			"agent_id":     agent.ID,
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

// GetStartupInfo 返回已加载工具和技能的信息，用于启动日志。
func (al *AgentLoop) GetStartupInfo() map[string]any {
	info := make(map[string]any)

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return info
	}

	// 工具信息
	toolsList := agent.Tools.List()
	info["tools"] = map[string]any{
		"count": len(toolsList),
		"names": toolsList,
	}

	// 技能信息
	info["skills"] = agent.ContextBuilder.GetSkillsInfo()

	// 代理信息
	info["agents"] = map[string]any{
		"count": len(al.registry.ListAgentIDs()),
		"ids":   al.registry.ListAgentIDs(),
	}

	return info
}

// GetUsageInfo 返回默认代理的累计 token 使用情况。
func (al *AgentLoop) GetUsageInfo() map[string]any {
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return nil
	}
	promptTokens := agent.TotalPromptTokens.Load()
	completionTokens := agent.TotalCompletionTokens.Load()
	return map[string]any{
		"model":             agent.Model,
		"max_tokens":        agent.MaxTokens,
		"temperature":       agent.Temperature,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      promptTokens + completionTokens,
		"requests":          agent.TotalRequests.Load(),
	}
}

// extractPeer 从入站消息中提取对端信息。
func extractPeer(msg bus.InboundMessage) *routing.RoutePeer {
	if msg.Peer.Kind == "" {
		return nil
	}
	peerID := msg.Peer.ID
	if peerID == "" {
		if msg.Peer.Kind == "direct" {
			peerID = msg.SenderID
		} else {
			peerID = msg.ChatID
		}
	}
	return &routing.RoutePeer{Kind: msg.Peer.Kind, ID: peerID}
}

// inboundMetadata 从入站消息元数据中获取指定键的值。
func inboundMetadata(msg bus.InboundMessage, key string) string {
	if msg.Metadata == nil {
		return ""
	}
	return msg.Metadata[key]
}

// extractParentPeer 从入站消息元数据中提取父对端（回复目标）信息。
func extractParentPeer(msg bus.InboundMessage) *routing.RoutePeer {
	parentKind := inboundMetadata(msg, metadataKeyParentPeerKind)
	parentID := inboundMetadata(msg, metadataKeyParentPeerID)
	if parentKind == "" || parentID == "" {
		return nil
	}
	return &routing.RoutePeer{Kind: parentKind, ID: parentID}
}
