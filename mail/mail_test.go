package mail

import (
	"context"
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
		TemplatesPath:   "",
		DefaultTemplate: "default.html",
		AutoReload:      true,
	}

	tm := NewTemplateManager(config)

	// Test loading embedded template
	template, err := tm.LoadTemplate("default.html")
	if err != nil {
		t.Errorf("Failed to load default template: %v", err)
	}

	if template == nil {
		t.Error("Expected template to be loaded, got nil")
	}

	// Test rendering template
	data := map[string]interface{}{
		"Subject": "Test Subject",
		"Message": "Test message content",
	}

	rendered, err := tm.RenderTemplate("default.html", data)
	if err != nil {
		t.Errorf("Failed to render template: %v", err)
	}

	if rendered == "" {
		t.Error("Expected rendered template to have content")
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
