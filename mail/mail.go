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
