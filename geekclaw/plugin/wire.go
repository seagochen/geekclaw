// Package plugin provides shared infrastructure for external plugin processes
// communicating via JSON-RPC 2.0 over stdio. It is used by all plugin bridge
// packages (channels/external, commands/external, tools/external, voice/external,
// providers/external) to avoid duplicating wire types, transport, and process
// lifecycle management.
package plugin

import (
	"context"
	"encoding/json"
)

// --------------------------------------------------------------------------
// JSON-RPC 2.0 wire types
// --------------------------------------------------------------------------

// Request is a JSON-RPC 2.0 request sent from geekclaw to an external plugin.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response from an external plugin to geekclaw.
type Response struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      int64    `json:"id"`
	Result  any      `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification (no ID) from an external plugin.
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

// ServiceHandler 处理来自插件的反向 JSON-RPC 调用。
// 插件通过 host.* 方法调用 Go 宿主服务。
type ServiceHandler func(ctx context.Context, params json.RawMessage) (any, error)
