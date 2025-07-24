package queue_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Valentin-Kaiser/go-core/queue"
)

func TestTaskScheduler_RegisterIntervalTask(t *testing.T) {
	scheduler := queue.NewTaskScheduler()

	var executed int32
	taskFunc := func(_ context.Context) error {
		atomic.AddInt32(&executed, 1)
		return nil
	}

	err := scheduler.RegisterIntervalTask("test-task", time.Second, taskFunc)
	if err != nil {
		t.Fatalf("failed to register interval task: %v", err)
	}

	task, err := scheduler.GetTask("test-task")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Name != "test-task" {
		t.Errorf("expected task name 'test-task', got '%s'", task.Name)
	}

	if task.Type != queue.TaskTypeInterval {
		t.Errorf("expected task type interval, got %v", task.Type)
	}

	if task.Interval != time.Second {
		t.Errorf("expected interval 1s, got %v", task.Interval)
	}

	if !task.Enabled {
		t.Error("expected task to be enabled")
	}

	// Test duplicate registration
	err = scheduler.RegisterIntervalTask("test-task", time.Second, taskFunc)
	if err == nil {
		t.Error("expected error when registering duplicate task")
	}
}

func TestTaskScheduler_RegisterCronTask(t *testing.T) {
	scheduler := queue.NewTaskScheduler()

	var executed int32
	taskFunc := func(_ context.Context) error {
		atomic.AddInt32(&executed, 1)
		return nil
	}

	// Test valid cron expression
	err := scheduler.RegisterCronTask("test-cron", "*/5 * * * *", taskFunc)
	if err != nil {
		t.Fatalf("failed to register cron task: %v", err)
	}

	task, err := scheduler.GetTask("test-cron")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Type != queue.TaskTypeCron {
		t.Errorf("expected task type cron, got %v", task.Type)
	}

	if task.CronSpec != "*/5 * * * *" {
		t.Errorf("expected cron spec '*/5 * * * *', got '%s'", task.CronSpec)
	}

	// Test invalid cron expression
	err = scheduler.RegisterCronTask("invalid-cron", "invalid", taskFunc)
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}

	// Test empty name
	err = scheduler.RegisterCronTask("", "*/5 * * * *", taskFunc)
	if err == nil {
		t.Error("expected error for empty task name")
	}

	// Test nil function
	err = scheduler.RegisterCronTask("nil-func", "*/5 * * * *", nil)
	if err == nil {
		t.Error("expected error for nil function")
	}
}

func TestTaskScheduler_StartStop(t *testing.T) {
	scheduler := queue.NewTaskScheduler()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*5)
	defer cancel()

	// Test start
	err := scheduler.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	if !scheduler.IsRunning() {
		t.Error("expected scheduler to be running")
	}

	// Test double start
	err = scheduler.Start(ctx)
	if err == nil {
		t.Error("expected error when starting already running scheduler")
	}

	// Test stop
	scheduler.Stop()

	if scheduler.IsRunning() {
		t.Error("expected scheduler to be stopped")
	}

	// Test double stop (should not panic)
	scheduler.Stop()
}

func TestTaskScheduler_IntervalTaskExecution(t *testing.T) {
	scheduler := queue.NewTaskScheduler().WithCheckInterval(time.Millisecond * 100)

	var executed int32
	var executionTimes []time.Time
	var mu sync.Mutex

	taskFunc := func(_ context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		atomic.AddInt32(&executed, 1)
		executionTimes = append(executionTimes, time.Now())
		return nil
	}

	err := scheduler.RegisterIntervalTask("test-interval", time.Millisecond*300, taskFunc)
	if err != nil {
		t.Fatalf("failed to register interval task: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	err = scheduler.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	// Wait for a few executions
	time.Sleep(time.Second)
	scheduler.Stop()

	execCount := atomic.LoadInt32(&executed)
	if execCount < 2 {
		t.Errorf("expected at least 2 executions, got %d", execCount)
	}

	// Check that task was executed at reasonable intervals
	mu.Lock()
	defer mu.Unlock()

	if len(executionTimes) < 2 {
		t.Skip("not enough executions to test intervals")
	}

	for i := 1; i < len(executionTimes); i++ {
		interval := executionTimes[i].Sub(executionTimes[i-1])
		if interval < time.Millisecond*200 || interval > time.Millisecond*500 {
			t.Errorf("unexpected interval between executions: %v", interval)
		}
	}
}

func TestTaskScheduler_TaskRetry(t *testing.T) {
	scheduler := queue.NewTaskScheduler().
		WithCheckInterval(time.Millisecond * 50).
		WithRetryDelay(time.Millisecond * 50)

	var attempts int32
	var successCount int32
	var lastError error

	taskFunc := func(_ context.Context) error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			lastError = fmt.Errorf("attempt %d failed", count)
			return lastError
		}
		atomic.AddInt32(&successCount, 1)
		return nil
	}

	err := scheduler.RegisterIntervalTaskWithOptions("retry-task", time.Second*10, taskFunc, queue.TaskOptions{
		MaxRetries: 3,
		RetryDelay: time.Millisecond * 30,
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	err = scheduler.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	// Wait for task to be executed and retried
	time.Sleep(time.Millisecond * 300)
	scheduler.Stop()

	// The task should execute and succeed after 3 attempts
	// It should only succeed once in the time window since interval is 10 seconds
	attemptCount := atomic.LoadInt32(&attempts)
	if attemptCount < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attemptCount)
	}

	successCountValue := atomic.LoadInt32(&successCount)
	if successCountValue < 1 {
		t.Errorf("expected at least 1 success, got %d", successCountValue)
	}

	// Check task stats
	task, err := scheduler.GetTask("retry-task")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.RunCount < 1 {
		t.Errorf("expected run count at least 1, got %d", task.RunCount)
	}

	if task.ErrorCount != 0 {
		t.Errorf("expected error count 0, got %d", task.ErrorCount)
	}

	if task.LastError != "" {
		t.Errorf("expected no last error, got '%s'", task.LastError)
	}
}

func TestTaskScheduler_TaskFailure(t *testing.T) {
	scheduler := queue.NewTaskScheduler().
		WithCheckInterval(time.Millisecond * 50).
		WithRetryDelay(time.Millisecond * 30)

	var attempts int32
	expectedError := errors.New("task always fails")

	taskFunc := func(_ context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return expectedError
	}

	err := scheduler.RegisterIntervalTaskWithOptions("failing-task", time.Second*5, taskFunc, queue.TaskOptions{
		MaxRetries: 2,
		RetryDelay: time.Millisecond * 30,
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	err = scheduler.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	// Wait for task to fail completely
	time.Sleep(time.Millisecond * 200)
	scheduler.Stop()

	attemptCount := atomic.LoadInt32(&attempts)
	if attemptCount < 3 { // At least 1 initial + 2 retries
		t.Errorf("expected at least 3 attempts, got %d", attemptCount)
	}

	// Check task stats
	task, err := scheduler.GetTask("failing-task")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.RunCount != 0 {
		t.Errorf("expected run count 0, got %d", task.RunCount)
	}

	if task.ErrorCount < 1 {
		t.Errorf("expected error count at least 1, got %d", task.ErrorCount)
	}

	if task.LastError != expectedError.Error() {
		t.Errorf("expected last error '%s', got '%s'", expectedError.Error(), task.LastError)
	}
}

func TestTaskScheduler_EnableDisableTask(t *testing.T) {
	scheduler := queue.NewTaskScheduler()

	taskFunc := func(_ context.Context) error {
		return nil
	}

	err := scheduler.RegisterIntervalTask("toggle-task", time.Second, taskFunc)
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Test disable
	err = scheduler.DisableTask("toggle-task")
	if err != nil {
		t.Fatalf("failed to disable task: %v", err)
	}

	task, err := scheduler.GetTask("toggle-task")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Enabled {
		t.Error("expected task to be disabled")
	}

	// Test enable
	err = scheduler.EnableTask("toggle-task")
	if err != nil {
		t.Fatalf("failed to enable task: %v", err)
	}

	task, err = scheduler.GetTask("toggle-task")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if !task.Enabled {
		t.Error("expected task to be enabled")
	}

	// Test non-existent task
	err = scheduler.EnableTask("non-existent")
	if err == nil {
		t.Error("expected error when enabling non-existent task")
	}

	err = scheduler.DisableTask("non-existent")
	if err == nil {
		t.Error("expected error when disabling non-existent task")
	}
}

func TestTaskScheduler_RemoveTask(t *testing.T) {
	scheduler := queue.NewTaskScheduler()

	taskFunc := func(_ context.Context) error {
		return nil
	}

	err := scheduler.RegisterIntervalTask("remove-task", time.Second, taskFunc)
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Test removal
	err = scheduler.RemoveTask("remove-task")
	if err != nil {
		t.Fatalf("failed to remove task: %v", err)
	}

	// Check that task is gone
	_, err = scheduler.GetTask("remove-task")
	if err == nil {
		t.Error("expected error when getting removed task")
	}

	// Test removing non-existent task
	err = scheduler.RemoveTask("non-existent")
	if err == nil {
		t.Error("expected error when removing non-existent task")
	}
}

func TestTaskScheduler_GetTasks(t *testing.T) {
	scheduler := queue.NewTaskScheduler()

	taskFunc := func(_ context.Context) error {
		return nil
	}

	// Register multiple tasks
	err := scheduler.RegisterIntervalTask("task1", time.Second, taskFunc)
	if err != nil {
		t.Fatalf("failed to register task1: %v", err)
	}

	err = scheduler.RegisterIntervalTask("task2", time.Second*2, taskFunc)
	if err != nil {
		t.Fatalf("failed to register task2: %v", err)
	}

	err = scheduler.RegisterCronTask("task3", "0 0 * * *", taskFunc)
	if err != nil {
		t.Fatalf("failed to register task3: %v", err)
	}

	tasks := scheduler.GetTasks()

	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	expectedTasks := []string{"task1", "task2", "task3"}
	for _, name := range expectedTasks {
		if _, exists := tasks[name]; !exists {
			t.Errorf("expected task '%s' to exist", name)
		}
	}

	// Check that returned tasks are copies (not references)
	originalTask := tasks["task1"]
	originalTask.Enabled = false

	task, err := scheduler.GetTask("task1")
	if err != nil {
		t.Fatalf("failed to get task1: %v", err)
	}

	if !task.Enabled {
		t.Error("modifying returned task affected original task")
	}
}
