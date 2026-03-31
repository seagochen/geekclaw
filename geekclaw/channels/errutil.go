package channels

import (
	"fmt"
	"net/http"
)

// ClassifySendError 根据 HTTP 状态码将原始错误包装为相应的哨兵错误。
// 执行 HTTP API 调用的频道应在其 Send 路径中使用此函数。
func ClassifySendError(statusCode int, rawErr error) error {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %v", ErrRateLimit, rawErr)
	case statusCode >= 500:
		return fmt.Errorf("%w: %v", ErrTemporary, rawErr)
	case statusCode >= 400:
		return fmt.Errorf("%w: %v", ErrSendFailed, rawErr)
	default:
		return rawErr
	}
}

// ClassifyNetError 将网络/超时错误包装为 ErrTemporary。
func ClassifyNetError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrTemporary, err)
}
