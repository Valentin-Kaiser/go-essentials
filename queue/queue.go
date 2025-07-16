// Package queue provides a comprehensive task queue and background job processing system.
//
// It supports in-memory and Redis-backed queues with features like:
//   - Priority-based job scheduling
//   - Worker pool management
//   - Retry mechanisms with exponential backoff
//   - Middleware support for logging, metrics, and recovery
//   - Batch processing capabilities
//   - Graceful shutdown handling
//   - Job progress tracking
//   - Comprehensive error handling
package queue

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/interruption"
	"github.com/rs/zerolog/log"
)

// Queue defines the interface for job queues
type Queue interface {
	Enqueue(ctx context.Context, job *Job) error
	Dequeue(ctx context.Context, timeout time.Duration) (*Job, error)
	Schedule(ctx context.Context, job *Job) error
	UpdateJob(ctx context.Context, job *Job) error
	GetJob(ctx context.Context, id string) (*Job, error)
	GetJobs(ctx context.Context, status Status, limit int) ([]*Job, error)
	GetStats(ctx context.Context) (*Stats, error)
	DeleteJob(ctx context.Context, id string) error
	Close() error
}

// Stats represents queue statistics
type Stats struct {
	JobsProcessed int64 `json:"jobs_processed"`
	JobsFailed    int64 `json:"jobs_failed"`
	JobsRetried   int64 `json:"jobs_retried"`
	QueueSize     int64 `json:"queue_size"`
	WorkersActive int64 `json:"workers_active"`
	WorkersBusy   int64 `json:"workers_busy"`
	TotalJobs     int64 `json:"total_jobs"`
	Pending       int64 `json:"pending"`
	Running       int64 `json:"running"`
	Completed     int64 `json:"completed"`
	Failed        int64 `json:"failed"`
	Retrying      int64 `json:"retrying"`
	Scheduled     int64 `json:"scheduled"`
	DeadLetter    int64 `json:"dead_letter"`
}

// Manager manages the job queue and workers
type Manager struct {
	queue            Queue
	handlers         map[string]JobHandler
	workerCount      int
	maxRetries       int
	retryDelay       time.Duration
	retryBackoff     float64
	scheduleInterval time.Duration
	shutdownChan     chan struct{}
	workerWg         sync.WaitGroup
	handlerMutex     sync.RWMutex
	running          int32
	stats            *Stats
	statsMutex       sync.RWMutex
	scheduleTicker   *time.Ticker
	progressChan     chan progressUpdate
}

type progressUpdate struct {
	jobID    string
	progress float64
}

// NewManager creates a new queue manager with default settings
func NewManager() *Manager {
	return &Manager{
		queue:            NewMemoryQueue(),
		handlers:         make(map[string]JobHandler),
		workerCount:      1,
		maxRetries:       3,
		retryDelay:       time.Second * 2,
		retryBackoff:     2.0,
		scheduleInterval: time.Second * 10,
		shutdownChan:     make(chan struct{}),
		stats:            &Stats{},
		progressChan:     make(chan progressUpdate, 100),
	}
}

// WithQueue sets the queue implementation
func (m *Manager) WithQueue(queue Queue) *Manager {
	m.queue = queue
	return m
}

// WithWorkers sets the number of workers
func (m *Manager) WithWorkers(workers int) *Manager {
	if workers > 0 {
		m.workerCount = workers
	}
	return m
}

// WithRetryAttempts sets the maximum number of retry attempts
func (m *Manager) WithRetryAttempts(attempts int) *Manager {
	if attempts >= 0 {
		m.maxRetries = attempts
	}
	return m
}

// WithRetryDelay sets the retry delay
func (m *Manager) WithRetryDelay(delay time.Duration) *Manager {
	if delay > 0 {
		m.retryDelay = delay
	}
	return m
}

// WithRetryBackoff sets the retry backoff multiplier
func (m *Manager) WithRetryBackoff(backoff float64) *Manager {
	if backoff > 0 {
		m.retryBackoff = backoff
	}
	return m
}

// WithScheduleInterval sets the interval for checking scheduled jobs
func (m *Manager) WithScheduleInterval(interval time.Duration) *Manager {
	if interval > 0 {
		m.scheduleInterval = interval
	}
	return m
}

// RegisterHandler registers a job handler for a specific job type
func (m *Manager) RegisterHandler(jobType string, handler JobHandler) {
	m.handlerMutex.Lock()
	defer m.handlerMutex.Unlock()
	m.handlers[jobType] = handler
}

// Start starts the queue manager
func (m *Manager) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return apperror.NewError("manager is already running")
	}

	log.Info().
		Int("workers", m.workerCount).
		Int("max_retries", m.maxRetries).
		Int64("retry_delay", m.retryDelay.Milliseconds()).
		Msg("starting queue manager")

	go m.progressReporter()

	m.scheduleTicker = time.NewTicker(m.scheduleInterval)
	go m.scheduleProcessor(ctx)

	for i := 0; i < m.workerCount; i++ {
		m.workerWg.Add(1)
		go m.worker(ctx, i)
	}

	interruption.OnSignal(m.Stop, os.Interrupt)
	return nil
}

// Stop stops the queue manager gracefully
func (m *Manager) Stop() error {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return apperror.NewError("manager is not running")
	}

	log.Info().Msg("stopping queue manager")

	if m.scheduleTicker != nil {
		m.scheduleTicker.Stop()
	}

	close(m.shutdownChan)

	done := make(chan struct{})
	go func() {
		m.workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("queue manager stopped")
	case <-time.After(time.Second * 5):
		log.Warn().Msg("timeout waiting for workers to stop")
	}

	return nil
}

// Enqueue adds a job to the queue
func (m *Manager) Enqueue(ctx context.Context, job *Job) error {
	if atomic.LoadInt32(&m.running) == 0 {
		return apperror.NewError("manager is not running")
	}

	if job.MaxAttempts == 0 {
		job.MaxAttempts = m.maxRetries
	}

	log.Debug().
		Str("job_id", job.ID).
		Str("job_type", job.Type).
		Str("priority", job.Priority.String()).
		Msg("job enqueued")

	return m.queue.Enqueue(ctx, job)
}

// Schedule adds a scheduled job to the queue
func (m *Manager) Schedule(ctx context.Context, job *Job) error {
	if atomic.LoadInt32(&m.running) == 0 {
		return apperror.NewError("manager is not running")
	}

	if job.MaxAttempts == 0 {
		job.MaxAttempts = m.maxRetries
	}

	log.Debug().
		Str("job_id", job.ID).
		Str("job_type", job.Type).
		Time("schedule_at", job.ScheduleAt).
		Msg("job scheduled")

	return m.queue.Schedule(ctx, job)
}

// GetJob retrieves a job by ID
func (m *Manager) GetJob(ctx context.Context, id string) (*Job, error) {
	return m.queue.GetJob(ctx, id)
}

// GetJobs retrieves jobs by status
func (m *Manager) GetJobs(ctx context.Context, status Status, limit int) ([]*Job, error) {
	return m.queue.GetJobs(ctx, status, limit)
}

// GetStats returns current queue statistics
func (m *Manager) GetStats() *Stats {
	m.statsMutex.RLock()
	defer m.statsMutex.RUnlock()

	if ctx := context.Background(); ctx != nil {
		if queueStats, err := m.queue.GetStats(ctx); err == nil {
			queueStats.WorkersActive = atomic.LoadInt64(&m.stats.WorkersActive)
			queueStats.WorkersBusy = atomic.LoadInt64(&m.stats.WorkersBusy)
			return queueStats
		}
	}

	// Fallback to manager stats only
	return &Stats{
		JobsProcessed: atomic.LoadInt64(&m.stats.JobsProcessed),
		JobsFailed:    atomic.LoadInt64(&m.stats.JobsFailed),
		JobsRetried:   atomic.LoadInt64(&m.stats.JobsRetried),
		QueueSize:     atomic.LoadInt64(&m.stats.QueueSize),
		WorkersActive: atomic.LoadInt64(&m.stats.WorkersActive),
		WorkersBusy:   atomic.LoadInt64(&m.stats.WorkersBusy),
	}
}

// IsRunning returns true if the manager is currently running
func (m *Manager) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

// worker processes jobs from the queue
func (m *Manager) worker(ctx context.Context, workerID int) {
	defer m.workerWg.Done()

	log.Debug().Int("worker_id", workerID).Msg("worker started")

	atomic.AddInt64(&m.stats.WorkersActive, 1)
	defer atomic.AddInt64(&m.stats.WorkersActive, -1)

	for {
		select {
		case <-ctx.Done():
			log.Debug().Int("worker_id", workerID).Msg("worker stopped due to context cancellation")
			return
		case <-m.shutdownChan:
			log.Debug().Int("worker_id", workerID).Msg("worker stopped due to shutdown signal")
			return
		default:
			job, err := m.queue.Dequeue(ctx, time.Second*5)
			if err != nil {
				continue
			}

			if job != nil {
				atomic.AddInt64(&m.stats.WorkersBusy, 1)
				m.processJob(ctx, job, workerID)
				atomic.AddInt64(&m.stats.WorkersBusy, -1)
			}
		}
	}
}

// processJob processes a single job
func (m *Manager) processJob(ctx context.Context, job *Job, workerID int) {
	log.Debug().
		Str("job_id", job.ID).
		Str("job_type", job.Type).
		Int("worker_id", workerID).
		Msg("processing job")

	job.Status = StatusRunning
	job.Attempts++
	job.UpdatedAt = time.Now()
	m.queue.UpdateJob(ctx, job)

	m.handlerMutex.RLock()
	handler, exists := m.handlers[job.Type]
	m.handlerMutex.RUnlock()

	if !exists {
		job.Status = StatusFailed
		job.Error = fmt.Sprintf("no handler registered for job type: %s", job.Type)
		job.UpdatedAt = time.Now()
		m.queue.UpdateJob(ctx, job)
		atomic.AddInt64(&m.stats.JobsFailed, 1)
		log.Error().Str("job_id", job.ID).Str("job_type", job.Type).Msg("no handler found")
		return
	}

	err := handler(ctx, job)
	if err != nil {
		if retryErr, ok := err.(*RetryableError); ok && job.Attempts < job.MaxAttempts {
			job.Status = StatusRetrying
			job.Error = retryErr.Error()
			job.RetryAt = time.Now().Add(m.calculateRetryDelay(job.Attempts))
			job.UpdatedAt = time.Now()
			m.queue.UpdateJob(ctx, job)
			atomic.AddInt64(&m.stats.JobsRetried, 1)

			log.Info().
				Str("job_id", job.ID).
				Str("job_type", job.Type).
				Int64("retry_delay", m.calculateRetryDelay(job.Attempts).Milliseconds()).
				Msg("job scheduled for retry")

			go func() {
				time.Sleep(m.calculateRetryDelay(job.Attempts))
				job.Status = StatusPending
				job.RetryAt = time.Time{}
				m.queue.Enqueue(ctx, job)
			}()
		} else {
			job.Status = StatusFailed
			job.Error = err.Error()
			job.UpdatedAt = time.Now()
			m.queue.UpdateJob(ctx, job)
			atomic.AddInt64(&m.stats.JobsFailed, 1)

			log.Error().
				Err(err).
				Str("job_id", job.ID).
				Str("job_type", job.Type).
				Int("attempt", job.Attempts).
				Msg("job failed")
		}
	} else {
		job.Status = StatusCompleted
		job.CompletedAt = time.Now()
		job.Progress = 1.0
		job.UpdatedAt = time.Now()
		m.queue.UpdateJob(ctx, job)
		atomic.AddInt64(&m.stats.JobsProcessed, 1)

		log.Debug().
			Str("job_id", job.ID).
			Str("job_type", job.Type).
			Msg("job completed successfully")
	}
}

// calculateRetryDelay calculates the retry delay with exponential backoff
func (m *Manager) calculateRetryDelay(attempt int) time.Duration {
	delay := float64(m.retryDelay.Nanoseconds())
	for i := 1; i < attempt; i++ {
		delay *= m.retryBackoff
	}
	return time.Duration(delay)
}

// scheduleProcessor processes scheduled jobs
func (m *Manager) scheduleProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.shutdownChan:
			return
		case <-m.scheduleTicker.C:
			jobs, err := m.queue.GetJobs(ctx, StatusScheduled, 100)
			if err != nil {
				log.Error().Err(err).Msg("failed to get scheduled jobs")
				continue
			}

			now := time.Now()
			for _, job := range jobs {
				if job.ScheduleAt.Before(now) || job.ScheduleAt.Equal(now) {
					job.Status = StatusPending
					job.ScheduleAt = time.Time{}
					m.queue.Enqueue(ctx, job)
				}
			}
		}
	}
}

// progressReporter handles job progress updates
func (m *Manager) progressReporter() {
	for update := range m.progressChan {
		log.Debug().
			Str("job_id", update.jobID).
			Float64("progress", update.progress).
			Msg("job progress updated")
	}
}

// RetryableError represents an error that should trigger a retry
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

// NewRetryableError creates a new retryable error
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{Err: err}
}

// JobProgress represents job progress information
type JobProgress struct {
	JobID    string  `json:"job_id"`
	Progress float64 `json:"progress"`
	Message  string  `json:"message,omitempty"`
}
