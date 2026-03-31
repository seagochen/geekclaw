package routing

import (
	"strings"

	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// RouteInput 包含来自入站消息的路由上下文。
type RouteInput struct {
	Channel    string      // 频道名称
	AccountID  string      // 账户 ID
	Peer       *RoutePeer  // 对话对端
	ParentPeer *RoutePeer  // 父级对话对端
	GuildID    string      // 公会 ID
	TeamID     string      // 团队 ID
}

// ResolvedRoute 是智能体路由的结果。
type ResolvedRoute struct {
	AgentID        string // 匹配到的智能体 ID
	Channel        string // 频道名称
	AccountID      string // 账户 ID
	SessionKey     string // 会话键
	MainSessionKey string // 主会话键
	MatchedBy      string // 匹配方式："binding.peer"、"binding.peer.parent"、"binding.guild"、"binding.team"、"binding.account"、"binding.channel"、"default"
}

// RouteResolver 根据配置绑定确定哪个智能体处理消息。
type RouteResolver struct {
	cfg *config.Config
}

// NewRouteResolver 创建一个新的路由解析器。
func NewRouteResolver(cfg *config.Config) *RouteResolver {
	return &RouteResolver{cfg: cfg}
}

// ResolveRoute 确定哪个智能体处理消息并构建会话键。
// 实现 7 级优先级级联：
// peer > parent_peer > guild > team > account > channel_wildcard > default
func (r *RouteResolver) ResolveRoute(input RouteInput) ResolvedRoute {
	channel := strings.ToLower(strings.TrimSpace(input.Channel))
	accountID := NormalizeAccountID(input.AccountID)
	peer := input.Peer

	dmScope := DMScope(r.cfg.Session.DMScope)
	if dmScope == "" {
		dmScope = DMScopeMain
	}
	identityLinks := r.cfg.Session.IdentityLinks

	bindings := r.filterBindings(channel, accountID)

	choose := func(agentID string, matchedBy string) ResolvedRoute {
		resolvedAgentID := r.pickAgentID(agentID)
		sessionKey := strings.ToLower(BuildAgentPeerSessionKey(SessionKeyParams{
			AgentID:       resolvedAgentID,
			Channel:       channel,
			AccountID:     accountID,
			Peer:          peer,
			DMScope:       dmScope,
			IdentityLinks: identityLinks,
		}))
		mainSessionKey := strings.ToLower(BuildAgentMainSessionKey(resolvedAgentID))
		return ResolvedRoute{
			AgentID:        resolvedAgentID,
			Channel:        channel,
			AccountID:      accountID,
			SessionKey:     sessionKey,
			MainSessionKey: mainSessionKey,
			MatchedBy:      matchedBy,
		}
	}

	// 优先级 1：对端绑定
	if peer != nil && strings.TrimSpace(peer.ID) != "" {
		if match := r.findPeerMatch(bindings, peer); match != nil {
			return choose(match.AgentID, "binding.peer")
		}
	}

	// 优先级 2：父级对端绑定
	parentPeer := input.ParentPeer
	if parentPeer != nil && strings.TrimSpace(parentPeer.ID) != "" {
		if match := r.findPeerMatch(bindings, parentPeer); match != nil {
			return choose(match.AgentID, "binding.peer.parent")
		}
	}

	// 优先级 3：公会绑定
	guildID := strings.TrimSpace(input.GuildID)
	if guildID != "" {
		if match := r.findGuildMatch(bindings, guildID); match != nil {
			return choose(match.AgentID, "binding.guild")
		}
	}

	// 优先级 4：团队绑定
	teamID := strings.TrimSpace(input.TeamID)
	if teamID != "" {
		if match := r.findTeamMatch(bindings, teamID); match != nil {
			return choose(match.AgentID, "binding.team")
		}
	}

	// 优先级 5：账户绑定
	if match := r.findAccountMatch(bindings); match != nil {
		return choose(match.AgentID, "binding.account")
	}

	// 优先级 6：频道通配符绑定
	if match := r.findChannelWildcardMatch(bindings); match != nil {
		return choose(match.AgentID, "binding.channel")
	}

	// 优先级 7：默认智能体
	return choose(r.resolveDefaultAgentID(), "default")
}

// filterBindings 按频道和账户 ID 过滤绑定配置。
func (r *RouteResolver) filterBindings(channel, accountID string) []config.AgentBinding {
	var filtered []config.AgentBinding
	for _, b := range r.cfg.Bindings {
		matchChannel := strings.ToLower(strings.TrimSpace(b.Match.Channel))
		if matchChannel == "" || matchChannel != channel {
			continue
		}
		if !matchesAccountID(b.Match.AccountID, accountID) {
			continue
		}
		filtered = append(filtered, b)
	}
	return filtered
}

// matchesAccountID 检查绑定中的账户 ID 是否匹配实际的账户 ID。
func matchesAccountID(matchAccountID, actual string) bool {
	trimmed := strings.TrimSpace(matchAccountID)
	if trimmed == "" {
		return actual == DefaultAccountID
	}
	if trimmed == "*" {
		return true
	}
	return strings.ToLower(trimmed) == strings.ToLower(actual)
}

// findPeerMatch 在绑定列表中查找匹配指定对端的绑定。
func (r *RouteResolver) findPeerMatch(bindings []config.AgentBinding, peer *RoutePeer) *config.AgentBinding {
	for i := range bindings {
		b := &bindings[i]
		if b.Match.Peer == nil {
			continue
		}
		peerKind := strings.ToLower(strings.TrimSpace(b.Match.Peer.Kind))
		peerID := strings.TrimSpace(b.Match.Peer.ID)
		if peerKind == "" || peerID == "" {
			continue
		}
		if peerKind == strings.ToLower(peer.Kind) && peerID == peer.ID {
			return b
		}
	}
	return nil
}

// findGuildMatch 在绑定列表中查找匹配指定公会 ID 的绑定。
func (r *RouteResolver) findGuildMatch(bindings []config.AgentBinding, guildID string) *config.AgentBinding {
	for i := range bindings {
		b := &bindings[i]
		matchGuild := strings.TrimSpace(b.Match.GuildID)
		if matchGuild != "" && matchGuild == guildID {
			return &bindings[i]
		}
	}
	return nil
}

// findTeamMatch 在绑定列表中查找匹配指定团队 ID 的绑定。
func (r *RouteResolver) findTeamMatch(bindings []config.AgentBinding, teamID string) *config.AgentBinding {
	for i := range bindings {
		b := &bindings[i]
		matchTeam := strings.TrimSpace(b.Match.TeamID)
		if matchTeam != "" && matchTeam == teamID {
			return &bindings[i]
		}
	}
	return nil
}

// findAccountMatch 在绑定列表中查找纯账户匹配的绑定（不含对端、公会或团队条件）。
func (r *RouteResolver) findAccountMatch(bindings []config.AgentBinding) *config.AgentBinding {
	for i := range bindings {
		b := &bindings[i]
		accountID := strings.TrimSpace(b.Match.AccountID)
		if accountID == "*" {
			continue
		}
		if b.Match.Peer != nil || b.Match.GuildID != "" || b.Match.TeamID != "" {
			continue
		}
		return &bindings[i]
	}
	return nil
}

// findChannelWildcardMatch 在绑定列表中查找频道通配符匹配的绑定。
func (r *RouteResolver) findChannelWildcardMatch(bindings []config.AgentBinding) *config.AgentBinding {
	for i := range bindings {
		b := &bindings[i]
		accountID := strings.TrimSpace(b.Match.AccountID)
		if accountID != "*" {
			continue
		}
		if b.Match.Peer != nil || b.Match.GuildID != "" || b.Match.TeamID != "" {
			continue
		}
		return &bindings[i]
	}
	return nil
}

// pickAgentID 根据智能体 ID 选择对应的已配置智能体，若未找到则回退到默认智能体。
func (r *RouteResolver) pickAgentID(agentID string) string {
	trimmed := strings.TrimSpace(agentID)
	if trimmed == "" {
		return NormalizeAgentID(r.resolveDefaultAgentID())
	}
	normalized := NormalizeAgentID(trimmed)
	agents := r.cfg.Agents.List
	if len(agents) == 0 {
		return normalized
	}
	for _, a := range agents {
		if NormalizeAgentID(a.ID) == normalized {
			return normalized
		}
	}
	return NormalizeAgentID(r.resolveDefaultAgentID())
}

// resolveDefaultAgentID 解析配置中的默认智能体 ID。
func (r *RouteResolver) resolveDefaultAgentID() string {
	agents := r.cfg.Agents.List
	if len(agents) == 0 {
		return DefaultAgentID
	}
	for _, a := range agents {
		if a.Default {
			id := strings.TrimSpace(a.ID)
			if id != "" {
				return NormalizeAgentID(id)
			}
		}
	}
	if id := strings.TrimSpace(agents[0].ID); id != "" {
		return NormalizeAgentID(id)
	}
	return DefaultAgentID
}
