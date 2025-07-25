// Package mail provides comprehensive email sending and SMTP server functionality
// with support for templates, queuing, and notification handling.
//
// Features:
//   - SMTP client for sending emails with various authentication methods
//   - SMTP server for receiving emails with notification handlers
//   - HTML template support with embedded and custom templates
//   - Queue integration for asynchronous email processing
//   - TLS/STARTTLS encryption support
//   - Attachment support
//   - Statistics tracking
//   - Retry mechanisms with exponential backoff
//
// Example Usage:
//
//	package main
//
//	import (
//		"context"
//		"time"
//
//		"github.com/Valentin-Kaiser/go-core/mail"
//		"github.com/Valentin-Kaiser/go-core/queue"
//	)
//
//	func main() {
//		// Create mail configuration
//		config := mail.DefaultConfig()
//		config.SMTP.Host = "smtp.gmail.com"
//		config.SMTP.Port = 587
//		config.SMTP.Username = "your-email@gmail.com"
//		config.SMTP.Password = "your-password"
//		config.SMTP.From = "noreply@example.com"
//		config.SMTP.Auth = true
//		config.SMTP.Encryption = "STARTTLS"
//
//		// Create queue manager
//		queueManager := queue.NewManager()
//
//		// Create mail manager
//		mailManager := mail.NewManager(config, queueManager)
//
//		// Start the manager
//		ctx := context.Background()
//		if err := mailManager.Start(ctx); err != nil {
//			panic(err)
//		}
//		defer mailManager.Stop(ctx)
//
//		// Send a simple email
//		message := mail.NewMessage().
//			To("recipient@example.com").
//			Subject("Test Email").
//			TextBody("Hello, this is a test email!").
//			Build()
//
//		if err := mailManager.Send(ctx, message); err != nil {
//			panic(err)
//		}
//
//		// Send an email with template
//		templateMessage := mail.NewMessage().
//			To("user@example.com").
//			Subject("Welcome!").
//			Template("welcome.html", map[string]interface{}{
//				"FirstName":     "John",
//				"ActivationURL": "https://example.com/activate?token=abc123",
//			}).
//			Build()
//
//		if err := mailManager.SendAsync(ctx, templateMessage); err != nil {
//			panic(err)
//		}
//	}
//
// SMTP Server Example:
//
//	config := mail.DefaultConfig()
//	config.Server.Enabled = true
//	config.Server.Host = "localhost"
//	config.Server.Port = 2525
//	config.Server.Domain = "example.com"
//	config.Server.Auth = true
//	config.Server.Username = "admin"
//	config.Server.Password = "password"
//
//	mailManager := mail.NewManager(config, nil)
//
//	// Add notification handler
//	mailManager.AddNotificationHandler(func(ctx context.Context, from string, to []string, data []byte) error {
//		log.Printf("Received email from %s to %v", from, to)
//		return nil
//	})
//
//	if err := mailManager.Start(ctx); err != nil {
//		panic(err)
//	}
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
	TemplateManager *TemplateManager
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

	manager.TemplateManager = NewTemplateManager(config.Templates)
	manager.sender = NewSMTPSender(config.Client, manager.TemplateManager)

	if config.Server.Enabled {
		manager.server = NewSMTPServer(config.Server, manager)
	}

	return manager
}

// Start starts the mail manager
func (m *Manager) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return apperror.NewError("mail manager is already running")
	}

	if m.config.Queue.Enabled && m.queueManager != nil {
		m.queueManager.RegisterHandler("mail", m.handleMailJob)
	}

	if m.config.Server.Enabled && m.server != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.server.Start(m.ctx); err != nil {
				log.Error().Err(err).Msg("[Mail] Failed to start SMTP server")
			}
		}()
	}

	return nil
}

// Stop stops the mail manager
func (m *Manager) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return apperror.NewError("mail manager is not running")
	}

	m.cancel()
	if m.server != nil && m.server.IsRunning() {
		if err := m.server.Stop(ctx); err != nil {
			log.Error().Err(err).Msg("[Mail] Failed to stop SMTP server")
		}
	}

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
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
		From:     m.config.Client.From,
		To:       []string{to},
		Subject:  "Test Email",
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
		return apperror.Wrap(err)
	}

	// Convert job data back to message
	message := m.jobDataToMessage(jobData)

	// Send the message
	if err := m.sender.Send(ctx, message); err != nil {
		m.incrementFailedCount()
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
func (m *Manager) jobDataToMessage(jobData map[string]interface{}) *Message {
	message := &Message{}

	if id, ok := jobData["id"].(string); ok {
		message.ID = id
	}
	if from, ok := jobData["from"].(string); ok {
		message.From = from
	}
	if to := jobData["to"]; to != nil {
		message.To = convertToStringSlice(to)
	}
	if cc := jobData["cc"]; cc != nil {
		message.CC = convertToStringSlice(cc)
	}
	if bcc := jobData["bcc"]; bcc != nil {
		message.BCC = convertToStringSlice(bcc)
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

	return message
}

// WithFS configures the template manager to load templates from a filesystem
func (m *Manager) WithFS(filesystem fs.FS) *Manager {
	if m.TemplateManager != nil {
		m.TemplateManager.WithFS(filesystem)
	}
	return m
}

// WithFileServer configures the template manager to load templates from a file path
func (m *Manager) WithFileServer(templatesPath string) *Manager {
	if m.TemplateManager != nil {
		m.TemplateManager.WithFileServer(templatesPath)
	}
	return m
}

// ReloadTemplates reloads all templates
func (m *Manager) ReloadTemplates() error {
	if m.TemplateManager != nil {
		return m.TemplateManager.ReloadTemplates()
	}
	return apperror.NewError("template manager is not initialized")
}

// convertToStringSlice safely converts various types to []string
// This handles different JSON unmarshaling scenarios for string slice fields
func convertToStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		// Direct string slice (ideal case)
		return v
	case []interface{}:
		// JSON unmarshaled as interface slice
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case string:
		// Single string value
		return []string{v}
	case nil:
		// Nil value
		return nil
	default:
		// Fallback: try to convert to string and return as single-item slice
		if str := convertToString(value); str != "" {
			return []string{str}
		}
		return nil
	}
}

// convertToString safely converts various types to string
func convertToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}
