package mail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Valentin-Kaiser/go-core/queue"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

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
	message := NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Test Subject").
		TextBody("Test message").
		Priority(PriorityHigh).
		Build()

	if message.From != "sender@example.com" {
		t.Errorf("Expected from to be sender@example.com, got %s", message.From)
	}

	if len(message.To) != 1 || message.To[0] != "recipient@example.com" {
		t.Errorf("Expected to to be [recipient@example.com], got %v", message.To)
	}

	if message.Subject != "Test Subject" {
		t.Errorf("Expected subject to be 'Test Subject', got %s", message.Subject)
	}

	if message.Priority != PriorityHigh {
		t.Errorf("Expected priority to be high, got %v", message.Priority)
	}
}

func TestTemplateManager(t *testing.T) {
	config := TemplateConfig{
		DefaultTemplate: "default.html",
		AutoReload:      true,
	}

	tm := NewTemplateManager(config)

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
	config := TemplateConfig{
		DefaultTemplate: "test.html",
		AutoReload:      true,
	}

	tm := NewTemplateManager(config)

	// Test that WithFS method works (we can't test with real filesystem without test data)
	result := tm.WithFS(nil)
	if result != tm {
		t.Error("Expected WithFS to return same template manager instance")
	}
} // TestTemplateManagerWithFileServer tests the template manager with file server
func TestTemplateManagerWithFileServer(t *testing.T) {
	// Create a temporary directory with a test template
	tempDir, err := os.MkdirTemp("", "mail_templates_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test template file
	templateContent := `<html><body><h1>{{.Subject}}</h1><p>{{.Message}}</p></body></html>`
	templatePath := filepath.Join(tempDir, "test.html")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to write test template: %v", err)
	}

	config := TemplateConfig{
		DefaultTemplate: "test.html",
		AutoReload:      true,
	}

	tm := NewTemplateManager(config)

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
	config := SMTPConfig{
		Host:       "localhost",
		Port:       587,
		From:       "sender@example.com",
		Auth:       false,
		Encryption: "NONE",
		MaxRetries: 3,
		RetryDelay: time.Second,
	}

	templateConfig := TemplateConfig{
		DefaultTemplate: "default.html",
	}

	tm := NewTemplateManager(templateConfig)
	sender := NewSMTPSender(config, tm)

	if sender == nil {
		t.Error("Expected sender to be created, got nil")
	}

	// Test message validation
	message := &Message{} // Empty message should fail validation

	ctx := context.Background()
	err := sender.Send(ctx, message)
	if err == nil {
		t.Error("Expected validation error for empty message")
	}
}

func TestMailManager(t *testing.T) {
	config := DefaultConfig()
	config.Queue.Enabled = false // Disable queue for this test

	queueManager := queue.NewManager()
	manager := NewManager(config, queueManager)

	if manager == nil {
		t.Error("Expected manager to be created, got nil")
	}

	if manager.IsRunning() {
		t.Error("Expected manager to not be running initially")
	}

	stats := manager.GetStats()
	if stats == nil {
		t.Error("Expected stats to be available")
	}

	if stats.SentCount != 0 {
		t.Errorf("Expected sent count to be 0, got %d", stats.SentCount)
	}
}

func TestPriorityString(t *testing.T) {
	tests := []struct {
		priority Priority
		expected string
	}{
		{PriorityLow, "low"},
		{PriorityNormal, "normal"},
		{PriorityHigh, "high"},
		{PriorityCritical, "critical"},
	}

	for _, test := range tests {
		if test.priority.String() != test.expected {
			t.Errorf("Expected priority %d to be %s, got %s", test.priority, test.expected, test.priority.String())
		}
	}
}

func TestAttachmentBuilder(t *testing.T) {
	attachment := Attachment{
		Filename:    "test.txt",
		ContentType: "text/plain",
		Content:     []byte("test content"),
		Size:        12,
		Inline:      false,
	}

	message := NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Test with attachment").
		TextBody("Please see attachment").
		Attach(attachment).
		Build()

	if len(message.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(message.Attachments))
	}

	if message.Attachments[0].Filename != "test.txt" {
		t.Errorf("Expected attachment filename to be test.txt, got %s", message.Attachments[0].Filename)
	}
}

func BenchmarkMessageCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewMessage().
			From("sender@example.com").
			To("recipient@example.com").
			Subject("Benchmark test").
			TextBody("This is a benchmark test").
			Build()
	}
}

func BenchmarkTemplateRendering(b *testing.B) {
	config := TemplateConfig{
		DefaultTemplate: "default.html",
	}

	tm := NewTemplateManager(config)
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
	config := DefaultConfig()
	queueManager := queue.NewManager()

	manager := NewManager(config, queueManager)

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
