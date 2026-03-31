package tasks

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewQueue(t *testing.T) {
	q := NewQueue()
	if q == nil {
		t.Fatal("NewQueue returned nil")
	}
	if q.Count() != 0 {
		t.Errorf("Expected empty queue, got %d tasks", q.Count())
	}
}

func TestQueue_StartAndFinish(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	// Start a task
	taskCtx, task := q.Start(ctx, "task-1", "session-1")
	if task == nil {
		t.Fatal("Start returned nil task")
	}
	if task.ID != "task-1" {
		t.Errorf("Expected task ID 'task-1', got '%s'", task.ID)
	}
	if task.SessionKey != "session-1" {
		t.Errorf("Expected session key 'session-1', got '%s'", task.SessionKey)
	}
	if q.Count() != 1 {
		t.Errorf("Expected 1 task in queue, got %d", q.Count())
	}

	// Check context is valid
	if taskCtx.Err() != nil {
		t.Errorf("Task context should not be canceled, got: %v", taskCtx.Err())
	}

	// Finish the task
	q.Finish("task-1")
	if q.Count() != 0 {
		t.Errorf("Expected 0 tasks after finish, got %d", q.Count())
	}
}

func TestQueue_Stop(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	taskCtx, _ := q.Start(ctx, "task-1", "session-1")

	// Stop the task
	stopped := q.Stop("task-1")
	if !stopped {
		t.Error("Expected Stop to return true")
	}

	// Check context is canceled
	select {
	case <-taskCtx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Task context should be canceled after Stop")
	}

	if q.Count() != 0 {
		t.Errorf("Expected 0 tasks after stop, got %d", q.Count())
	}

	// Stopping non-existent task should return false
	stopped = q.Stop("non-existent")
	if stopped {
		t.Error("Expected Stop on non-existent task to return false")
	}
}

func TestQueue_GetLatest(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	// Queue should be empty initially
	_, ok := q.GetLatest()
	if ok {
		t.Error("Expected GetLatest to return false for empty queue")
	}

	// Add tasks
	q.Start(ctx, "task-1", "session-1")
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	q.Start(ctx, "task-2", "session-1")

	latest, ok := q.GetLatest()
	if !ok {
		t.Fatal("Expected GetLatest to return true")
	}
	if latest.ID != "task-2" {
		t.Errorf("Expected latest task to be 'task-2', got '%s'", latest.ID)
	}

	// Stop latest
	q.StopLatest()

	latest, ok = q.GetLatest()
	if !ok {
		t.Fatal("Expected GetLatest to return true after stopping one")
	}
	if latest.ID != "task-1" {
		t.Errorf("Expected latest task to now be 'task-1', got '%s'", latest.ID)
	}
}

func TestQueue_StopLatest(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	// Empty queue
	_, ok := q.StopLatest()
	if ok {
		t.Error("Expected StopLatest to return false for empty queue")
	}

	// Add and stop
	q.Start(ctx, "task-1", "session-1")
	time.Sleep(10 * time.Millisecond)
	_, task2 := q.Start(ctx, "task-2", "session-1")

	stopped, ok := q.StopLatest()
	if !ok {
		t.Fatal("Expected StopLatest to return true")
	}
	if stopped.ID != "task-2" {
		t.Errorf("Expected to stop 'task-2', stopped '%s'", stopped.ID)
	}

	// Check task-2 context is canceled
	select {
	case <-task2.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Task-2 context should be canceled")
	}

	if q.Count() != 1 {
		t.Errorf("Expected 1 task remaining, got %d", q.Count())
	}
}

func TestQueue_GetLatestBySession(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	q.Start(ctx, "task-1", "session-a")
	time.Sleep(10 * time.Millisecond)
	q.Start(ctx, "task-2", "session-b")
	time.Sleep(10 * time.Millisecond)
	q.Start(ctx, "task-3", "session-a")

	// Get latest for session-a
	task, ok := q.GetLatestBySession("session-a")
	if !ok {
		t.Fatal("Expected to find task for session-a")
	}
	if task.ID != "task-3" {
		t.Errorf("Expected 'task-3' for session-a, got '%s'", task.ID)
	}

	// Get latest for session-b
	task, ok = q.GetLatestBySession("session-b")
	if !ok {
		t.Fatal("Expected to find task for session-b")
	}
	if task.ID != "task-2" {
		t.Errorf("Expected 'task-2' for session-b, got '%s'", task.ID)
	}

	// Non-existent session
	_, ok = q.GetLatestBySession("session-c")
	if ok {
		t.Error("Expected no task for non-existent session")
	}
}

func TestQueue_StopLatestBySession(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	q.Start(ctx, "task-1", "session-a")
	time.Sleep(10 * time.Millisecond)
	q.Start(ctx, "task-2", "session-b")
	time.Sleep(10 * time.Millisecond)
	q.Start(ctx, "task-3", "session-a")

	// Stop latest for session-a
	stopped, ok := q.StopLatestBySession("session-a")
	if !ok {
		t.Fatal("Expected to stop task for session-a")
	}
	if stopped.ID != "task-3" {
		t.Errorf("Expected to stop 'task-3', got '%s'", stopped.ID)
	}

	if q.Count() != 2 {
		t.Errorf("Expected 2 tasks remaining, got %d", q.Count())
	}

	// Non-existent session
	_, ok = q.StopLatestBySession("session-c")
	if ok {
		t.Error("Expected false for non-existent session")
	}
}

func TestQueue_List(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	q.Start(ctx, "task-1", "session-1")
	time.Sleep(10 * time.Millisecond)
	q.Start(ctx, "task-2", "session-1")

	tasks := q.List()
	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks in list, got %d", len(tasks))
	}

	// Check order (oldest first)
	if tasks[0].ID != "task-1" || tasks[1].ID != "task-2" {
		t.Error("Expected tasks in order of creation")
	}
}

func TestQueue_StartReplacesExisting(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	taskCtx1, _ := q.Start(ctx, "task-1", "session-1")
	time.Sleep(10 * time.Millisecond)
	taskCtx2, task2 := q.Start(ctx, "task-1", "session-1") // Same ID

	// First context should be canceled
	select {
	case <-taskCtx1.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("First task context should be canceled")
	}

	// Second context should be active
	if taskCtx2.Err() != nil {
		t.Error("Second task context should be active")
	}

	if task2.StartTime.IsZero() {
		t.Error("New task should have non-zero start time")
	}

	if q.Count() != 1 {
		t.Errorf("Expected 1 task, got %d", q.Count())
	}
}

func TestQueue_ConcurrentAccess(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	// Concurrent starts
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			q.Start(ctx, fmt.Sprintf("task-%d", id), "session-1")
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for goroutines")
		}
	}

	if q.Count() != 10 {
		t.Errorf("Expected 10 tasks, got %d", q.Count())
	}

	// Concurrent stops
	for i := 0; i < 10; i++ {
		go func(id int) {
			q.Stop(fmt.Sprintf("task-%d", id))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for goroutines")
		}
	}

	if q.Count() != 0 {
		t.Errorf("Expected 0 tasks after stop, got %d", q.Count())
	}
}

func TestTask_ToInfo(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	_, task := q.Start(ctx, "task-1", "session-1")
	info := task.ToInfo()

	if info.ID != "task-1" {
		t.Errorf("Expected ID 'task-1', got '%s'", info.ID)
	}
	if info.SessionKey != "session-1" {
		t.Errorf("Expected session key 'session-1', got '%s'", info.SessionKey)
	}
	if info.StartTime.IsZero() {
		t.Error("Expected non-zero start time")
	}
	if info.Duration < 0 {
		t.Error("Expected non-negative duration")
	}
}

func TestTask_Context(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	taskCtx, task := q.Start(ctx, "task-1", "session-1")

	// Task.Context() should return the same context
	if task.Context() != taskCtx {
		t.Error("Task.Context() should return the task's context")
	}
}

