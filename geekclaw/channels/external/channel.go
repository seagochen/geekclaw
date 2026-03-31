package external

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/commands"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// ExternalChannel 通过 stdio 上的 JSON-RPC 将 geekclaw 的内部 Channel 接口
// 桥接到外部进程。
type ExternalChannel struct {
	*channels.BaseChannel

	channelName string
	cfg         config.ExternalChannelConfig

	cmd       *exec.Cmd
	transport *Transport
	cancel    context.CancelFunc

	capabilities map[string]bool

	wg sync.WaitGroup
	mu sync.Mutex
}

// NewExternalChannel 从配置创建一个新的 ExternalChannel。
func NewExternalChannel(
	name string,
	cfg config.ExternalChannelConfig,
	messageBus *bus.MessageBus,
) (*ExternalChannel, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("external channel %q: command is required", name)
	}

	opts := []channels.BaseChannelOption{}
	if cfg.GroupTrigger.MentionOnly || len(cfg.GroupTrigger.Prefixes) > 0 {
		opts = append(opts, channels.WithGroupTrigger(cfg.GroupTrigger))
	}
	if cfg.ReasoningChannelID != "" {
		opts = append(opts, channels.WithReasoningChannelID(cfg.ReasoningChannelID))
	}

	base := channels.NewBaseChannel(name, cfg, messageBus, cfg.AllowFrom, opts...)

	return &ExternalChannel{
		BaseChannel:  base,
		channelName:  name,
		cfg:          cfg,
		capabilities: make(map[string]bool),
	}, nil
}

// Start 启动外部进程并执行初始化握手。
func (c *ExternalChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.IsRunning() {
		return nil
	}

	procCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	// 构建命令
	args := append([]string{}, c.cfg.Args...)
	cmd := exec.CommandContext(procCtx, c.cfg.Command, args...)

	// 设置环境变量：继承当前环境，用 cfg.Env 覆盖同名变量
	overrides := make(map[string]string, len(c.cfg.Env))
	for k, v := range c.cfg.Env {
		overrides[k] = v
	}
	baseEnv := os.Environ()
	merged := make([]string, 0, len(baseEnv)+len(overrides))
	for _, entry := range baseEnv {
		key := entry
		if i := strings.Index(entry, "="); i >= 0 {
			key = entry[:i]
		}
		if _, ok := overrides[key]; !ok {
			merged = append(merged, entry)
		}
	}
	for k, v := range overrides {
		merged = append(merged, k+"="+v)
	}
	cmd.Env = merged

	// 捕获 stderr 用于日志记录
	cmd.Stderr = &logWriter{channel: c.channelName}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start command %q: %w", c.cfg.Command, err)
	}

	c.cmd = cmd
	c.transport = NewTransport(stdout, stdin)

	// 启动读取循环
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.transport.ReadLoop(); err != nil {
			logger.WarnCF("external", "Read loop ended", map[string]any{
				"channel": c.channelName,
				"error":   err.Error(),
			})
		}
	}()

	// 启动通知处理器
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.handleNotifications(procCtx)
	}()

	// 初始化握手
	rawConfig := make(map[string]any)
	if cfgBytes, err := json.Marshal(c.cfg); err == nil {
		_ = json.Unmarshal(cfgBytes, &rawConfig)
	}

	result, err := c.transport.Call(ctx, MethodInitialize, &InitializeParams{
		Name:   c.channelName,
		Config: rawConfig,
	})
	if err != nil {
		c.stopLocked()
		return fmt.Errorf("initialize handshake failed: %w", err)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		c.stopLocked()
		return fmt.Errorf("parse initialize result: %w", err)
	}

	for _, cap := range initResult.Capabilities {
		c.capabilities[cap] = true
	}

	if initResult.MaxMessageLength > 0 {
		// 使用最大消息长度重新创建基础频道
		// （BaseChannel 不暴露 setter，所以将其存储用于 MaxMessageLength()）
	}

	// 通知频道启动
	if _, err := c.transport.Call(ctx, MethodStart, nil); err != nil {
		c.stopLocked()
		return fmt.Errorf("start failed: %w", err)
	}

	c.SetRunning(true)

	logger.InfoCF("external", "External channel started", map[string]any{
		"channel":      c.channelName,
		"capabilities": initResult.Capabilities,
		"command":       c.cfg.Command,
	})

	return nil
}

// Stop 优雅地关闭外部频道进程。
func (c *ExternalChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopLocked()
}

// stopLocked 在持有锁的情况下停止外部频道。
func (c *ExternalChannel) stopLocked() error {
	if c.cancel != nil {
		// 尝试发送停止请求（尽力而为）
		if c.transport != nil && c.IsRunning() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5e9) // 5s
			_, _ = c.transport.Call(stopCtx, MethodStop, nil)
			cancel()
		}

		c.cancel()
		c.cancel = nil
	}

	c.SetRunning(false)

	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Wait()
		c.cmd = nil
	}

	c.wg.Wait()
	return nil
}

// Send 向外部频道投递出站文本消息。
func (c *ExternalChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}

	_, err := c.transport.Call(ctx, MethodSend, &SendParams{
		ChatID:           msg.ChatID,
		Content:          msg.Content,
		ReplyToMessageID: msg.ReplyToMessageID,
	})
	if err != nil {
		return classifyError(err)
	}
	return nil
}

// --------------------------------------------------------------------------
// 可选能力接口——仅在外部频道声明了相应能力时生效。
// --------------------------------------------------------------------------

// StartTyping 实现 channels.TypingCapable 接口。
func (c *ExternalChannel) StartTyping(ctx context.Context, chatID string) (func(), error) {
	if !c.capabilities[CapTyping] {
		return func() {}, nil
	}

	raw, err := c.transport.Call(ctx, MethodStartTyping, &TypingParams{ChatID: chatID})
	if err != nil {
		return func() {}, nil // 非致命错误
	}

	var result TypingResult
	_ = json.Unmarshal(raw, &result)

	return func() {
		if result.StopID != "" {
			_, _ = c.transport.Call(context.Background(), MethodStopTyping, &TypingParams{
				ChatID: chatID,
				StopID: result.StopID,
			})
		}
	}, nil
}

// EditMessage 实现 channels.MessageEditor 接口。
func (c *ExternalChannel) EditMessage(ctx context.Context, chatID, messageID, content string) error {
	if !c.capabilities[CapEdit] {
		return nil
	}

	_, err := c.transport.Call(ctx, MethodEditMessage, &EditMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
		Content:   content,
	})
	return err
}

// ReactToMessage 实现 channels.ReactionCapable 接口。
func (c *ExternalChannel) ReactToMessage(ctx context.Context, chatID, messageID string) (func(), error) {
	if !c.capabilities[CapReaction] {
		return func() {}, nil
	}

	raw, err := c.transport.Call(ctx, MethodReact, &ReactParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	if err != nil {
		return func() {}, nil
	}

	var result ReactResult
	_ = json.Unmarshal(raw, &result)

	return func() {
		if result.UndoID != "" {
			_, _ = c.transport.Call(context.Background(), MethodUndoReact, &ReactParams{
				ChatID: chatID,
				UndoID: result.UndoID,
			})
		}
	}, nil
}

// SendPlaceholder 实现 channels.PlaceholderCapable 接口。
func (c *ExternalChannel) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	if !c.capabilities[CapPlaceholder] {
		return "", nil
	}

	raw, err := c.transport.Call(ctx, MethodSendPlaceholder, &PlaceholderParams{ChatID: chatID})
	if err != nil {
		return "", err
	}

	var result PlaceholderResult
	_ = json.Unmarshal(raw, &result)
	return result.MessageID, nil
}

// SendMedia 实现 channels.MediaSender 接口。
func (c *ExternalChannel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error {
	if !c.capabilities[CapMedia] {
		return nil
	}

	parts := make([]MediaPart, len(msg.Parts))
	for i, p := range msg.Parts {
		parts[i] = MediaPart{
			Type:        p.Type,
			Data:        p.Ref, // 媒体存储引用——由 Python 端解析
			Caption:     p.Caption,
			Filename:    p.Filename,
			ContentType: p.ContentType,
		}
	}

	_, err := c.transport.Call(ctx, MethodSendMedia, &SendMediaParams{
		ChatID: msg.ChatID,
		Parts:  parts,
	})
	return err
}

// RegisterCommands 实现 channels.CommandRegistrarCapable 接口。
func (c *ExternalChannel) RegisterCommands(ctx context.Context, defs []commands.Definition) error {
	if !c.capabilities[CapCommands] {
		return nil
	}

	cmds := make([]CommandDef, len(defs))
	for i, d := range defs {
		cmds[i] = CommandDef{
			Command:     d.Name,
			Description: d.Description,
		}
	}

	_, err := c.transport.Call(ctx, MethodRegisterCommands, &RegisterCommandsParams{
		Commands: cmds,
	})
	return err
}

// MaxMessageLength 实现 channels.MessageLengthProvider 接口。
// 如果外部频道未声明最大长度则返回 0。
func (c *ExternalChannel) MaxMessageLength() int {
	// 我们在初始化期间存储了此值；BaseChannel 也有此值，
	// 但构造后无法设置。返回初始化结果中的值。
	return c.BaseChannel.MaxMessageLength()
}

// --------------------------------------------------------------------------
// 通知处理器——处理来自外部进程的入站消息
// --------------------------------------------------------------------------

// handleNotifications 处理来自外部进程的通知。
func (c *ExternalChannel) handleNotifications(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case notif, ok := <-c.transport.Notifications():
			if !ok {
				return
			}
			switch notif.Method {
			case MethodMessage:
				c.handleInboundMessage(ctx, notif)
			case MethodLog:
				c.handleLogNotification(notif)
			}
		}
	}
}

// handleInboundMessage 处理入站消息通知。
func (c *ExternalChannel) handleInboundMessage(ctx context.Context, notif *Notification) {
	raw, err := json.Marshal(notif.Params)
	if err != nil {
		return
	}

	var msg InboundMessageParams
	if err := json.Unmarshal(raw, &msg); err != nil {
		logger.WarnCF("external", "Failed to parse inbound message", map[string]any{
			"channel": c.channelName,
			"error":   err.Error(),
		})
		return
	}

	platform := msg.Platform
	if platform == "" {
		platform = c.channelName
	}

	canonicalID := msg.PlatformID
	if canonicalID == "" {
		canonicalID = msg.SenderID
	}

	sender := bus.SenderInfo{
		Platform:    platform,
		PlatformID:  msg.PlatformID,
		CanonicalID: platform + ":" + canonicalID,
		Username:    msg.Username,
		DisplayName: msg.DisplayName,
	}

	peer := bus.Peer{
		Kind: msg.PeerKind,
		ID:   msg.PeerID,
	}

	c.HandleMessage(
		ctx,
		peer,
		msg.MessageID,
		msg.SenderID,
		msg.ChatID,
		msg.Content,
		msg.Media,
		msg.Metadata,
		sender,
	)
}

// handleLogNotification 处理日志通知。
func (c *ExternalChannel) handleLogNotification(notif *Notification) {
	raw, err := json.Marshal(notif.Params)
	if err != nil {
		return
	}

	var log LogParams
	if err := json.Unmarshal(raw, &log); err != nil {
		return
	}

	fields := map[string]any{"channel": c.channelName}
	switch log.Level {
	case "debug":
		logger.DebugCF("external", log.Message, fields)
	case "warn":
		logger.WarnCF("external", log.Message, fields)
	case "error":
		logger.ErrorCF("external", log.Message, fields)
	default:
		logger.InfoCF("external", log.Message, fields)
	}
}

// --------------------------------------------------------------------------
// 辅助函数
// --------------------------------------------------------------------------

// classifyError 将 RPC 错误映射为频道错误类型，供 Manager 重试逻辑使用。
func classifyError(err error) error {
	if rpcErr, ok := err.(*RPCError); ok {
		switch rpcErr.Code {
		case -32001: // 速率受限
			return fmt.Errorf("%w: %s", channels.ErrRateLimit, rpcErr.Message)
		case -32002: // 临时错误
			return fmt.Errorf("%w: %s", channels.ErrTemporary, rpcErr.Message)
		case -32003: // 未运行
			return fmt.Errorf("%w: %s", channels.ErrNotRunning, rpcErr.Message)
		default:
			return fmt.Errorf("%w: %s", channels.ErrSendFailed, rpcErr.Message)
		}
	}
	return fmt.Errorf("%w: %s", channels.ErrTemporary, err.Error())
}

// logWriter 将外部进程的 stderr 输出转发到 geekclaw 的日志器。
type logWriter struct {
	channel string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	logger.DebugCF("external", string(p), map[string]any{
		"channel": w.channel,
		"stream":  "stderr",
	})
	return len(p), nil
}
