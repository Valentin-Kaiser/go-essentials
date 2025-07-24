package mail

import (
	"context"
	"encoding/json"
	"io/fs"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/queue"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Manager manages email sending and SMTP server functionality
type Manager struct {
	config          *Config
	sender          Sender
	server          Server
	templateManager TemplateManager
	queueManager    *queue.Manager
	stats           *Stats
	statsMutex      sync.RWMutex
	running         int32
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewManager creates a new mail manager
func NewManager(config *Config, queueManager *queue.Manager) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		config:       config,
		queueManager: queueManager,
		stats:        &Stats{},
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize components
	manager.templateManager = NewTemplateManager(config.Templates)
	manager.sender = NewSMTPSender(config.SMTP, manager.templateManager)

	if config.Server.Enabled {
		manager.server = NewSMTPServer(config.Server, manager)
	}

	return manager
}

// Start starts the mail manager
func (m *Manager) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return apperror.NewError("mail manager is already running")
	}

	log.Info().Msg("[Mail] Starting mail manager")

	// Register mail job handler with queue manager if queue is enabled
	if m.config.Queue.Enabled && m.queueManager != nil {
		m.queueManager.RegisterHandler("mail", m.handleMailJob)
		log.Info().Str("queue", m.config.Queue.QueueName).Msg("[Mail] Registered mail job handler")
	}

	// Start SMTP server if enabled
	if m.config.Server.Enabled && m.server != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.server.Start(m.ctx); err != nil {
				log.Error().Err(err).Msg("[Mail] Failed to start SMTP server")
			}
		}()
	}

	log.Info().Msg("[Mail] Mail manager started successfully")
	return nil
}

// Stop stops the mail manager
func (m *Manager) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return apperror.NewError("mail manager is not running")
	}

	log.Info().Msg("[Mail] Stopping mail manager")

	// Cancel context to signal shutdown
	m.cancel()

	// Stop SMTP server if running
	if m.server != nil && m.server.IsRunning() {
		if err := m.server.Stop(ctx); err != nil {
			log.Error().Err(err).Msg("[Mail] Failed to stop SMTP server")
		}
	}

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("[Mail] Mail manager stopped successfully")
		return nil
	case <-ctx.Done():
		log.Warn().Msg("[Mail] Mail manager stop timed out")
		return ctx.Err()
	}
}

// Send sends an email message synchronously
func (m *Manager) Send(ctx context.Context, message *Message) error {
	if !m.IsRunning() {
		return apperror.NewError("mail manager is not running")
	}

	// Generate ID if not set
	if message.ID == "" {
		message.ID = uuid.New().String()
	}

	log.Debug().
		Str("id", message.ID).
		Str("subject", message.Subject).
		Strs("to", message.To).
		Msg("[Mail] Sending email")

	// Send the message
	err := m.sender.Send(ctx, message)
	if err != nil {
		m.incrementFailedCount()
		log.Error().
			Err(err).
			Str("id", message.ID).
			Str("subject", message.Subject).
			Msg("[Mail] Failed to send email")
		return apperror.Wrap(err)
	}

	m.incrementSentCount()
	m.updateLastSent()

	log.Info().
		Str("id", message.ID).
		Str("subject", message.Subject).
		Strs("to", message.To).
		Msg("[Mail] Email sent successfully")

	return nil
}

// SendAsync sends an email message asynchronously using the queue
func (m *Manager) SendAsync(ctx context.Context, message *Message) error {
	if !m.IsRunning() {
		return apperror.NewError("mail manager is not running")
	}

	if !m.config.Queue.Enabled || m.queueManager == nil {
		return apperror.NewError("queue is not enabled")
	}

	// Generate ID if not set
	if message.ID == "" {
		message.ID = uuid.New().String()
	}

	log.Debug().
		Str("id", message.ID).
		Str("subject", message.Subject).
		Strs("to", message.To).
		Msg("[Mail] Queuing email for async sending")

	// Create queue job
	jobData := map[string]interface{}{
		"id":            message.ID,
		"from":          message.From,
		"to":            message.To,
		"cc":            message.CC,
		"bcc":           message.BCC,
		"reply_to":      message.ReplyTo,
		"subject":       message.Subject,
		"text_body":     message.TextBody,
		"html_body":     message.HTMLBody,
		"template":      message.Template,
		"template_data": message.TemplateData,
		"attachments":   message.Attachments,
		"headers":       message.Headers,
		"priority":      int(message.Priority),
		"created_at":    message.CreatedAt,
		"schedule_at":   message.ScheduleAt,
		"metadata":      message.Metadata,
	}

	job := queue.NewJob("mail").
		WithPayload(jobData).
		WithPriority(queue.Priority(message.Priority)).
		WithMaxAttempts(m.config.Queue.MaxAttempts)

	// Schedule the job if specified
	if message.ScheduleAt != nil {
		job = job.WithScheduleAt(*message.ScheduleAt)
	}

	// Enqueue the job
	err := m.queueManager.Enqueue(ctx, job.Build())
	if err != nil {
		log.Error().
			Err(err).
			Str("id", message.ID).
			Str("subject", message.Subject).
			Msg("[Mail] Failed to queue email")
		return apperror.Wrap(err)
	}

	m.incrementQueuedCount()

	log.Info().
		Str("id", message.ID).
		Str("subject", message.Subject).
		Strs("to", message.To).
		Msg("[Mail] Email queued for async sending")

	return nil
}

// AddNotificationHandler adds a notification handler to the SMTP server
func (m *Manager) AddNotificationHandler(handler NotificationHandler) error {
	if m.server == nil {
		return apperror.NewError("SMTP server is not configured")
	}

	m.server.AddHandler(handler)
	log.Info().Msg("[Mail] Added notification handler to SMTP server")
	return nil
}

// GetStats returns the current mail statistics
func (m *Manager) GetStats() *Stats {
	m.statsMutex.RLock()
	defer m.statsMutex.RUnlock()

	// Create a copy to avoid race conditions
	stats := &Stats{
		SentCount:     m.stats.SentCount,
		FailedCount:   m.stats.FailedCount,
		QueuedCount:   m.stats.QueuedCount,
		ReceivedCount: m.stats.ReceivedCount,
	}

	if m.stats.LastSent != nil {
		lastSent := *m.stats.LastSent
		stats.LastSent = &lastSent
	}

	if m.stats.LastReceived != nil {
		lastReceived := *m.stats.LastReceived
		stats.LastReceived = &lastReceived
	}

	return stats
}

// IsRunning returns true if the mail manager is running
func (m *Manager) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

// SendTestEmail sends a test email
func (m *Manager) SendTestEmail(ctx context.Context, to string) error {
	testMessage := &Message{
		From:    m.config.SMTP.From,
		To:      []string{to},
		Subject: "Test Email",
		TextBody: "This is a test email from the mail manager.",
		HTMLBody: "<p>This is a test email from the mail manager.</p>",
	}

	return m.Send(ctx, testMessage)
}

// handleMailJob handles queued mail jobs
func (m *Manager) handleMailJob(ctx context.Context, job *queue.Job) error {
	log.Debug().
		Str("job_id", job.ID).
		Msg("[Mail] Processing mail job")

	// Decode the message from job payload
	var jobData map[string]interface{}
	if err := json.Unmarshal(job.Payload, &jobData); err != nil {
		log.Error().
			Err(err).
			Str("job_id", job.ID).
			Msg("[Mail] Failed to decode mail job payload")
		return apperror.Wrap(err)
	}

	// Convert job data back to message
	message, err := m.jobDataToMessage(jobData)
	if err != nil {
		log.Error().
			Err(err).
			Str("job_id", job.ID).
			Msg("[Mail] Failed to convert job data to message")
		return apperror.Wrap(err)
	}

	// Send the message
	if err = m.sender.Send(ctx, message); err != nil {
		m.incrementFailedCount()
		log.Error().
			Err(err).
			Str("job_id", job.ID).
			Str("message_id", message.ID).
			Str("subject", message.Subject).
			Msg("[Mail] Failed to send queued email")
		return apperror.Wrap(err)
	}

	m.incrementSentCount()
	m.updateLastSent()
	m.decrementQueuedCount()

	log.Info().
		Str("job_id", job.ID).
		Str("message_id", message.ID).
		Str("subject", message.Subject).
		Strs("to", message.To).
		Msg("[Mail] Queued email sent successfully")

	return nil
}

// NotifyMessageReceived notifies that a message was received by the SMTP server
func (m *Manager) NotifyMessageReceived() {
	m.incrementReceivedCount()
	m.updateLastReceived()
}

// incrementSentCount increments the sent count
func (m *Manager) incrementSentCount() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	m.stats.SentCount++
}

// incrementFailedCount increments the failed count
func (m *Manager) incrementFailedCount() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	m.stats.FailedCount++
}

// incrementQueuedCount increments the queued count
func (m *Manager) incrementQueuedCount() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	m.stats.QueuedCount++
}

// decrementQueuedCount decrements the queued count
func (m *Manager) decrementQueuedCount() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	if m.stats.QueuedCount > 0 {
		m.stats.QueuedCount--
	}
}

// incrementReceivedCount increments the received count
func (m *Manager) incrementReceivedCount() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	m.stats.ReceivedCount++
}

// updateLastSent updates the last sent timestamp
func (m *Manager) updateLastSent() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	now := time.Now()
	m.stats.LastSent = &now
}

// updateLastReceived updates the last received timestamp
func (m *Manager) updateLastReceived() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	now := time.Now()
	m.stats.LastReceived = &now
}

// jobDataToMessage converts job data back to a Message
func (m *Manager) jobDataToMessage(jobData map[string]interface{}) (*Message, error) {
	message := &Message{}

	if id, ok := jobData["id"].(string); ok {
		message.ID = id
	}
	if from, ok := jobData["from"].(string); ok {
		message.From = from
	}
	if to, ok := jobData["to"].([]interface{}); ok {
		message.To = make([]string, len(to))
		for i, v := range to {
			if s, ok := v.(string); ok {
				message.To[i] = s
			}
		}
	}
	if cc, ok := jobData["cc"].([]interface{}); ok {
		message.CC = make([]string, len(cc))
		for i, v := range cc {
			if s, ok := v.(string); ok {
				message.CC[i] = s
			}
		}
	}
	if bcc, ok := jobData["bcc"].([]interface{}); ok {
		message.BCC = make([]string, len(bcc))
		for i, v := range bcc {
			if s, ok := v.(string); ok {
				message.BCC[i] = s
			}
		}
	}
	if replyTo, ok := jobData["reply_to"].(string); ok {
		message.ReplyTo = replyTo
	}
	if subject, ok := jobData["subject"].(string); ok {
		message.Subject = subject
	}
	if textBody, ok := jobData["text_body"].(string); ok {
		message.TextBody = textBody
	}
	if htmlBody, ok := jobData["html_body"].(string); ok {
		message.HTMLBody = htmlBody
	}
	if template, ok := jobData["template"].(string); ok {
		message.Template = template
	}
	if templateData, ok := jobData["template_data"]; ok {
		message.TemplateData = templateData
	}
	if priority, ok := jobData["priority"].(float64); ok {
		message.Priority = Priority(int(priority))
	}
	if headers, ok := jobData["headers"].(map[string]interface{}); ok {
		message.Headers = make(map[string]string)
		for k, v := range headers {
			if s, ok := v.(string); ok {
				message.Headers[k] = s
			}
		}
	}
	if metadata, ok := jobData["metadata"].(map[string]interface{}); ok {
		message.Metadata = make(map[string]string)
		for k, v := range metadata {
			if s, ok := v.(string); ok {
				message.Metadata[k] = s
			}
		}
	}
	if createdAt, ok := jobData["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			message.CreatedAt = t
		}
	}
	if scheduleAt, ok := jobData["schedule_at"].(string); ok && scheduleAt != "" {
		if t, err := time.Parse(time.RFC3339, scheduleAt); err == nil {
			message.ScheduleAt = &t
		}
	}

	return message, nil
}

// WithFS configures the template manager to load templates from a filesystem
func (m *Manager) WithFS(filesystem fs.FS) *Manager {
	if m.templateManager != nil {
		m.templateManager.WithFS(filesystem)
	}
	return m
}

// WithFileServer configures the template manager to load templates from a file path
func (m *Manager) WithFileServer(templatesPath string) *Manager {
	if m.templateManager != nil {
		m.templateManager.WithFileServer(templatesPath)
	}
	return m
}

// ReloadTemplates reloads all templates
func (m *Manager) ReloadTemplates() error {
	if m.templateManager != nil {
		return m.templateManager.ReloadTemplates()
	}
	return apperror.NewError("template manager is not initialized")
}

// GetTemplateManager returns the template manager for advanced configuration
func (m *Manager) GetTemplateManager() TemplateManager {
	return m.templateManager
}
