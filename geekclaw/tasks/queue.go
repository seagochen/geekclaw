// Package tasks 提供 AI 代理操作的任务队列管理。
// 支持跟踪活跃任务、取消任务和管理任务生命周期。
package tasks

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Task 表示一个活跃的 AI 处理任务。
type Task struct {
	ID        string        // 唯一任务标识符（例如会话键）
	SessionKey string       // 任务的会话键
	StartTime time.Time     // 任务启动时间
	ctx       context.Context    // 任务特定的上下文（派生自父上下文）
	cancel    context.CancelFunc // 停止任务的取消函数
}

// Queue 管理活跃的 AI 任务。
// 提供线程安全的任务跟踪和取消操作。
type Queue struct {
	mu     sync.RWMutex
	tasks  map[string]*Task // 按 ID 索引的活跃任务
	order  []string         // 按创建时间排序的任务 ID（最早的在前）
}

// NewQueue 创建一个新的任务队列。
func NewQueue() *Queue {
	return &Queue{
		tasks: make(map[string]*Task),
		order: make([]string, 0),
	}
}

// Start 创建一个新任务并添加到队列中。
// 返回的上下文应用于与该任务相关的所有操作。
// 如果已存在相同 ID 的任务，将先取消它。
func (q *Queue) Start(parentCtx context.Context, id, sessionKey string) (context.Context, *Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 如果存在相同 ID 的任务则取消
	if existing, ok := q.tasks[id]; ok {
		existing.cancel()
		q.removeTaskInternal(id)
	}

	// 创建带取消功能的新任务上下文
	ctx, cancel := context.WithCancel(parentCtx)
	task := &Task{
		ID:         id,
		SessionKey: sessionKey,
		StartTime:  time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}

	q.tasks[id] = task
	q.order = append(q.order, id)

	return ctx, task
}

// Get 通过 ID 获取任务。
func (q *Queue) Get(id string) (*Task, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	task, ok := q.tasks[id]
	return task, ok
}

// GetLatest 返回最近启动的任务。
func (q *Queue) GetLatest() (*Task, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.order) == 0 {
		return nil, false
	}

	latestID := q.order[len(q.order)-1]
	task, ok := q.tasks[latestID]
	return task, ok
}

// GetLatestBySession 返回给定会话键的最近任务。
func (q *Queue) GetLatestBySession(sessionKey string) (*Task, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// 反向遍历以找到最近匹配的任务
	for i := len(q.order) - 1; i >= 0; i-- {
		id := q.order[i]
		if task, ok := q.tasks[id]; ok && task.SessionKey == sessionKey {
			return task, true
		}
	}

	return nil, false
}

// Stop 通过 ID 取消任务并从队列中移除。
// 如果找到并取消了任务，返回 true。
func (q *Queue) Stop(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.tasks[id]
	if !ok {
		return false
	}

	task.cancel()
	q.removeTaskInternal(id)
	return true
}

// StopLatest 取消最近的任务。
// 返回被停止的任务和是否找到任务。
func (q *Queue) StopLatest() (*Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.order) == 0 {
		return nil, false
	}

	latestID := q.order[len(q.order)-1]
	task, ok := q.tasks[latestID]
	if !ok {
		return nil, false
	}

	task.cancel()
	q.removeTaskInternal(latestID)
	return task, true
}

// StopLatestBySession 取消给定会话的最近任务。
// 返回被停止的任务和是否找到任务。
func (q *Queue) StopLatestBySession(sessionKey string) (*Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 查找该会话的最近任务
	var targetID string
	for i := len(q.order) - 1; i >= 0; i-- {
		id := q.order[i]
		if task, ok := q.tasks[id]; ok && task.SessionKey == sessionKey {
			targetID = id
			break
		}
	}

	if targetID == "" {
		return nil, false
	}

	task, ok := q.tasks[targetID]
	if !ok {
		return nil, false
	}

	task.cancel()
	q.removeTaskInternal(targetID)
	return task, true
}

// Finish 将任务标记为完成并从队列中移除。
// 当任务正常完成（非取消）时应调用此方法。
func (q *Queue) Finish(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task, ok := q.tasks[id]; ok {
		// 取消上下文以确保清理，但不传播错误
		task.cancel()
		q.removeTaskInternal(id)
	}
}

// List 返回所有活跃任务的快照。
func (q *Queue) List() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*Task, 0, len(q.order))
	for _, id := range q.order {
		if task, ok := q.tasks[id]; ok {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

// Count 返回活跃任务的数量。
func (q *Queue) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tasks)
}

// IsActive 检查任务是否当前活跃。
func (q *Queue) IsActive(id string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	_, ok := q.tasks[id]
	return ok
}

// removeTaskInternal 从内部数据结构中移除任务。
// 必须在持有锁的情况下调用。
func (q *Queue) removeTaskInternal(id string) {
	delete(q.tasks, id)

	// 从顺序切片中移除
	newOrder := make([]string, 0, len(q.order)-1)
	for _, taskID := range q.order {
		if taskID != id {
			newOrder = append(newOrder, taskID)
		}
	}
	q.order = newOrder
}

// TaskInfo 提供任务信息的只读视图。
type TaskInfo struct {
	ID         string    `json:"id"`
	SessionKey string    `json:"session_key"`
	StartTime  time.Time `json:"start_time"`
	Duration   time.Duration `json:"duration_ms"`
}

// ToInfo 将 Task 转换为 TaskInfo。
func (t *Task) ToInfo() TaskInfo {
	return TaskInfo{
		ID:         t.ID,
		SessionKey: t.SessionKey,
		StartTime:  t.StartTime,
		Duration:   time.Since(t.StartTime),
	}
}

// Context 返回任务的上下文。
func (t *Task) Context() context.Context {
	return t.ctx
}

// String 返回任务的字符串表示。
func (t *Task) String() string {
	return fmt.Sprintf("Task[%s] session=%s started=%v elapsed=%v",
		t.ID, t.SessionKey, t.StartTime.Format("15:04:05"), time.Since(t.StartTime).Round(time.Second))
}
