// Package interactive 提供会话状态管理和
// AI 代理交互的逐步确认流程。
package interactive

import (
	"context"
	"sync"
	"time"
)

// ConfirmationState 表示待处理用户确认的状态。
type ConfirmationState int

const (
	StatePending   ConfirmationState = iota // 待处理
	StateConfirmed                          // 已确认
	StateCancelled                          // 已取消
	StateExpired                            // 已过期
)

// ConfirmationOption 表示确认对话框中的单个选项。
type ConfirmationOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// ConfirmationRequest 表示一个待处理的用户确认请求。
type ConfirmationRequest struct {
	ID          string               `json:"id"`
	SessionKey  string               `json:"session_key"`
	Message     string               `json:"message"`
	Options     []ConfirmationOption `json:"options,omitempty"`
	State       ConfirmationState    `json:"state"`
	CreatedAt   time.Time            `json:"created_at"`
	ExpiresAt   time.Time            `json:"expires_at"`
	Selected    string               `json:"selected,omitempty"` // 选中的选项 ID
	ResponseCh  chan string          `json:"-"`                  // 接收用户响应的通道
	closed      bool                                             // 防止 ResponseCh 被重复关闭
}

// IsExpired 检查确认请求是否已过期。
func (c *ConfirmationRequest) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// CloseResponseCh 安全地关闭响应通道，防止重复关闭导致 panic。
func (c *ConfirmationRequest) CloseResponseCh() {
	if !c.closed {
		c.closed = true
		close(c.ResponseCh)
	}
}

// PlanStep 表示执行计划中的单个步骤。
type PlanStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"` // pending、running、completed、failed、skipped
}

// ExecutionPlan 表示一个需要用户批准的多步骤执行计划。
type ExecutionPlan struct {
	ID          string     `json:"id"`
	SessionKey  string     `json:"session_key"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Steps       []PlanStep `json:"steps"`
	State       PlanState  `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

// PlanState 表示执行计划的状态。
type PlanState int

const (
	PlanPending   PlanState = iota // 待处理
	PlanApproved                   // 已批准
	PlanRejected                   // 已拒绝
	PlanRunning                    // 执行中
	PlanCompleted                  // 已完成
	PlanFailed                     // 已失败
	PlanCancelled                  // 已取消
	PlanExpired                    // 已过期
)

// Session 管理单个会话的交互状态。
type Session struct {
	SessionKey          string
	Mode                Mode
	PendingConfirmation *ConfirmationRequest
	ActivePlan          *ExecutionPlan
	History             []Interaction
	mu                  sync.RWMutex
}

// Mode 表示交互模式。
type Mode int

const (
	ModeAuto    Mode = iota // 自动模式 - AI 决定何时请求确认
	ModeConfirm             // 确认模式 - 执行前始终请求确认
	ModeDirect              // 直接模式 - 无需确认直接执行
)

// Interaction 表示会话历史中的单次交互。
type Interaction struct {
	Type      string    `json:"type"` // confirmation、plan、response
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// NewSession 创建一个新的交互会话。
func NewSession(sessionKey string) *Session {
	return &Session{
		SessionKey: sessionKey,
		Mode:       ModeAuto,
		History:    make([]Interaction, 0),
	}
}

// SetMode 更改交互模式。
func (s *Session) SetMode(mode Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Mode = mode
}

// GetMode 返回当前的交互模式。
func (s *Session) GetMode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Mode
}

// AddInteraction 将一次交互添加到历史记录。
func (s *Session) AddInteraction(interaction Interaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = append(s.History, interaction)
}

// GetHistory 返回交互历史记录。
func (s *Session) GetHistory() []Interaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Interaction, len(s.History))
	copy(result, s.History)
	return result
}

// Context 为交互操作提供上下文。
type Context struct {
	context.Context
	Session     *Session
	SendMessage func(content string) error
}

// ConfirmationResult 表示确认请求的结果。
type ConfirmationResult struct {
	Confirmed bool
	Selected  string
	Expired   bool
	Error     error
}
