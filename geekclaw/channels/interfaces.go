package channels

import (
	"context"

	"github.com/seagosoft/geekclaw/geekclaw/commands"
	"github.com/seagosoft/geekclaw/geekclaw/media"
)

// TypingCapable — 能够显示输入/思考指示器的频道。
// StartTyping 开始显示指示器并返回停止函数。
// 停止函数必须是幂等的，可安全多次调用。
type TypingCapable interface {
	StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

// MessageEditor — 能够编辑现有消息的频道。
// messageID 始终为字符串；频道内部转换平台特定类型。
type MessageEditor interface {
	EditMessage(ctx context.Context, chatID string, messageID string, content string) error
}

// ReactionCapable — 能够对入站消息添加反应（如表情）的频道。
// ReactToMessage 添加反应并返回撤销函数以移除反应。
// 撤销函数必须是幂等的，可安全多次调用。
type ReactionCapable interface {
	ReactToMessage(ctx context.Context, chatID, messageID string) (undo func(), err error)
}

// PlaceholderCapable — 能够发送占位符消息（如"思考中..."）的频道，
// 该消息稍后会被编辑为实际回复。
// 频道必须同时实现 MessageEditor 才能使占位符有效。
// SendPlaceholder 返回占位符的平台消息 ID，以便
// Manager.preSend 之后通过 MessageEditor.EditMessage 编辑它。
type PlaceholderCapable interface {
	SendPlaceholder(ctx context.Context, chatID string) (messageID string, err error)
}

// PlaceholderRecorder 由 Manager 注入到频道中。
// 频道在入站时调用这些方法来注册输入/占位符状态。
// Manager 在出站时使用已注册的状态来停止输入指示器并编辑占位符。
type PlaceholderRecorder interface {
	RecordPlaceholder(channel, chatID, placeholderID string)
	RecordTypingStop(channel, chatID string, stop func())
	RecordReactionUndo(channel, chatID string, undo func())
}

// CommandRegistrarCapable 由能够在上游平台注册命令菜单的频道实现
// （例如 Telegram BotCommand）。
// 不支持平台级命令菜单的频道可以忽略此接口。
type CommandRegistrarCapable interface {
	RegisterCommands(ctx context.Context, defs []commands.Definition) error
}

// MediaStoreAware 由接受 MediaStore 注入的频道实现。
type MediaStoreAware interface {
	SetMediaStore(s media.MediaStore)
}

// PlaceholderRecorderAware 由接受 PlaceholderRecorder 注入的频道实现。
type PlaceholderRecorderAware interface {
	SetPlaceholderRecorder(r PlaceholderRecorder)
}

// OwnerAware 由接受所有者 Channel 注入的频道实现。
// 所有者是嵌入 BaseChannel 的具体频道，使
// HandleMessage 能够自动触发输入/反应/占位符能力。
type OwnerAware interface {
	SetOwner(ch Channel)
}
