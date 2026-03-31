package channels

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/media"
)

var (
	uniqueIDCounter uint64
	uniqueIDPrefix  string
)

func init() {
	// 从 crypto/rand 一次性读取唯一前缀（单次系统调用）。
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 回退到基于时间的前缀
		binary.BigEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	uniqueIDPrefix = hex.EncodeToString(b[:])
}

// audioAnnotationRe 匹配频道注入的音频/语音标注（例如 [voice]、[audio: file.ogg]）。
var audioAnnotationRe = regexp.MustCompile(`\[(voice|audio)(?::[^\]]*)?\]`)

// uniqueID 使用随机前缀和原子计数器生成进程内唯一 ID。
// 此 ID 仅用于内部关联（例如媒体作用域键），不具备密码学安全性——
// 不得在需要不可预测性的场景中使用。
func uniqueID() string {
	n := atomic.AddUint64(&uniqueIDCounter, 1)
	return uniqueIDPrefix + strconv.FormatUint(n, 16)
}

// Channel 定义了消息频道的核心接口。
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg bus.OutboundMessage) error
	IsRunning() bool
	IsAllowed(senderID string) bool
	IsAllowedSender(sender bus.SenderInfo) bool
	ReasoningChannelID() string
}

// BaseChannelOption 是用于配置 BaseChannel 的函数式选项。
type BaseChannelOption func(*BaseChannel)

// WithMaxMessageLength 设置频道的最大消息长度（以 rune 计）。
// 超过此限制的消息将由 Manager 自动拆分。
// 值为 0 表示无限制。
func WithMaxMessageLength(n int) BaseChannelOption {
	return func(c *BaseChannel) { c.maxMessageLength = n }
}

// WithGroupTrigger 设置频道的群组触发配置。
func WithGroupTrigger(gt config.GroupTriggerConfig) BaseChannelOption {
	return func(c *BaseChannel) { c.groupTrigger = gt }
}

// WithReasoningChannelID 设置推理频道 ID，用于发送思维链内容。
func WithReasoningChannelID(id string) BaseChannelOption {
	return func(c *BaseChannel) { c.reasoningChannelID = id }
}

// MessageLengthProvider 是频道可选实现的接口，用于声明最大消息长度。
// Manager 通过类型断言使用此接口来决定是否拆分出站消息。
type MessageLengthProvider interface {
	MaxMessageLength() int
}

// BaseChannel 提供频道的基础实现，包含通用配置和消息处理逻辑。
type BaseChannel struct {
	config              any
	bus                 *bus.MessageBus
	running             atomic.Bool
	name                string
	allowList           []string
	maxMessageLength    int
	groupTrigger        config.GroupTriggerConfig
	mediaStore          media.MediaStore
	placeholderRecorder PlaceholderRecorder
	owner               Channel // 嵌入此 BaseChannel 的具体频道
	reasoningChannelID  string
}

// NewBaseChannel 创建一个新的基础频道实例。
func NewBaseChannel(
	name string,
	config any,
	bus *bus.MessageBus,
	allowList []string,
	opts ...BaseChannelOption,
) *BaseChannel {
	bc := &BaseChannel{
		config:    config,
		bus:       bus,
		name:      name,
		allowList: allowList,
	}
	for _, opt := range opts {
		opt(bc)
	}
	return bc
}

// MaxMessageLength 返回此频道的最大消息长度（以 rune 计）。
// 值为 0 表示无限制。
func (c *BaseChannel) MaxMessageLength() int {
	return c.maxMessageLength
}

// ShouldRespondInGroup 判断机器人是否应在群聊中回复。
// 各频道负责：
//  1. 检测 isMentioned（平台相关）
//  2. 从内容中去除机器人提及（平台相关）
//  3. 调用此方法获取群组回复决策
//
// 逻辑：
//   - 如果被提及 → 始终回复
//   - 如果配置了 mention_only 且未被提及 → 忽略
//   - 如果配置了前缀 → 内容以任一前缀开头时回复（并去除前缀）
//   - 如果配置了前缀但无匹配且未被提及 → 忽略
//   - 否则（未配置 group_trigger）→ 回复所有消息（宽松默认）
func (c *BaseChannel) ShouldRespondInGroup(isMentioned bool, content string) (bool, string) {
	gt := c.groupTrigger

	// 被提及 → 始终回复
	if isMentioned {
		return true, strings.TrimSpace(content)
	}

	// mention_only → 要求提及
	if gt.MentionOnly {
		return false, content
	}

	// 前缀匹配
	if len(gt.Prefixes) > 0 {
		for _, prefix := range gt.Prefixes {
			if prefix != "" && strings.HasPrefix(content, prefix) {
				return true, strings.TrimSpace(strings.TrimPrefix(content, prefix))
			}
		}
		// 配置了前缀但无匹配且未被提及 → 忽略
		return false, content
	}

	// 未配置 group_trigger → 宽松模式（回复所有消息）
	return true, strings.TrimSpace(content)
}

// Name 返回频道名称。
func (c *BaseChannel) Name() string {
	return c.name
}

// ReasoningChannelID 返回推理频道 ID。
func (c *BaseChannel) ReasoningChannelID() string {
	return c.reasoningChannelID
}

// IsRunning 返回频道是否正在运行。
func (c *BaseChannel) IsRunning() bool {
	return c.running.Load()
}

// IsAllowed 检查给定的发送者 ID 是否在允许列表中。
func (c *BaseChannel) IsAllowed(senderID string) bool {
	if len(c.allowList) == 0 {
		return true
	}

	// 从复合 senderID（如 "123456|username"）中提取各部分
	idPart := senderID
	userPart := ""
	if idx := strings.Index(senderID, "|"); idx > 0 {
		idPart = senderID[:idx]
		userPart = senderID[idx+1:]
	}

	for _, allowed := range c.allowList {
		// 去除允许值开头的 "@" 以进行用户名匹配
		trimmed := strings.TrimPrefix(allowed, "@")
		allowedID := trimmed
		allowedUser := ""
		if idx := strings.Index(trimmed, "|"); idx > 0 {
			allowedID = trimmed[:idx]
			allowedUser = trimmed[idx+1:]
		}

		// 支持任一侧使用 "id|username" 复合格式。
		// 保持与旧版 Telegram 允许列表条目的向后兼容性。
		if senderID == allowed ||
			idPart == allowed ||
			senderID == trimmed ||
			idPart == trimmed ||
			idPart == allowedID ||
			(allowedUser != "" && senderID == allowedUser) ||
			(userPart != "" && (userPart == allowed || userPart == trimmed || userPart == allowedUser)) {
			return true
		}
	}

	return false
}

// IsAllowedSender 检查结构化的 SenderInfo 是否被允许列表许可。
// 它为每个条目委托给 MatchAllowed，在所有旧格式和新的规范 "platform:id" 格式之间
// 提供统一的匹配。
func (c *BaseChannel) IsAllowedSender(sender bus.SenderInfo) bool {
	if len(c.allowList) == 0 {
		return true
	}

	for _, allowed := range c.allowList {
		if MatchAllowed(sender, allowed) {
			return true
		}
	}

	return false
}

// HandleMessage 处理入站消息，进行权限检查并发布到消息总线。
func (c *BaseChannel) HandleMessage(
	ctx context.Context,
	peer bus.Peer,
	messageID, senderID, chatID, content string,
	media []string,
	metadata map[string]string,
	senderOpts ...bus.SenderInfo,
) {
	// 优先使用基于 SenderInfo 的权限检查，否则回退到字符串匹配
	var sender bus.SenderInfo
	if len(senderOpts) > 0 {
		sender = senderOpts[0]
	}
	if sender.CanonicalID != "" || sender.PlatformID != "" {
		if !c.IsAllowedSender(sender) {
			return
		}
	} else {
		if !c.IsAllowed(senderID) {
			return
		}
	}

	// 如果有规范 ID 则使用，否则保留原始 senderID
	resolvedSenderID := senderID
	if sender.CanonicalID != "" {
		resolvedSenderID = sender.CanonicalID
	}

	scope := BuildMediaScope(c.name, chatID, messageID)

	msg := bus.InboundMessage{
		Channel:    c.name,
		SenderID:   resolvedSenderID,
		Sender:     sender,
		ChatID:     chatID,
		Content:    content,
		Media:      media,
		Peer:       peer,
		MessageID:  messageID,
		MediaScope: scope,
		Metadata:   metadata,
	}

	// 发布前自动触发输入指示器、消息反应和占位符。
	// 每个能力独立运作——同一消息可能同时触发三者。
	if c.owner != nil && c.placeholderRecorder != nil {
		// 输入指示器——独立流水线
		if tc, ok := c.owner.(TypingCapable); ok {
			if stop, err := tc.StartTyping(ctx, chatID); err == nil {
				c.placeholderRecorder.RecordTypingStop(c.name, chatID, stop)
			}
		}
		// 反应——独立流水线
		if rc, ok := c.owner.(ReactionCapable); ok && messageID != "" {
			if undo, err := rc.ReactToMessage(ctx, chatID, messageID); err == nil {
				c.placeholderRecorder.RecordReactionUndo(c.name, chatID, undo)
			}
		}
		// 占位符——独立流水线。
		// 当消息包含音频时跳过：代理将在转录完成后发送占位符，
		// 这样用户仅在语音处理完毕后才会看到"思考中…"。
		if !audioAnnotationRe.MatchString(content) {
			if pc, ok := c.owner.(PlaceholderCapable); ok {
				if phID, err := pc.SendPlaceholder(ctx, chatID); err == nil && phID != "" {
					c.placeholderRecorder.RecordPlaceholder(c.name, chatID, phID)
				}
			}
		}
	}

	if err := c.bus.PublishInbound(ctx, msg); err != nil {
		logger.ErrorCF("channels", "Failed to publish inbound message", map[string]any{
			"channel": c.name,
			"chat_id": chatID,
			"error":   err.Error(),
		})
	}
}

// SetRunning 设置频道的运行状态。
func (c *BaseChannel) SetRunning(running bool) {
	c.running.Store(running)
}

// SetMediaStore 将 MediaStore 注入到频道中。
func (c *BaseChannel) SetMediaStore(s media.MediaStore) { c.mediaStore = s }

// GetMediaStore 返回注入的 MediaStore（可能为 nil）。
func (c *BaseChannel) GetMediaStore() media.MediaStore { return c.mediaStore }

// SetPlaceholderRecorder 将 PlaceholderRecorder 注入到频道中。
func (c *BaseChannel) SetPlaceholderRecorder(r PlaceholderRecorder) {
	c.placeholderRecorder = r
}

// GetPlaceholderRecorder 返回注入的 PlaceholderRecorder（可能为 nil）。
func (c *BaseChannel) GetPlaceholderRecorder() PlaceholderRecorder {
	return c.placeholderRecorder
}

// SetOwner 注入嵌入此 BaseChannel 的具体频道。
// 使 HandleMessage 能够自动触发 TypingCapable / ReactionCapable / PlaceholderCapable。
func (c *BaseChannel) SetOwner(ch Channel) {
	c.owner = ch
}

// BuildMediaScope 构建用于媒体生命周期跟踪的作用域键。
func BuildMediaScope(channel, chatID, messageID string) string {
	id := messageID
	if id == "" {
		id = uniqueID()
	}
	return channel + ":" + chatID + ":" + id
}
