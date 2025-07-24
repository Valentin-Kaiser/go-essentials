package mail

import (
	"crypto/tls"
	"time"
)

// Config holds the configuration for the mail package
type Config struct {
	// SMTP Client Configuration
	SMTP SMTPConfig `yaml:"smtp" json:"smtp"`

	// SMTP Server Configuration
	Server ServerConfig `yaml:"server" json:"server"`

	// Queue Configuration
	Queue QueueConfig `yaml:"queue" json:"queue"`

	// Templates Configuration
	Templates TemplateConfig `yaml:"templates" json:"templates"`
}

// SMTPConfig holds the SMTP client configuration for sending emails
type SMTPConfig struct {
	// Host is the SMTP server hostname
	Host string `yaml:"host" json:"host"`

	// Port is the SMTP server port
	Port int `yaml:"port" json:"port"`

	// Username for SMTP authentication
	Username string `yaml:"username" json:"username"`

	// Password for SMTP authentication
	Password string `yaml:"password" json:"password"`

	// From address for outgoing emails
	From string `yaml:"from" json:"from"`

	// FQDN for HELO command
	FQDN string `yaml:"fqdn" json:"fqdn"`

	// Authentication enabled
	Auth bool `yaml:"auth" json:"auth"`

	// AuthMethod defines the authentication method (PLAIN, CRAMMD5, LOGIN)
	AuthMethod string `yaml:"auth_method" json:"auth_method"`

	// Encryption method (NONE, STARTTLS, TLS)
	Encryption string `yaml:"encryption" json:"encryption"`

	// SkipCertificateVerification skips TLS certificate verification
	SkipCertificateVerification bool `yaml:"skip_cert_verification" json:"skip_cert_verification"`

	// Timeout for SMTP operations
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// MaxRetries for failed email sending
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// RetryDelay between retries
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`
}

// ServerConfig holds the SMTP server configuration
type ServerConfig struct {
	// Enabled indicates if the SMTP server should be started
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Host to bind the server to
	Host string `yaml:"host" json:"host"`

	// Port to bind the server to
	Port int `yaml:"port" json:"port"`

	// Domain name for the server
	Domain string `yaml:"domain" json:"domain"`

	// Authentication required for incoming messages
	Auth bool `yaml:"auth" json:"auth"`

	// Username for server authentication
	Username string `yaml:"username" json:"username"`

	// Password for server authentication
	Password string `yaml:"password" json:"password"`

	// TLS encryption enabled
	TLS bool `yaml:"tls" json:"tls"`

	// Certificate file path for TLS
	CertFile string `yaml:"cert_file" json:"cert_file"`

	// Key file path for TLS
	KeyFile string `yaml:"key_file" json:"key_file"`

	// ReadTimeout for server connections
	ReadTimeout time.Duration `yaml:"read_timeout" json:"read_timeout"`

	// WriteTimeout for server connections
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`

	// MaxMessageBytes is the maximum size of a message
	MaxMessageBytes int64 `yaml:"max_message_bytes" json:"max_message_bytes"`

	// MaxRecipients is the maximum number of recipients per message
	MaxRecipients int `yaml:"max_recipients" json:"max_recipients"`

	// AllowInsecureAuth allows authentication over non-TLS connections
	AllowInsecureAuth bool `yaml:"allow_insecure_auth" json:"allow_insecure_auth"`
}

// QueueConfig holds the queue configuration for mail processing
type QueueConfig struct {
	// Enabled indicates if queue processing should be used
	Enabled bool `yaml:"enabled" json:"enabled"`

	// WorkerCount is the number of workers processing mail jobs
	WorkerCount int `yaml:"worker_count" json:"worker_count"`

	// QueueName is the name of the queue for mail jobs
	QueueName string `yaml:"queue_name" json:"queue_name"`

	// Priority for mail jobs
	Priority int `yaml:"priority" json:"priority"`

	// MaxAttempts for failed mail jobs
	MaxAttempts int `yaml:"max_attempts" json:"max_attempts"`

	// JobTimeout for mail job processing
	JobTimeout time.Duration `yaml:"job_timeout" json:"job_timeout"`
}

// TemplateConfig holds the template configuration
type TemplateConfig struct {
	// TemplatesPath is the path to custom email templates
	TemplatesPath string `yaml:"templates_path" json:"templates_path"`

	// DefaultTemplate is the name of the default template
	DefaultTemplate string `yaml:"default_template" json:"default_template"`

	// AutoReload indicates if templates should be reloaded on change
	AutoReload bool `yaml:"auto_reload" json:"auto_reload"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		SMTP: SMTPConfig{
			Host:                        "localhost",
			Port:                        587,
			From:                        "noreply@example.com",
			FQDN:                        "localhost",
			Auth:                        false,
			AuthMethod:                  "PLAIN",
			Encryption:                  "STARTTLS",
			SkipCertificateVerification: false,
			Timeout:                     30 * time.Second,
			MaxRetries:                  3,
			RetryDelay:                  5 * time.Second,
		},
		Server: ServerConfig{
			Enabled:           false,
			Host:              "localhost",
			Port:              2525,
			Domain:            "localhost",
			Auth:              false,
			TLS:               false,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			MaxMessageBytes:   10 * 1024 * 1024, // 10MB
			MaxRecipients:     100,
			AllowInsecureAuth: false,
		},
		Queue: QueueConfig{
			Enabled:     true,
			WorkerCount: 5,
			QueueName:   "mail",
			Priority:    1,
			MaxAttempts: 3,
			JobTimeout:  60 * time.Second,
		},
		Templates: TemplateConfig{
			TemplatesPath:   "templates",
			DefaultTemplate: "default.html",
			AutoReload:      true,
		},
	}
}

// TLSConfig returns a TLS configuration for the SMTP client
func (c *SMTPConfig) TLSConfig() *tls.Config {
	return &tls.Config{
		ServerName:         c.Host,
		InsecureSkipVerify: c.SkipCertificateVerification,
		MinVersion:         tls.VersionTLS12,
	}
}

// TLSConfig returns a TLS configuration for the SMTP server
func (c *ServerConfig) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}
