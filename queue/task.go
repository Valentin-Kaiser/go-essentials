// Package queue provides background task execution capabilities.
//
// This package extends the queue system with background task support for:
//   - Cron-based scheduling (using cron expressions)
//   - Interval-based scheduling (using time.Duration)
//   - Task registration and management
//   - Graceful shutdown handling
//   - Error recovery and retries
//   - Context-aware execution
//
// Example usage:
//
//	package main
//
//	import (
//		"context"
//		"fmt"
//		"time"
//
//		"github.com/Valentin-Kaiser/go-core/queue"
//	)
//
//	func main() {
//		// Create a new task scheduler
//		scheduler := queue.NewTaskScheduler()
//
//		// Register a cron-based task
//		scheduler.RegisterCronTask("cleanup", "0 0 * * *", func(ctx context.Context) error {
//			fmt.Println("Running daily cleanup task")
//			return nil
//		})
//
//		// Register an interval-based task
//		scheduler.RegisterIntervalTask("health-check", time.Minute*5, func(ctx context.Context) error {
//			fmt.Println("Running health check")
//			return nil
//		})
//
//		// Start the scheduler
//		if err := scheduler.Start(context.Background()); err != nil {
//			panic(err)
//		}
//		defer scheduler.Stop()
//
//		// Keep the program running
//		select {}
//	}
package queue

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/interruption"
	"github.com/rs/zerolog/log"
)

// TaskFunc represents a task function that can be executed
type TaskFunc func(ctx context.Context) error

// TaskType represents the type of task scheduling
type TaskType int

const (
	TaskTypeCron TaskType = iota
	TaskTypeInterval
)

// String returns the string representation of the task type
func (t TaskType) String() string {
	switch t {
	case TaskTypeCron:
		return "cron"
	case TaskTypeInterval:
		return "interval"
	default:
		return "unknown"
	}
}

// Task represents a scheduled task
type Task struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Type       TaskType      `json:"type"`
	CronSpec   string        `json:"cron_spec,omitempty"`
	Interval   time.Duration `json:"interval,omitempty"`
	Function   TaskFunc      `json:"-"`
	NextRun    time.Time     `json:"next_run"`
	LastRun    time.Time     `json:"last_run"`
	RunCount   int64         `json:"run_count"`
	ErrorCount int64         `json:"error_count"`
	LastError  string        `json:"last_error,omitempty"`
	IsRunning  bool          `json:"is_running"`
	MaxRetries int           `json:"max_retries"`
	RetryDelay time.Duration `json:"retry_delay"`
	Timeout    time.Duration `json:"timeout"`
	Enabled    bool          `json:"enabled"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// TaskScheduler manages background tasks
type TaskScheduler struct {
	tasks          map[string]*Task
	tasksMutex     sync.RWMutex
	running        int32
	shutdownChan   chan struct{}
	workerWg       sync.WaitGroup
	checkInterval  time.Duration
	defaultTimeout time.Duration
	defaultRetries int
	retryDelay     time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewTaskScheduler creates a new task scheduler with default settings
func NewTaskScheduler() *TaskScheduler {
	return &TaskScheduler{
		tasks:          make(map[string]*Task),
		shutdownChan:   make(chan struct{}),
		checkInterval:  time.Second * 10,
		defaultTimeout: time.Minute * 5,
		defaultRetries: 3,
		retryDelay:     time.Second * 5,
	}
}

// WithCheckInterval sets the interval for checking scheduled tasks
func (s *TaskScheduler) WithCheckInterval(interval time.Duration) *TaskScheduler {
	if interval > 0 {
		s.checkInterval = interval
	}
	return s
}

// WithDefaultTimeout sets the default timeout for task execution
func (s *TaskScheduler) WithDefaultTimeout(timeout time.Duration) *TaskScheduler {
	if timeout > 0 {
		s.defaultTimeout = timeout
	}
	return s
}

// WithDefaultRetries sets the default number of retries for failed tasks
func (s *TaskScheduler) WithDefaultRetries(retries int) *TaskScheduler {
	if retries >= 0 {
		s.defaultRetries = retries
	}
	return s
}

// WithRetryDelay sets the delay between retries
func (s *TaskScheduler) WithRetryDelay(delay time.Duration) *TaskScheduler {
	if delay > 0 {
		s.retryDelay = delay
	}
	return s
}

// RegisterCronTask registers a new cron-based task
func (s *TaskScheduler) RegisterCronTask(name, cronSpec string, fn TaskFunc) error {
	return s.RegisterCronTaskWithOptions(name, cronSpec, fn, TaskOptions{})
}

// RegisterIntervalTask registers a new interval-based task
func (s *TaskScheduler) RegisterIntervalTask(name string, interval time.Duration, fn TaskFunc) error {
	return s.RegisterIntervalTaskWithOptions(name, interval, fn, TaskOptions{})
}

// TaskOptions provides configuration options for tasks
type TaskOptions struct {
	MaxRetries int
	RetryDelay time.Duration
	Timeout    time.Duration
	Enabled    bool
}

// RegisterCronTaskWithOptions registers a new cron-based task with options
func (s *TaskScheduler) RegisterCronTaskWithOptions(name, cronSpec string, fn TaskFunc, options TaskOptions) error {
	if name == "" {
		return apperror.NewError("task name cannot be empty")
	}
	if cronSpec == "" {
		return apperror.NewError("cron specification cannot be empty")
	}
	if fn == nil {
		return apperror.NewError("task function cannot be nil")
	}

	if err := s.validateCronSpec(cronSpec); err != nil {
		return apperror.NewError(fmt.Sprintf("invalid cron specification: %v", err))
	}

	s.tasksMutex.Lock()
	defer s.tasksMutex.Unlock()
	if _, exists := s.tasks[name]; exists {
		return apperror.NewError(fmt.Sprintf("task with name '%s' already exists", name))
	}

	maxRetries := s.defaultRetries
	if options.MaxRetries > 0 {
		maxRetries = options.MaxRetries
	}

	retryDelay := s.retryDelay
	if options.RetryDelay > 0 {
		retryDelay = options.RetryDelay
	}

	timeout := s.defaultTimeout
	if options.Timeout > 0 {
		timeout = options.Timeout
	}

	enabled := true
	if options.Enabled {
		enabled = options.Enabled
	}

	task := &Task{
		ID:         generateTaskID(),
		Name:       name,
		Type:       TaskTypeCron,
		CronSpec:   cronSpec,
		Function:   fn,
		MaxRetries: maxRetries,
		RetryDelay: retryDelay,
		Timeout:    timeout,
		Enabled:    enabled,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	nextRun, err := s.calculateNextCronRun(cronSpec, time.Now())
	if err != nil {
		return apperror.NewError(fmt.Sprintf("failed to calculate next run time: %v", err))
	}

	task.NextRun = nextRun
	s.tasks[name] = task

	log.Info().
		Str("task_name", name).
		Str("cron_spec", cronSpec).
		Time("next_run", nextRun).
		Msg("Cron task registered")

	return nil
}

// RegisterIntervalTaskWithOptions registers a new interval-based task with options
func (s *TaskScheduler) RegisterIntervalTaskWithOptions(name string, interval time.Duration, fn TaskFunc, options TaskOptions) error {
	if name == "" {
		return apperror.NewError("task name cannot be empty")
	}
	if interval <= 0 {
		return apperror.NewError("interval must be positive")
	}
	if fn == nil {
		return apperror.NewError("task function cannot be nil")
	}

	s.tasksMutex.Lock()
	defer s.tasksMutex.Unlock()

	if _, exists := s.tasks[name]; exists {
		return apperror.NewError(fmt.Sprintf("task with name '%s' already exists", name))
	}

	maxRetries := s.defaultRetries
	if options.MaxRetries > 0 {
		maxRetries = options.MaxRetries
	}

	retryDelay := s.retryDelay
	if options.RetryDelay > 0 {
		retryDelay = options.RetryDelay
	}

	timeout := s.defaultTimeout
	if options.Timeout > 0 {
		timeout = options.Timeout
	}

	enabled := true
	if options.Enabled {
		enabled = options.Enabled
	}

	task := &Task{
		ID:         generateTaskID(),
		Name:       name,
		Type:       TaskTypeInterval,
		Interval:   interval,
		Function:   fn,
		NextRun:    time.Now(), // Run immediately the first time
		MaxRetries: maxRetries,
		RetryDelay: retryDelay,
		Timeout:    timeout,
		Enabled:    enabled,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	s.tasks[name] = task

	log.Info().
		Str("task_name", name).
		Dur("interval", interval).
		Time("next_run", task.NextRun).
		Msg("Interval task registered")

	return nil
}

// Start starts the task scheduler
func (s *TaskScheduler) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return apperror.NewError("task scheduler is already running")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	s.workerWg.Add(1)
	go s.schedulerLoop()

	log.Info().Msg("Task scheduler started")
	go func() {
		err := interruption.OnSignal(func() error {
			log.Info().Msg("Received interruption signal, stopping task scheduler...")
			s.Stop()
			return nil
		}, os.Interrupt, syscall.SIGTERM)
		if err != nil {
			log.Error().Err(err).Msg("Failed to register signal handler")
		}
	}()

	return nil
}

// Stop stops the task scheduler
func (s *TaskScheduler) Stop() {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return
	}

	log.Info().Msg("Stopping task scheduler...")
	if s.cancel != nil {
		s.cancel()
	}

	close(s.shutdownChan)
	s.workerWg.Wait()

	log.Info().Msg("Task scheduler stopped")
}

// schedulerLoop is the main scheduler loop
func (s *TaskScheduler) schedulerLoop() {
	defer s.workerWg.Done()

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownChan:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRunTasks()
		}
	}
}

// checkAndRunTasks checks for tasks that need to be executed and runs them
func (s *TaskScheduler) checkAndRunTasks() {
	s.tasksMutex.RLock()
	var tasksToRun []*Task
	now := time.Now()

	for _, task := range s.tasks {
		if task.Enabled && !task.IsRunning && now.After(task.NextRun) {
			tasksToRun = append(tasksToRun, task)
		}
	}
	s.tasksMutex.RUnlock()

	for _, task := range tasksToRun {
		s.workerWg.Add(1)
		go s.runTask(task)
	}
}

// runTask executes a single task
func (s *TaskScheduler) runTask(task *Task) {
	defer s.workerWg.Done()

	s.tasksMutex.Lock()
	task.IsRunning = true
	task.UpdatedAt = time.Now()
	s.tasksMutex.Unlock()

	taskCtx, cancel := context.WithTimeout(s.ctx, task.Timeout)
	defer cancel()

	var lastError error
	for attempt := 0; attempt <= task.MaxRetries; attempt++ {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		log.Debug().
			Str("task_name", task.Name).
			Int("attempt", attempt+1).
			Int("max_retries", task.MaxRetries+1).
			Msg("Executing task")

		err := task.Function(taskCtx)

		if err == nil {
			s.tasksMutex.Lock()
			task.IsRunning = false
			task.LastRun = time.Now()
			task.RunCount++
			task.LastError = ""
			task.UpdatedAt = time.Now()

			if task.Type == TaskTypeCron {
				nextRun, calcErr := s.calculateNextCronRun(task.CronSpec, time.Now())
				if calcErr != nil {
					log.Error().
						Err(calcErr).
						Str("task_name", task.Name).
						Msg("Failed to calculate next cron run time")
				} else {
					task.NextRun = nextRun
				}
			} else {
				task.NextRun = time.Now().Add(task.Interval)
			}
			s.tasksMutex.Unlock()

			log.Info().
				Str("task_name", task.Name).
				Int64("run_count", task.RunCount).
				Time("next_run", task.NextRun).
				Msg("Task executed successfully")
			return
		}

		lastError = err
		log.Warn().
			Err(err).
			Str("task_name", task.Name).
			Int("attempt", attempt+1).
			Msg("Task execution failed")

		if attempt < task.MaxRetries {
			select {
			case <-taskCtx.Done():
				return
			case <-time.After(task.RetryDelay):
			}
		}
	}

	s.tasksMutex.Lock()
	task.IsRunning = false
	task.LastRun = time.Now()
	task.ErrorCount++
	task.LastError = lastError.Error()
	task.UpdatedAt = time.Now()

	if task.Type == TaskTypeCron {
		nextRun, calcErr := s.calculateNextCronRun(task.CronSpec, time.Now())
		if calcErr != nil {
			log.Error().
				Err(calcErr).
				Str("task_name", task.Name).
				Msg("Failed to calculate next cron run time")
		} else {
			task.NextRun = nextRun
		}
	} else {
		task.NextRun = time.Now().Add(task.Interval)
	}
	s.tasksMutex.Unlock()

	log.Error().
		Err(lastError).
		Str("task_name", task.Name).
		Int64("error_count", task.ErrorCount).
		Time("next_run", task.NextRun).
		Msg("Task execution failed after all retries")
}

// GetTask returns a task by name
func (s *TaskScheduler) GetTask(name string) (*Task, error) {
	s.tasksMutex.RLock()
	defer s.tasksMutex.RUnlock()

	task, exists := s.tasks[name]
	if !exists {
		return nil, apperror.NewError(fmt.Sprintf("task '%s' not found", name))
	}

	taskCopy := *task
	return &taskCopy, nil
}

// GetTasks returns all registered tasks
func (s *TaskScheduler) GetTasks() map[string]*Task {
	s.tasksMutex.RLock()
	defer s.tasksMutex.RUnlock()

	tasks := make(map[string]*Task, len(s.tasks))
	for name, task := range s.tasks {
		taskCopy := *task
		tasks[name] = &taskCopy
	}

	return tasks
}

// EnableTask enables a task
func (s *TaskScheduler) EnableTask(name string) error {
	s.tasksMutex.Lock()
	defer s.tasksMutex.Unlock()

	task, exists := s.tasks[name]
	if !exists {
		return apperror.NewError(fmt.Sprintf("task '%s' not found", name))
	}

	task.Enabled = true
	task.UpdatedAt = time.Now()

	log.Info().
		Str("task_name", name).
		Msg("Task enabled")

	return nil
}

// DisableTask disables a task
func (s *TaskScheduler) DisableTask(name string) error {
	s.tasksMutex.Lock()
	defer s.tasksMutex.Unlock()

	task, exists := s.tasks[name]
	if !exists {
		return apperror.NewError(fmt.Sprintf("task '%s' not found", name))
	}

	task.Enabled = false
	task.UpdatedAt = time.Now()

	log.Info().
		Str("task_name", name).
		Msg("Task disabled")

	return nil
}

// RemoveTask removes a task from the scheduler
func (s *TaskScheduler) RemoveTask(name string) error {
	s.tasksMutex.Lock()
	defer s.tasksMutex.Unlock()

	task, exists := s.tasks[name]
	if !exists {
		return apperror.NewError(fmt.Sprintf("task '%s' not found", name))
	}

	if task.IsRunning {
		return apperror.NewError(fmt.Sprintf("cannot remove running task '%s'", name))
	}

	delete(s.tasks, name)

	log.Info().
		Str("task_name", name).
		Msg("Task removed")

	return nil
}

// IsRunning returns true if the scheduler is running
func (s *TaskScheduler) IsRunning() bool {
	return atomic.LoadInt32(&s.running) == 1
}

// generateTaskID generates a unique task ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}
