package routing

import (
	"fmt"
	"strings"
)

// DMScope 控制私聊会话隔离的粒度。
type DMScope string

const (
	DMScopeMain                  DMScope = "main"                     // 所有私聊共享主会话
	DMScopePerPeer               DMScope = "per-peer"                 // 每个对端独立会话
	DMScopePerChannelPeer        DMScope = "per-channel-peer"         // 每个频道+对端独立会话
	DMScopePerAccountChannelPeer DMScope = "per-account-channel-peer" // 每个账户+频道+对端独立会话
)

// RoutePeer 表示一个具有类型和 ID 的聊天对端。
type RoutePeer struct {
	Kind string // "direct"（私聊）、"group"（群组）、"channel"（频道）
	ID   string // 对端唯一标识
}

// SessionKeyParams 保存会话键构建所需的所有输入参数。
type SessionKeyParams struct {
	AgentID       string              // 智能体 ID
	Channel       string              // 频道名称
	AccountID     string              // 账户 ID
	Peer          *RoutePeer          // 对话对端
	DMScope       DMScope             // 私聊会话隔离范围
	IdentityLinks map[string][]string // 身份关联映射
}

// ParsedSessionKey 是解析智能体作用域会话键的结果。
type ParsedSessionKey struct {
	AgentID string // 智能体 ID
	Rest    string // 会话键的剩余部分
}

// BuildAgentMainSessionKey 返回 "agent:<agentId>:main" 格式的主会话键。
func BuildAgentMainSessionKey(agentID string) string {
	return fmt.Sprintf("agent:%s:%s", NormalizeAgentID(agentID), DefaultMainKey)
}

// BuildAgentPeerSessionKey 根据智能体、频道、对端和私聊隔离范围构建会话键。
func BuildAgentPeerSessionKey(params SessionKeyParams) string {
	agentID := NormalizeAgentID(params.AgentID)

	peer := params.Peer
	if peer == nil {
		peer = &RoutePeer{Kind: "direct"}
	}
	peerKind := strings.TrimSpace(peer.Kind)
	if peerKind == "" {
		peerKind = "direct"
	}

	if peerKind == "direct" {
		dmScope := params.DMScope
		if dmScope == "" {
			dmScope = DMScopeMain
		}
		peerID := strings.TrimSpace(peer.ID)

		// 解析身份关联（跨平台折叠）
		if dmScope != DMScopeMain && peerID != "" {
			if linked := resolveLinkedPeerID(params.IdentityLinks, params.Channel, peerID); linked != "" {
				peerID = linked
			}
		}
		peerID = strings.ToLower(peerID)

		switch dmScope {
		case DMScopePerAccountChannelPeer:
			if peerID != "" {
				channel := normalizeChannel(params.Channel)
				accountID := NormalizeAccountID(params.AccountID)
				return fmt.Sprintf("agent:%s:%s:%s:direct:%s", agentID, channel, accountID, peerID)
			}
		case DMScopePerChannelPeer:
			if peerID != "" {
				channel := normalizeChannel(params.Channel)
				return fmt.Sprintf("agent:%s:%s:direct:%s", agentID, channel, peerID)
			}
		case DMScopePerPeer:
			if peerID != "" {
				return fmt.Sprintf("agent:%s:direct:%s", agentID, peerID)
			}
		}
		return BuildAgentMainSessionKey(agentID)
	}

	// 群组/频道对端始终获得独立的对端会话
	channel := normalizeChannel(params.Channel)
	peerID := strings.ToLower(strings.TrimSpace(peer.ID))
	if peerID == "" {
		peerID = "unknown"
	}
	return fmt.Sprintf("agent:%s:%s:%s:%s", agentID, channel, peerKind, peerID)
}

// ParseAgentSessionKey 从 "agent:<agentId>:<rest>" 中提取 agentId 和 rest。
func ParseAgentSessionKey(sessionKey string) *ParsedSessionKey {
	raw := strings.TrimSpace(sessionKey)
	if raw == "" {
		return nil
	}
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) < 3 {
		return nil
	}
	if parts[0] != "agent" {
		return nil
	}
	agentID := strings.TrimSpace(parts[1])
	rest := parts[2]
	if agentID == "" || rest == "" {
		return nil
	}
	return &ParsedSessionKey{AgentID: agentID, Rest: rest}
}

// IsSubagentSessionKey 判断会话键是否代表子智能体。
func IsSubagentSessionKey(sessionKey string) bool {
	raw := strings.TrimSpace(sessionKey)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(raw), "subagent:") {
		return true
	}
	parsed := ParseAgentSessionKey(raw)
	if parsed == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(parsed.Rest), "subagent:")
}

// normalizeChannel 规范化频道名称为小写，空值返回 "unknown"。
func normalizeChannel(channel string) string {
	c := strings.TrimSpace(strings.ToLower(channel))
	if c == "" {
		return "unknown"
	}
	return c
}

// resolveLinkedPeerID 通过身份关联映射解析对端 ID，实现跨平台身份折叠。
func resolveLinkedPeerID(identityLinks map[string][]string, channel, peerID string) string {
	if len(identityLinks) == 0 {
		return ""
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return ""
	}

	candidates := make(map[string]bool)
	rawCandidate := strings.ToLower(peerID)
	if rawCandidate != "" {
		candidates[rawCandidate] = true
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel != "" {
		scopedCandidate := fmt.Sprintf("%s:%s", channel, strings.ToLower(peerID))
		candidates[scopedCandidate] = true
	}

	// 如果 peerID 已经是规范的 "platform:id" 格式，也将裸 ID 部分
	// 添加为候选项，以便向后兼容使用原始 ID（如 "123" 而非 "telegram:123"）
	// 的 identity_links 配置。
	if idx := strings.Index(rawCandidate, ":"); idx > 0 && idx < len(rawCandidate)-1 {
		bareID := rawCandidate[idx+1:]
		candidates[bareID] = true
	}

	if len(candidates) == 0 {
		return ""
	}

	for canonical, ids := range identityLinks {
		canonicalName := strings.TrimSpace(canonical)
		if canonicalName == "" {
			continue
		}
		for _, id := range ids {
			normalized := strings.ToLower(strings.TrimSpace(id))
			if normalized != "" && candidates[normalized] {
				return canonicalName
			}
		}
	}
	return ""
}
