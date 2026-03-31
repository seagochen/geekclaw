package channels

import "errors"

var (
	// ErrNotRunning 表示频道未运行。
	// Manager 不会重试。
	ErrNotRunning = errors.New("channel not running")

	// ErrRateLimit 表示平台返回了速率限制响应（例如 HTTP 429）。
	// Manager 将等待固定延迟后重试。
	ErrRateLimit = errors.New("rate limited")

	// ErrTemporary 表示临时故障（例如网络超时、5xx）。
	// Manager 将使用指数退避进行重试。
	ErrTemporary = errors.New("temporary failure")

	// ErrSendFailed 表示永久性故障（例如无效的聊天 ID、非 429 的 4xx）。
	// Manager 不会重试。
	ErrSendFailed = errors.New("send failed")
)
