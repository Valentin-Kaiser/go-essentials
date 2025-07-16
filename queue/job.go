package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
)

// Priority defines the priority levels for jobs
type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityCritical
)

// String returns the string representation of the priority
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Status represents the status of a job
type Status int

const (
	StatusPending Status = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusRetrying
	StatusScheduled
	StatusDeadLetter
)

// String returns the string representation of the status
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusRetrying:
		return "retrying"
	case StatusScheduled:
		return "scheduled"
	case StatusDeadLetter:
		return "dead_letter"
	default:
		return "unknown"
	}
}

// Job represents a job to be processed
type Job struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Priority    Priority          `json:"priority"`
	Status      Status            `json:"status"`
	Attempts    int               `json:"attempts"`
	MaxAttempts int               `json:"max_attempts"`
	Progress    float64           `json:"progress"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
	ScheduleAt  time.Time         `json:"schedule_at,omitempty"`
	RetryAt     time.Time         `json:"retry_at,omitempty"`
	Timeout     time.Duration     `json:"timeout"`
	Payload     json.RawMessage   `json:"payload,omitempty"`
	Results     json.RawMessage   `json:"results,omitempty"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// IsScheduled returns true if the job is scheduled for a future time
func (j *Job) IsScheduled() bool {
	return !j.ScheduleAt.IsZero() && j.ScheduleAt.After(time.Now())
}

// IsExpired returns true if the job has exceeded its maximum attempts
func (j *Job) IsExpired() bool {
	return j.Attempts >= j.MaxAttempts
}

// JobHandler is a function that processes jobs
type JobHandler func(ctx context.Context, job *Job) error

// Batch represents a batch of jobs
type Batch struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	JobIDs      []string          `json:"job_ids"`
	Status      Status            `json:"status"`
	Total       int               `json:"total"`
	Pending     int               `json:"pending"`
	Running     int               `json:"running"`
	Completed   int               `json:"completed"`
	Failed      int               `json:"failed"`
	CreatedAt   time.Time         `json:"created_at"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// BatchManager manages job batches
type BatchManager struct {
	manager *Manager
	batches map[string]*Batch
	mutex   sync.RWMutex
}

// NewBatchManager creates a new batch manager
func NewBatchManager(manager *Manager) *BatchManager {
	return &BatchManager{
		manager: manager,
		batches: make(map[string]*Batch),
	}
}

// CreateBatch creates a new batch of jobs
func (bm *BatchManager) CreateBatch(ctx context.Context, name string, jobs []*Job) (*Batch, error) {
	bm.mutex.Lock()
	defer bm.mutex.Unlock()

	batch := &Batch{
		ID:        bm.generateBatchID(),
		Name:      name,
		JobIDs:    make([]string, 0, len(jobs)),
		Status:    StatusPending,
		Total:     len(jobs),
		Pending:   len(jobs),
		CreatedAt: time.Now(),
		Metadata:  make(map[string]string),
	}

	// Enqueue all jobs
	for _, job := range jobs {
		if job.Metadata == nil {
			job.Metadata = make(map[string]string)
		}
		job.Metadata["batch_id"] = batch.ID

		if err := bm.manager.Enqueue(ctx, job); err != nil {
			return nil, apperror.Wrap(err)
		}

		batch.JobIDs = append(batch.JobIDs, job.ID)
	}

	bm.batches[batch.ID] = batch
	return batch, nil
}

// GetBatch retrieves a batch by ID
func (bm *BatchManager) GetBatch(id string) (*Batch, error) {
	bm.mutex.RLock()
	defer bm.mutex.RUnlock()

	batch, exists := bm.batches[id]
	if !exists {
		return nil, apperror.NewError("batch not found")
	}

	return batch, nil
}

// UpdateBatchStatus updates the status of a batch based on its jobs
func (bm *BatchManager) UpdateBatchStatus(ctx context.Context, batchID string) error {
	bm.mutex.Lock()
	defer bm.mutex.Unlock()

	batch, exists := bm.batches[batchID]
	if !exists {
		return apperror.NewError("batch not found")
	}

	batch.Pending = 0
	batch.Running = 0
	batch.Completed = 0
	batch.Failed = 0

	for _, jobID := range batch.JobIDs {
		job, err := bm.manager.GetJob(ctx, jobID)
		if err != nil {
			continue
		}

		switch job.Status {
		case StatusPending:
			batch.Pending++
		case StatusRunning:
			batch.Running++
		case StatusCompleted:
			batch.Completed++
		case StatusFailed:
			batch.Failed++
		}
	}

	switch {
	case batch.Completed == batch.Total:
		batch.Status = StatusCompleted
		batch.CompletedAt = time.Now()
	case batch.Failed > 0:
		batch.Status = StatusFailed
	case batch.Running > 0:
		batch.Status = StatusRunning
	default:
		batch.Status = StatusPending
	}

	return nil
}

// GetBatches returns all batches
func (bm *BatchManager) GetBatches() []*Batch {
	bm.mutex.RLock()
	defer bm.mutex.RUnlock()

	batches := make([]*Batch, 0, len(bm.batches))
	for _, batch := range bm.batches {
		batches = append(batches, batch)
	}

	return batches
}

// DeleteBatch removes a batch
func (bm *BatchManager) DeleteBatch(ctx context.Context, id string) error {
	bm.mutex.Lock()
	defer bm.mutex.Unlock()

	batch, exists := bm.batches[id]
	if !exists {
		return apperror.NewError("batch not found")
	}

	for _, jobID := range batch.JobIDs {
		bm.manager.queue.DeleteJob(ctx, jobID)
	}

	delete(bm.batches, id)
	return nil
}

// generateBatchID generates a unique batch ID
func (bm *BatchManager) generateBatchID() string {
	return fmt.Sprintf("batch_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond())
}

// JobContext provides context for job execution
type JobContext struct {
	context.Context
	Job *Job
}

// NewJobContext creates a new job context
func NewJobContext(ctx context.Context, job *Job) *JobContext {
	return &JobContext{
		Context: ctx,
		Job:     job,
	}
}

// ReportProgress reports job progress
func (jc *JobContext) ReportProgress(progress float64) {
	jc.Job.Progress = progress
	jc.Job.UpdatedAt = time.Now()
}

// GetPayload gets a value from the job payload
func (jc *JobContext) GetPayload(key string) (interface{}, bool) {
	if jc.Job.Payload == nil {
		return nil, false
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(jc.Job.Payload, &payload); err != nil {
		return nil, false
	}

	value, exists := payload[key]
	return value, exists
}

// GetPayloadString gets a string value from the job payload
func (jc *JobContext) GetPayloadString(key string) (string, bool) {
	value, exists := jc.GetPayload(key)
	if !exists {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}

// GetPayloadInt gets an int value from the job payload
func (jc *JobContext) GetPayloadInt(key string) (int, bool) {
	value, exists := jc.GetPayload(key)
	if !exists {
		return 0, false
	}

	switch v := value.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// GetPayloadBool gets a bool value from the job payload
func (jc *JobContext) GetPayloadBool(key string) (bool, bool) {
	value, exists := jc.GetPayload(key)
	if !exists {
		return false, false
	}
	b, ok := value.(bool)
	return b, ok
}

// GetMetadata gets a metadata value
func (jc *JobContext) GetMetadata(key string) (string, bool) {
	if jc.Job.Metadata == nil {
		return "", false
	}
	value, exists := jc.Job.Metadata[key]
	return value, exists
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*RetryableError)
	return ok
}
