// Package health 提供健康检查 HTTP 服务。
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"sync"
	"time"
)

// Server 是健康检查 HTTP 服务器。
type Server struct {
	server    *http.Server
	mu        sync.RWMutex
	ready     bool
	checks    map[string]Check
	startTime time.Time
}

// Check 表示一个健康检查项。
type Check struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// StatusResponse 是健康检查的响应结构。
type StatusResponse struct {
	Status string           `json:"status"`
	Uptime string           `json:"uptime"`
	Checks map[string]Check `json:"checks,omitempty"`
}

// NewServer 创建一个新的健康检查服务器。
func NewServer(host string, port int) *Server {
	mux := http.NewServeMux()
	s := &Server{
		ready:     false,
		checks:    make(map[string]Check),
		startTime: time.Now(),
	}

	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)

	addr := fmt.Sprintf("%s:%d", host, port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s
}

// Start 启动健康检查服务器。
func (s *Server) Start() error {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	return s.server.ListenAndServe()
}

// StartContext 在给定上下文中启动健康检查服务器。
func (s *Server) StartContext(ctx context.Context) error {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.server.Shutdown(context.Background())
	}
}

// Stop 优雅地停止健康检查服务器。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.ready = false
	s.mu.Unlock()
	return s.server.Shutdown(ctx)
}

// SetReady 设置服务器的就绪状态。
func (s *Server) SetReady(ready bool) {
	s.mu.Lock()
	s.ready = ready
	s.mu.Unlock()
}

// RegisterCheck 注册一个健康检查项。
func (s *Server) RegisterCheck(name string, checkFn func() (bool, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, msg := checkFn()
	s.checks[name] = Check{
		Name:      name,
		Status:    statusString(status),
		Message:   msg,
		Timestamp: time.Now(),
	}
}

// healthHandler 处理 /health 端点请求。
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	uptime := time.Since(s.startTime)
	resp := StatusResponse{
		Status: "ok",
		Uptime: uptime.String(),
	}

	json.NewEncoder(w).Encode(resp)
}

// readyHandler 处理 /ready 端点请求。
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.mu.RLock()
	ready := s.ready
	checks := make(map[string]Check)
	maps.Copy(checks, s.checks)
	s.mu.RUnlock()

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(StatusResponse{
			Status: "not ready",
			Checks: checks,
		})
		return
	}

	for _, check := range checks {
		if check.Status == "fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(StatusResponse{
				Status: "not ready",
				Checks: checks,
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	uptime := time.Since(s.startTime)
	json.NewEncoder(w).Encode(StatusResponse{
		Status: "ready",
		Uptime: uptime.String(),
		Checks: checks,
	})
}

// RegisterOnMux 将 /health 和 /ready 处理器注册到给定的 mux 上。
// 允许健康检查端点由共享的 HTTP 服务器提供服务。
func (s *Server) RegisterOnMux(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)
}

// statusString 将布尔状态转换为字符串。
func statusString(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}
