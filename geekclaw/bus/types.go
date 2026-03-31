package bus

// Peer 标识消息的路由对端（直接、群组、频道等）。
type Peer struct {
	Kind string `json:"kind"` // "direct" | "group" | "channel" | ""
	ID   string `json:"id"`
}

// SenderInfo 提供结构化的发送者身份信息。
type SenderInfo struct {
	Platform    string `json:"platform,omitempty"`     // "telegram"、"discord"、"slack" 等
	PlatformID  string `json:"platform_id,omitempty"`  // 原始平台 ID，例如 "123456"
	CanonicalID string `json:"canonical_id,omitempty"` // "platform:id" 格式
	Username    string `json:"username,omitempty"`     // 用户名（例如 @alice）
	DisplayName string `json:"display_name,omitempty"` // 显示名称
}

// InboundMessage 表示从渠道接收的入站消息。
type InboundMessage struct {
	Channel    string            `json:"channel"`
	SenderID   string            `json:"sender_id"`
	Sender     SenderInfo        `json:"sender"`
	ChatID     string            `json:"chat_id"`
	Content    string            `json:"content"`
	Media      []string          `json:"media,omitempty"`
	Peer       Peer              `json:"peer"`                  // 路由对端
	MessageID  string            `json:"message_id,omitempty"`  // 平台消息 ID
	MediaScope string            `json:"media_scope,omitempty"` // 媒体生命周期范围
	SessionKey string            `json:"session_key"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage 表示发送到渠道的出站消息。
type OutboundMessage struct {
	Channel          string `json:"channel"`
	ChatID           string `json:"chat_id"`
	Content          string `json:"content"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}

// MediaPart 描述要发送的单个媒体附件。
type MediaPart struct {
	Type        string `json:"type"`                   // "image" | "audio" | "video" | "file"
	Ref         string `json:"ref"`                    // 媒体存储引用，例如 "media://abc123"
	Caption     string `json:"caption,omitempty"`      // 可选的说明文字
	Filename    string `json:"filename,omitempty"`     // 原始文件名提示
	ContentType string `json:"content_type,omitempty"` // MIME 类型提示
}

// OutboundMediaMessage 通过总线将媒体附件从 Agent 传送到渠道。
type OutboundMediaMessage struct {
	Channel string      `json:"channel"`
	ChatID  string      `json:"chat_id"`
	Parts   []MediaPart `json:"parts"`
}
