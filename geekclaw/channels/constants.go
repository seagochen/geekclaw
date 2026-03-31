// GeekClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

package channels

// internalChannels 定义用于内部通信的频道，
// 这些频道不应暴露给外部用户或记录为最后活跃频道。
var internalChannels = map[string]struct{}{
	"cli":      {},
	"system":   {},
	"subagent": {},
}

// IsInternalChannel 返回该频道是否为内部频道。
func IsInternalChannel(channel string) bool {
	_, found := internalChannels[channel]
	return found
}
