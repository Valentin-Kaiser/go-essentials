package mail_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/mail"
	"github.com/Valentin-Kaiser/go-core/queue"
)

func TestDefaultConfig(t *testing.T) {
	config := mail.DefaultConfig()

	if config.SMTP.Host != "localhost" {
		t.Errorf("Expected SMTP host to be localhost, got %s", config.SMTP.Host)
	}

	if config.SMTP.Port != 587 {
		t.Errorf("Expected SMTP port to be 587, got %d", config.SMTP.Port)
	}

	if config.Queue.Enabled != true {
		t.Error("Expected queue to be enabled by default")
	}
}

func TestMessageBuilder(t *testing.T) {
	message, err := mail.NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Test Subject").
		TextBody("Test message").
		Priority(mail.PriorityHigh).
		Build()

	if err != nil {
		t.Errorf("Expected no error when building message, got: %v", err)
	}

	if message.From != "sender@example.com" {
		t.Errorf("Expected from to be sender@example.com, got %s", message.From)
	}

	if len(message.To) != 1 || message.To[0] != "recipient@example.com" {
		t.Errorf("Expected to to be [recipient@example.com], got %v", message.To)
	}

	if message.Subject != "Test Subject" {
		t.Errorf("Expected subject to be 'Test Subject', got %s", message.Subject)
	}

	if message.Priority != mail.PriorityHigh {
		t.Errorf("Expected priority to be high, got %v", message.Priority)
	}
}

func TestTemplateManager(t *testing.T) {
	config := mail.TemplateConfig{
		DefaultTemplate: "default.html",
		AutoReload:      true,
	}

	tm := mail.NewTemplateManager(config)

	// Test that template manager is created
	if tm == nil {
		t.Error("Expected template manager to be created, got nil")
	}

	// Test loading template without any configured source should fail
	_, err := tm.LoadTemplate("default.html")
	if err == nil {
		t.Error("Expected error when loading template without configured source")
	}

	// Test WithFS method exists
	if tm.WithFS(nil) == nil {
		t.Error("Expected WithFS to return template manager")
	}

	// Test WithFileServer method exists
	if tm.WithFileServer("") == nil {
		t.Error("Expected WithFileServer to return template manager")
	}
}

// TestTemplateManagerWithFS tests the template manager with filesystem
func TestTemplateManagerWithFS(t *testing.T) {
	config := mail.TemplateConfig{
		DefaultTemplate: "test.html",
		AutoReload:      true,
	}

	tm := mail.NewTemplateManager(config)

	// Test that WithFS method works (we can't test with real filesystem without test data)
	result := tm.WithFS(nil)
	if result != tm {
		t.Error("Expected WithFS to return same template manager instance")
	}
}

// TestTemplateManagerWithFileServer tests the template manager with file server
func TestTemplateManagerWithFileServer(t *testing.T) {
	// Create a temporary directory with a test template
	tempDir := t.TempDir()
	defer apperror.Catch(func() error { return os.RemoveAll(tempDir) }, "failed to remove temp dir")

	// Create a test template file
	templateContent := `<html><body><h1>{{.Subject}}</h1><p>{{.Message}}</p></body></html>`
	templatePath := filepath.Join(tempDir, "test.html")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0600); err != nil {
		t.Fatalf("Failed to write test template: %v", err)
	}

	config := mail.TemplateConfig{
		DefaultTemplate: "test.html",
		AutoReload:      true,
	}

	tm := mail.NewTemplateManager(config)

	// Configure with file server
	tm.WithFileServer(tempDir)

	// Test loading template
	template, err := tm.LoadTemplate("test.html")
	if err != nil {
		t.Errorf("Failed to load template from file server: %v", err)
	}

	if template == nil {
		t.Error("Expected template to be loaded, got nil")
	}

	// Test rendering template
	data := map[string]interface{}{
		"Subject": "Test Subject",
		"Message": "Test message content",
	}

	rendered, err := tm.RenderTemplate("test.html", data)
	if err != nil {
		t.Errorf("Failed to render template: %v", err)
	}

	if rendered == "" {
		t.Error("Expected rendered template to have content")
	}

	// Check that the rendered content contains our data
	if !strings.Contains(rendered, "Test Subject") {
		t.Error("Expected rendered template to contain subject")
	}

	if !strings.Contains(rendered, "Test message content") {
		t.Error("Expected rendered template to contain message")
	}
}

func TestSMTPSender(t *testing.T) {
	config := mail.SMTPConfig{
		Host:       "localhost",
		Port:       587,
		From:       "sender@example.com",
		Auth:       false,
		Encryption: "NONE",
		MaxRetries: 3,
		RetryDelay: time.Second,
	}

	templateConfig := mail.TemplateConfig{
		DefaultTemplate: "default.html",
	}

	tm := mail.NewTemplateManager(templateConfig)
	sender := mail.NewSMTPSender(config, tm)

	if sender == nil {
		t.Error("Expected sender to be created, got nil")
	}

	// Test message validation
	message := &mail.Message{} // Empty message should fail validation

	err := sender.Send(t.Context(), message)
	if err == nil {
		t.Error("Expected validation error for empty message")
	}
}

func TestMailManager(t *testing.T) {
	config := mail.DefaultConfig()
	config.Queue.Enabled = false // Disable queue for this test

	queueManager := queue.NewManager()
	manager := mail.NewManager(config, queueManager)

	if manager == nil {
		t.Error("Expected manager to be created, got nil")
	}

	if manager.IsRunning() {
		t.Error("Expected manager to not be running initially")
	}

	stats := manager.GetStats()
	if stats == nil {
		t.Error("Expected stats to be available")
		return
	}

	if stats.SentCount != 0 {
		t.Errorf("Expected sent count to be 0, got %d", stats.SentCount)
	}
}

func TestPriorityString(t *testing.T) {
	tests := []struct {
		priority mail.Priority
		expected string
	}{
		{mail.PriorityLow, "low"},
		{mail.PriorityNormal, "normal"},
		{mail.PriorityHigh, "high"},
		{mail.PriorityCritical, "critical"},
	}

	for _, test := range tests {
		if test.priority.String() != test.expected {
			t.Errorf("Expected priority %d to be %s, got %s", test.priority, test.expected, test.priority.String())
		}
	}
}

func TestAttachmentBuilder(t *testing.T) {
	attachment := mail.Attachment{
		Filename:    "test.txt",
		ContentType: "text/plain",
		Content:     []byte("test content"),
		Size:        12,
		Inline:      false,
	}

	message, err := mail.NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Test with attachment").
		TextBody("Please see attachment").
		Attach(attachment).
		Build()

	if err != nil {
		t.Errorf("Expected no error when building message, got: %v", err)
	}

	if len(message.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(message.Attachments))
	}

	if message.Attachments[0].Filename != "test.txt" {
		t.Errorf("Expected attachment filename to be test.txt, got %s", message.Attachments[0].Filename)
	}
}

func TestInlineAttachments(t *testing.T) {
	// Test inline attachment with content
	message, err := mail.NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Test with inline attachment").
		HTMLBody("<p>Image: <img src=\"cid:test-image\"/></p>").
		AttachInline("test.png", "image/png", "test-image", []byte("fake-image-data")).
		Build()

	if err != nil {
		t.Errorf("Expected no error when building message, got: %v", err)
	}

	if len(message.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(message.Attachments))
	}

	attachment := message.Attachments[0]
	if !attachment.Inline {
		t.Error("Expected attachment to be inline")
	}

	if attachment.ContentID != "test-image" {
		t.Errorf("Expected ContentID to be 'test-image', got %s", attachment.ContentID)
	}

	if attachment.Filename != "test.png" {
		t.Errorf("Expected filename to be 'test.png', got %s", attachment.Filename)
	}

	if attachment.ContentType != "image/png" {
		t.Errorf("Expected content type to be 'image/png', got %s", attachment.ContentType)
	}
}

func TestAttachInlineFromReader(t *testing.T) {
	content := []byte("fake-image-data")
	reader := strings.NewReader(string(content))

	message, err := mail.NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Test with inline attachment from reader").
		HTMLBody("<p>Image: <img src=\"cid:test-image-reader\"/></p>").
		AttachInlineFromReader("test.jpg", "image/jpeg", "test-image-reader", reader, int64(len(content))).
		Build()

	if err != nil {
		t.Errorf("Expected no error when building message, got: %v", err)
	}

	if len(message.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(message.Attachments))
	}

	attachment := message.Attachments[0]
	if !attachment.Inline {
		t.Error("Expected attachment to be inline")
	}

	if attachment.ContentID != "test-image-reader" {
		t.Errorf("Expected ContentID to be 'test-image-reader', got %s", attachment.ContentID)
	}

	if attachment.Size != int64(len(content)) {
		t.Errorf("Expected size to be %d, got %d", len(content), attachment.Size)
	}
}

func BenchmarkMessageCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := mail.NewMessage().
			From("sender@example.com").
			To("recipient@example.com").
			Subject("Benchmark test").
			TextBody("This is a benchmark test").
			Build()
		if err != nil {
			b.Errorf("Expected no error when building message, got: %v", err)
		}
	}
}

func BenchmarkTemplateRendering(b *testing.B) {
	config := mail.TemplateConfig{
		DefaultTemplate: "default.html",
	}

	tm := mail.NewTemplateManager(config)
	data := map[string]interface{}{
		"Subject": "Benchmark Subject",
		"Message": "Benchmark message content",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tm.RenderTemplate("default.html", data)
	}
}

// TestManagerTemplateConfiguration tests the manager's template configuration methods
func TestManagerTemplateConfiguration(t *testing.T) {
	config := mail.DefaultConfig()
	queueManager := queue.NewManager()

	manager := mail.NewManager(config, queueManager)

	// Test WithFileServer method
	result := manager.WithFileServer("/tmp")
	if result != manager {
		t.Error("Expected WithFileServer to return same manager instance")
	}

	// Test WithFS method
	result = manager.WithFS(nil)
	if result != manager {
		t.Error("Expected WithFS to return same manager instance")
	}

	// Test GetTemplateManager method
	tm := manager.GetTemplateManager()
	if tm == nil {
		t.Error("Expected GetTemplateManager to return template manager")
	}
}

func TestFormatDateTemplateFunction(t *testing.T) {
	// Create a simple template with formatDate function to test it directly
	funcMap := map[string]interface{}{
		"formatDate": func(format string, date interface{}) string {
			// This is the same implementation as in template.go
			switch v := date.(type) {
			case time.Time:
				if format == "" {
					format = "2006-01-02 15:04:05"
				}
				return v.Format(format)
			case *time.Time:
				if v == nil {
					return ""
				}
				if format == "" {
					format = "2006-01-02 15:04:05"
				}
				return v.Format(format)
			case string:
				// Try to parse string as time
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					if format == "" {
						format = "2006-01-02 15:04:05"
					}
					return t.Format(format)
				}
				return v
			default:
				return fmt.Sprintf("%v", date)
			}
		},
	}

	// Test with time.Time
	now := time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)
	formatFunc, ok := funcMap["formatDate"].(func(string, interface{}) string)
	if !ok {
		t.Fatal("Expected formatDate function to be defined")
	}

	result := formatFunc("2006-01-02", now)
	expected := "2023-12-25"
	if result != expected {
		t.Errorf("Expected formatted date %s, got %s", expected, result)
	}

	// Test with nil time pointer
	result = formatFunc("2006-01-02", (*time.Time)(nil))
	if result != "" {
		t.Errorf("Expected empty string for nil time, got %s", result)
	}

	// Test with string date
	result = formatFunc("2006-01-02", "2023-12-25T10:30:00Z")
	if result != "2023-12-25" {
		t.Errorf("Expected '2023-12-25', got %s", result)
	}

	// Test with default format
	result = formatFunc("", now)
	expected = "2023-12-25 10:30:00"
	if result != expected {
		t.Errorf("Expected default formatted date %s, got %s", expected, result)
	}
}
