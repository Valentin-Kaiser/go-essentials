package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/emersion/go-smtp"
	"github.com/rs/zerolog/log"
)

// smtpServer implements the Server interface
type smtpServer struct {
	config   ServerConfig
	server   *smtp.Server
	manager  *Manager
	handlers []NotificationHandler
	running  int32
	mutex    sync.RWMutex
}

// NewSMTPServer creates a new SMTP server
func NewSMTPServer(config ServerConfig, manager *Manager) Server {
	return &smtpServer{
		config:   config,
		manager:  manager,
		handlers: make([]NotificationHandler, 0),
	}
}

// Start starts the SMTP server
func (s *smtpServer) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return apperror.NewError("SMTP server is already running")
	}

	log.Info().
		Str("host", s.config.Host).
		Int("port", s.config.Port).
		Str("domain", s.config.Domain).
		Bool("tls", s.config.TLS).
		Bool("auth", s.config.Auth).
		Msg("[Mail] Starting SMTP server")

	// Create SMTP server
	s.server = smtp.NewServer(&smtpBackend{server: s})

	// Configure server
	s.server.Addr = fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	s.server.Domain = s.config.Domain
	s.server.ReadTimeout = s.config.ReadTimeout
	s.server.WriteTimeout = s.config.WriteTimeout
	s.server.MaxMessageBytes = s.config.MaxMessageBytes
	s.server.MaxRecipients = s.config.MaxRecipients
	s.server.AllowInsecureAuth = s.config.AllowInsecureAuth

	// Configure TLS if enabled
	if s.config.TLS {
		tlsConfig, err := s.loadTLSConfig()
		if err != nil {
			atomic.StoreInt32(&s.running, 0)
			return apperror.Wrap(err)
		}
		s.server.TLSConfig = tlsConfig
	}

	// Start server in goroutine
	go func() {
		var err error
		if s.config.TLS {
			log.Info().Msg("[Mail] Starting SMTP server with TLS")
			err = s.server.ListenAndServeTLS()
		} else {
			log.Info().Msg("[Mail] Starting SMTP server without TLS")
			err = s.server.ListenAndServe()
		}

		if err != nil && err != smtp.ErrServerClosed {
			log.Error().Err(err).Msg("[Mail] SMTP server error")
		}

		atomic.StoreInt32(&s.running, 0)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	log.Info().
		Str("addr", s.server.Addr).
		Msg("[Mail] SMTP server started successfully")

	return nil
}

// Stop stops the SMTP server
func (s *smtpServer) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return apperror.NewError("SMTP server is not running")
	}

	log.Info().Msg("[Mail] Stopping SMTP server")

	if s.server != nil {
		if err := s.server.Close(); err != nil {
			log.Error().Err(err).Msg("[Mail] Failed to close SMTP server")
			return apperror.Wrap(err)
		}
	}

	log.Info().Msg("[Mail] SMTP server stopped successfully")
	return nil
}

// AddHandler adds a notification handler
func (s *smtpServer) AddHandler(handler NotificationHandler) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.handlers = append(s.handlers, handler)
}

// IsRunning returns true if the server is running
func (s *smtpServer) IsRunning() bool {
	return atomic.LoadInt32(&s.running) == 1
}

// loadTLSConfig loads or generates TLS configuration
func (s *smtpServer) loadTLSConfig() (*tls.Config, error) {
	tlsConfig := s.config.TLSConfig()

	// Try to load existing certificates
	if s.config.CertFile != "" && s.config.KeyFile != "" {
		if _, err := os.Stat(s.config.CertFile); err == nil {
			if _, err := os.Stat(s.config.KeyFile); err == nil {
				cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
				if err != nil {
					log.Warn().Err(err).Msg("[Mail] Failed to load TLS certificates, generating new ones")
				} else {
					tlsConfig.Certificates = []tls.Certificate{cert}
					log.Info().Msg("[Mail] Loaded existing TLS certificates")
					return tlsConfig, nil
				}
			}
		}
	}

	// Generate self-signed certificates
	log.Info().Msg("[Mail] Generating self-signed TLS certificates")
	cert, err := s.generateSelfSignedCert()
	if err != nil {
		return nil, apperror.Wrap(err)
	}

	tlsConfig.Certificates = []tls.Certificate{cert}
	return tlsConfig, nil
}

// generateSelfSignedCert generates a self-signed certificate
func (s *smtpServer) generateSelfSignedCert() (tls.Certificate, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, apperror.NewError("failed to generate private key").AddError(err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Mail Server"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // Valid for 1 year
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost", s.config.Domain},
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, apperror.NewError("failed to create certificate").AddError(err)
	}

	// Encode certificate
	certPEM := &bytes.Buffer{}
	if err := pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return tls.Certificate{}, apperror.NewError("failed to encode certificate").AddError(err)
	}

	// Encode private key
	keyPEM := &bytes.Buffer{}
	if err := pem.Encode(keyPEM, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}); err != nil {
		return tls.Certificate{}, apperror.NewError("failed to encode private key").AddError(err)
	}

	// Save certificates if paths are specified
	if s.config.CertFile != "" && s.config.KeyFile != "" {
		if err := os.MkdirAll(filepath.Dir(s.config.CertFile), 0755); err != nil {
			log.Warn().Err(err).Msg("[Mail] Failed to create certificate directory")
		} else {
			if err := os.WriteFile(s.config.CertFile, certPEM.Bytes(), 0644); err != nil {
				log.Warn().Err(err).Msg("[Mail] Failed to save certificate file")
			}

			if err := os.WriteFile(s.config.KeyFile, keyPEM.Bytes(), 0600); err != nil {
				log.Warn().Err(err).Msg("[Mail] Failed to save key file")
			}
		}
	}

	// Create TLS certificate
	cert, err := tls.X509KeyPair(certPEM.Bytes(), keyPEM.Bytes())
	if err != nil {
		return tls.Certificate{}, apperror.NewError("failed to create TLS certificate").AddError(err)
	}

	return cert, nil
}

// notifyHandlers notifies all registered handlers
func (s *smtpServer) notifyHandlers(ctx context.Context, from string, to []string, data []byte) {
	s.mutex.RLock()
	handlers := make([]NotificationHandler, len(s.handlers))
	copy(handlers, s.handlers)
	s.mutex.RUnlock()

	for _, handler := range handlers {
		go func(h NotificationHandler) {
			if err := h(ctx, from, to, data); err != nil {
				log.Error().
					Err(err).
					Str("from", from).
					Strs("to", to).
					Msg("[Mail] Notification handler failed")
			}
		}(handler)
	}
}

// smtpBackend implements the smtp.Backend interface
type smtpBackend struct {
	server *smtpServer
}

// NewSession creates a new SMTP session
func (b *smtpBackend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	log.Debug().
		Str("remote_addr", conn.Conn().RemoteAddr().String()).
		Msg("[Mail] New SMTP session")

	return &smtpSession{
		server: b.server,
		conn:   conn,
	}, nil
}

// smtpSession implements the smtp.Session interface
type smtpSession struct {
	server        *smtpServer
	conn          *smtp.Conn
	authenticated bool
	from          string
	to            []string
}

// AuthPlain handles PLAIN authentication
func (s *smtpSession) AuthPlain(username, password string) error {
	if !s.server.config.Auth {
		return smtp.ErrAuthUnsupported
	}

	log.Debug().
		Str("username", username).
		Msg("[Mail] SMTP PLAIN authentication attempt")

	if username == s.server.config.Username && password == s.server.config.Password {
		s.authenticated = true
		log.Debug().Str("username", username).Msg("[Mail] SMTP authentication successful")
		return nil
	}

	log.Warn().Str("username", username).Msg("[Mail] SMTP authentication failed")
	return smtp.ErrAuthFailed
}

// Mail handles the MAIL FROM command
func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	if s.server.config.Auth && !s.authenticated {
		log.Warn().Str("from", from).Msg("[Mail] Unauthenticated MAIL command rejected")
		return smtp.ErrAuthRequired
	}

	log.Debug().Str("from", from).Msg("[Mail] MAIL FROM")
	s.from = from
	return nil
}

// Rcpt handles the RCPT TO command
func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	if s.server.config.Auth && !s.authenticated {
		log.Warn().Str("to", to).Msg("[Mail] Unauthenticated RCPT command rejected")
		return smtp.ErrAuthRequired
	}

	log.Debug().Str("to", to).Msg("[Mail] RCPT TO")
	s.to = append(s.to, to)
	return nil
}

// Data handles the email data
func (s *smtpSession) Data(r io.Reader) error {
	if s.server.config.Auth && !s.authenticated {
		log.Warn().Msg("[Mail] Unauthenticated DATA command rejected")
		return smtp.ErrAuthRequired
	}

	log.Debug().
		Str("from", s.from).
		Strs("to", s.to).
		Msg("[Mail] Receiving email data")

	// Read message data
	data, err := io.ReadAll(r)
	if err != nil {
		log.Error().Err(err).Msg("[Mail] Failed to read email data")
		return apperror.NewError("failed to read email data").AddError(err)
	}

	// Parse email message
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		log.Error().Err(err).Msg("[Mail] Failed to parse email message")
		return apperror.NewError("failed to parse email message").AddError(err)
	}

	// Log received message
	log.Info().
		Str("from", s.from).
		Strs("to", s.to).
		Str("subject", msg.Header.Get("Subject")).
		Int("size", len(data)).
		Msg("[Mail] Email received")

	// Notify manager
	if s.server.manager != nil {
		s.server.manager.NotifyMessageReceived()
	}

	// Notify handlers
	ctx := context.Background()
	s.server.notifyHandlers(ctx, s.from, s.to, data)

	return nil
}

// Reset resets the session
func (s *smtpSession) Reset() {
	log.Debug().Msg("[Mail] SMTP session reset")
	s.from = ""
	s.to = nil
}

// Logout handles session logout
func (s *smtpSession) Logout() error {
	log.Debug().Msg("[Mail] SMTP session logout")
	return nil
}
