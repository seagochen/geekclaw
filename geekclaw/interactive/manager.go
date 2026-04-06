// Package interactive 提供 AI 交互的会话状态管理。
package interactive

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout 是确认请求的默认超时时间。
const DefaultTimeout = 5 * time.Minute

// Manager 管理交互式会话和待处理的确认请求。
type Manager struct {
	sessions      map[string]*Session
	confirmations map[string]*ConfirmationRequest
	plans         map[string]*ExecutionPlan
	mu            sync.RWMutex
	stopCleanup   chan struct{} // 停止后台清理
}

// NewManager 创建一个新的交互管理器。
func NewManager() *Manager {
	return &Manager{
		sessions:      make(map[string]*Session),
		confirmations: make(map[string]*ConfirmationRequest),
		plans:         make(map[string]*ExecutionPlan),
	}
}

// StartCleanup 启动后台定期清理过期的确认请求和执行计划。
func (m *Manager) StartCleanup(interval time.Duration) {
	m.stopCleanup = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.stopCleanup:
				return
			case <-ticker.C:
				m.CleanupExpired()
			}
		}
	}()
}

// StopCleanup 停止后台清理。
func (m *Manager) StopCleanup() {
	if m.stopCleanup != nil {
		close(m.stopCleanup)
	}
}

// GetSession 获取或创建指定会话键的会话。
func (m *Manager) GetSession(sessionKey string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[sessionKey]; ok {
		return session
	}

	session := NewSession(sessionKey)
	m.sessions[sessionKey] = session
	return session
}

// HasSession 检查会话是否存在。
func (m *Manager) HasSession(sessionKey string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessions[sessionKey]
	return ok
}

// RemoveSession 移除会话及其所有待处理的确认请求。
func (m *Manager) RemoveSession(sessionKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 取消所有待处理的确认请求
	for id, conf := range m.confirmations {
		if conf.SessionKey == sessionKey {
			conf.State = StateCancelled
			conf.CloseResponseCh()
			delete(m.confirmations, id)
		}
	}

	// 取消所有活跃的执行计划
	for id, plan := range m.plans {
		if plan.SessionKey == sessionKey {
			plan.State = PlanCancelled
			delete(m.plans, id)
		}
	}

	delete(m.sessions, sessionKey)
}

// RequestConfirmation 创建一个新的确认请求并立即返回。
// 调用者应从 conf.ResponseCh 读取以阻塞等待用户响应。
func (m *Manager) RequestConfirmation(
	ctx context.Context,
	sessionKey string,
	message string,
	options []ConfirmationOption,
	timeout time.Duration,
) (*ConfirmationRequest, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	confID := generateConfirmationID(sessionKey)
	responseCh := make(chan string, 1)

	conf := &ConfirmationRequest{
		ID:         confID,
		SessionKey: sessionKey,
		Message:    message,
		Options:    options,
		State:      StatePending,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(timeout),
		ResponseCh: responseCh,
	}

	m.mu.Lock()
	m.confirmations[confID] = conf
	session := m.getOrCreateSessionLocked(sessionKey)
	session.PendingConfirmation = conf
	// 在锁内添加到会话历史，避免竞态
	session.AddInteraction(Interaction{
		Type:      "confirmation_request",
		Content:   message,
		Timestamp: time.Now(),
	})
	m.mu.Unlock()

	return conf, nil
}

// RespondToConfirmation 处理用户对确认请求的响应。
func (m *Manager) RespondToConfirmation(confID string, response string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conf, ok := m.confirmations[confID]
	if !ok {
		return fmt.Errorf("confirmation request not found: %s", confID)
	}

	if conf.State != StatePending {
		return fmt.Errorf("confirmation request already resolved")
	}

	if conf.IsExpired() {
		conf.State = StateExpired
		conf.CloseResponseCh()
		delete(m.confirmations, confID)
		return fmt.Errorf("confirmation request expired")
	}

	// 如果提供了选项，验证响应是否匹配
	if len(conf.Options) > 0 {
		valid := false
		for _, opt := range conf.Options {
			if strings.EqualFold(opt.ID, response) || strings.EqualFold(opt.Label, response) {
				valid = true
				conf.Selected = opt.ID
				break
			}
		}
		// 也接受数字响应（1、2、3...）
		if !valid {
			for i, opt := range conf.Options {
				if response == fmt.Sprintf("%d", i+1) {
					valid = true
					conf.Selected = opt.ID
					break
				}
			}
		}
		if !valid && (strings.EqualFold(response, "yes") || strings.EqualFold(response, "y")) {
			valid = true
			conf.Selected = "yes"
		}
		if !valid && (strings.EqualFold(response, "no") || strings.EqualFold(response, "n")) {
			valid = true
			conf.Selected = "no"
		}
		if !valid {
			return fmt.Errorf("invalid option: %s", response)
		}
	} else {
		conf.Selected = response
	}

	conf.State = StateConfirmed

	// 从会话中清除
	session := m.sessions[conf.SessionKey]
	if session != nil {
		session.PendingConfirmation = nil
		session.AddInteraction(Interaction{
			Type:      "confirmation_response",
			Content:   response,
			Timestamp: time.Now(),
		})
	}

	// 通过通道发送响应
	select {
	case conf.ResponseCh <- response:
	default:
	}

	conf.CloseResponseCh()
	delete(m.confirmations, confID)

	return nil
}

// CancelConfirmation 取消一个待处理的确认请求。
func (m *Manager) CancelConfirmation(confID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conf, ok := m.confirmations[confID]
	if !ok {
		return fmt.Errorf("confirmation request not found: %s", confID)
	}

	conf.State = StateCancelled
	conf.CloseResponseCh()

	// 从会话中清除
	session := m.sessions[conf.SessionKey]
	if session != nil {
		session.PendingConfirmation = nil
	}

	delete(m.confirmations, confID)
	return nil
}

// GetPendingConfirmation 返回指定会话的待处理确认请求。
// 如果确认已过期，将被清理并返回 nil。
func (m *Manager) GetPendingConfirmation(sessionKey string) *ConfirmationRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionKey]
	if !ok {
		return nil
	}

	conf := session.PendingConfirmation
	if conf == nil {
		return nil
	}
	if conf.IsExpired() {
		conf.State = StateExpired
		conf.CloseResponseCh()
		session.PendingConfirmation = nil
		delete(m.confirmations, conf.ID)
		return nil
	}
	return conf
}

// CreatePlan 创建一个需要用户批准的执行计划。
func (m *Manager) CreatePlan(
	sessionKey string,
	title string,
	description string,
	steps []PlanStep,
) *ExecutionPlan {
	planID := generatePlanID(sessionKey)

	plan := &ExecutionPlan{
		ID:          planID,
		SessionKey:  sessionKey,
		Title:       title,
		Description: description,
		Steps:       steps,
		State:       PlanPending,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(DefaultTimeout),
	}

	m.mu.Lock()
	m.plans[planID] = plan
	session := m.getOrCreateSessionLocked(sessionKey)
	session.ActivePlan = plan
	// 在锁内添加到会话历史，避免竞态
	session.AddInteraction(Interaction{
		Type:      "plan_created",
		Content:   title,
		Timestamp: time.Now(),
	})
	m.mu.Unlock()

	return plan
}

// ApprovePlan 批准一个执行计划。
func (m *Manager) ApprovePlan(planID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plan, ok := m.plans[planID]
	if !ok {
		return fmt.Errorf("plan not found: %s", planID)
	}

	if plan.State != PlanPending {
		return fmt.Errorf("plan already processed")
	}

	plan.State = PlanApproved
	return nil
}

// RejectPlan 拒绝一个执行计划。
func (m *Manager) RejectPlan(planID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plan, ok := m.plans[planID]
	if !ok {
		return fmt.Errorf("plan not found: %s", planID)
	}

	plan.State = PlanRejected

	// 从会话中清除
	session := m.sessions[plan.SessionKey]
	if session != nil {
		session.ActivePlan = nil
	}

	delete(m.plans, planID)
	return nil
}

// GetActivePlan 返回指定会话的活跃执行计划。
func (m *Manager) GetActivePlan(sessionKey string) *ExecutionPlan {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionKey]
	if !ok {
		return nil
	}

	plan := session.ActivePlan
	if plan != nil && time.Now().After(plan.ExpiresAt) {
		return nil
	}
	return plan
}

// CompletePlan 将执行计划标记为已完成。
func (m *Manager) CompletePlan(planID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plan, ok := m.plans[planID]
	if !ok {
		return fmt.Errorf("plan not found: %s", planID)
	}

	plan.State = PlanCompleted

	// 从会话中清除
	session := m.sessions[plan.SessionKey]
	if session != nil {
		session.ActivePlan = nil
		session.AddInteraction(Interaction{
			Type:      "plan_completed",
			Content:   plan.Title,
			Timestamp: time.Now(),
		})
	}

	delete(m.plans, planID)
	return nil
}

// CleanupExpired 清理已过期的确认请求和执行计划。
func (m *Manager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	for id, conf := range m.confirmations {
		if now.After(conf.ExpiresAt) {
			conf.State = StateExpired
			conf.CloseResponseCh()

			// 从会话中清除
			if session, ok := m.sessions[conf.SessionKey]; ok {
				session.PendingConfirmation = nil
			}

			delete(m.confirmations, id)
		}
	}

	for id, plan := range m.plans {
		if now.After(plan.ExpiresAt) {
			plan.State = PlanExpired

			// 从会话中清除
			if session, ok := m.sessions[plan.SessionKey]; ok {
				session.ActivePlan = nil
			}

			delete(m.plans, id)
		}
	}
}

// 辅助方法

// getOrCreateSessionLocked 获取或创建会话（调用者必须持有锁）。
func (m *Manager) getOrCreateSessionLocked(sessionKey string) *Session {
	if session, ok := m.sessions[sessionKey]; ok {
		return session
	}
	session := NewSession(sessionKey)
	m.sessions[sessionKey] = session
	return session
}

// generateConfirmationID 生成确认请求的唯一 ID。
func generateConfirmationID(sessionKey string) string {
	return fmt.Sprintf("conf_%s_%d", sessionKey, time.Now().UnixNano())
}

// generatePlanID 生成执行计划的唯一 ID。
func generatePlanID(sessionKey string) string {
	return fmt.Sprintf("plan_%s_%d", sessionKey, time.Now().UnixNano())
}

// ModeFromString 将字符串转换为 Mode。
func ModeFromString(s string) Mode {
	switch strings.ToLower(s) {
	case "confirm", "confirmation", "on":
		return ModeConfirm
	case "direct", "off":
		return ModeDirect
	default:
		return ModeAuto
	}
}

// String 返回 Mode 的字符串表示。
func (m Mode) String() string {
	switch m {
	case ModeConfirm:
		return "confirm"
	case ModeDirect:
		return "direct"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}
