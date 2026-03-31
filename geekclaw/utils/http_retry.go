package utils

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// maxRetries 是 HTTP 请求的最大重试次数。
const maxRetries = 3

// retryDelayUnit 是重试间隔的基本时间单位。
var retryDelayUnit = time.Second

// shouldRetry 判断给定的 HTTP 状态码是否应该重试。
func shouldRetry(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode >= 500
}

// DoRequestWithRetry 执行 HTTP 请求，并在遇到可重试错误时自动重试。
func DoRequestWithRetry(client *http.Client, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for i := range maxRetries {
		if i > 0 && resp != nil {
			resp.Body.Close()
		}

		resp, err = client.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				break
			}
			if !shouldRetry(resp.StatusCode) {
				break
			}
		}

		if i < maxRetries-1 {
			if err = sleepWithCtx(req.Context(), retryDelayUnit*time.Duration(i+1)); err != nil {
				if resp != nil {
					resp.Body.Close()
				}
				return nil, fmt.Errorf("failed to sleep: %w", err)
			}
		}
	}
	return resp, err
}

// sleepWithCtx 在给定的持续时间内休眠，支持上下文取消。
func sleepWithCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
