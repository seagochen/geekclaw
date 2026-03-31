package channels

import "net/http"

// WebhookHandler 是通过 HTTP webhook 接收消息的频道的可选接口。
// Manager 发现实现此接口的频道并将其注册到共享 HTTP 服务器上。
type WebhookHandler interface {
	// WebhookPath 返回在共享服务器上挂载此处理器的路径。
	// 示例："/webhook/line"、"/webhook/wecom"
	WebhookPath() string
	http.Handler // ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// HealthChecker 是在共享 HTTP 服务器上暴露健康检查端点的频道的可选接口。
type HealthChecker interface {
	HealthPath() string
	HealthHandler(w http.ResponseWriter, r *http.Request)
}
