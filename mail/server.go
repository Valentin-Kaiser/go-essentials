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
	config           ServerConfig
	server           *smtp.Server
	manager          *Manager
	handlers         []NotificationHandler
	running          int32
	mutex            sync.RWMutex
	handlerSemaphore chan struct{}      // Semaphore to limit concurrent handlers
	handlerQueue     chan handlerTask   // Queue for pending handler tasks
	ctx              context.Context    // Context for server lifecycle
	cancel           context.CancelFunc // Cancel function for server lifecycle
	wg               sync.WaitGroup     // WaitGroup for graceful shutdown
}

// handlerTask represents a pending notification handler task
type handlerTask struct {
	handler NotificationHandler
	ctx     context.Context
	from    string
	to      []string
	data    []byte
}

// NewSMTPServer creates a new SMTP server
func NewSMTPServer(config ServerConfig, manager *Manager) Server {
	// Set default for MaxConcurrentHandlers if not specified
	if config.MaxConcurrentHandlers <= 0 {
		config.MaxConcurrentHandlers = 50
	}

	ctx, cancel := context.WithCancel(context.Background())
	server := &smtpServer{
		config:           config,
		manager:          manager,
		handlers:         make([]NotificationHandler, 0),
		handlerSemaphore: make(chan struct{}, config.MaxConcurrentHandlers),
		handlerQueue:     make(chan handlerTask, config.MaxConcurrentHandlers*2), // Queue size = 2x semaphore size
		ctx:              ctx,
		cancel:           cancel,
	}

	// Start worker pool for handling queued notification tasks
	server.startWorkerPool()
	return server
}

// Start starts the SMTP server
func (s *smtpServer) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return apperror.NewError("SMTP server is already running")
	}
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
	serverReady := make(chan error, 1)
	go func() {
		var err error
		if s.config.TLS {
			err = s.server.ListenAndServeTLS()
		} else {
			err = s.server.ListenAndServe()
		}

		if err != nil && err != smtp.ErrServerClosed {
			log.Error().Err(err).Msg("[Mail] SMTP server error")
			select {
			case serverReady <- err:
			default:
			}
		}

		atomic.StoreInt32(&s.running, 0)
	}()

	// Check if server started successfully by attempting to connect
	go func() {
		maxAttempts := 10
		for i := 0; i < maxAttempts; i++ {
			time.Sleep(10 * time.Millisecond)

			// Try to connect to the server
			conn, err := net.DialTimeout("tcp", s.server.Addr, 100*time.Millisecond)
			if err == nil {
				apperror.Catch(conn.Close, "failed to close connection")
				select {
				case serverReady <- nil:
				default:
				}
				return
			}

			// Check if we should stop trying
			if !s.IsRunning() {
				select {
				case serverReady <- apperror.NewError("server stopped before becoming ready"):
				default:
				}
				return
			}
		}

		// Timeout waiting for server to be ready
		select {
		case serverReady <- apperror.NewError("timeout waiting for server to become ready"):
		default:
		}
	}()

	// Wait for server to be ready or fail
	return <-serverReady
}

// Stop stops the SMTP server
func (s *smtpServer) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return apperror.NewError("SMTP server is not running")
	}

	// Cancel context to stop worker pool
	s.cancel()
	if s.server != nil {
		if err := s.server.Close(); err != nil {
			log.Error().Err(err).Msg("[Mail] Failed to close SMTP server")
			return apperror.Wrap(err)
		}
	}

	// Wait for worker pool to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		log.Warn().Msg("[Mail] SMTP server stop timed out waiting for worker pool")
		return ctx.Err()
	}
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
				if err == nil {
					tlsConfig.Certificates = []tls.Certificate{cert}
					return tlsConfig, nil
				}
			}
		}
	}

	cert, err := s.generateSelfSignedCert()
	if err != nil {
		return nil, apperror.Wrap(err)
	}

	tlsConfig.Certificates = []tls.Certificate{cert}
	return tlsConfig, nil
}

// generateSelfSignedCert generates a self-signed certificate
func (s *smtpServer) generateSelfSignedCert() (tls.Certificate, error) {
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
		if err := os.MkdirAll(filepath.Dir(s.config.CertFile), 0750); err != nil {
			log.Warn().Err(err).Msg("[Mail] Failed to create certificate directory")
		} else {
			if err := os.WriteFile(s.config.CertFile, certPEM.Bytes(), 0600); err != nil {
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

// notifyHandlers notifies all registered handlers using worker pool with queueing
func (s *smtpServer) notifyHandlers(ctx context.Context, from string, to []string, data []byte) {
	s.mutex.RLock()
	handlers := make([]NotificationHandler, len(s.handlers))
	copy(handlers, s.handlers)
	s.mutex.RUnlock()

	for _, handler := range handlers {
		task := handlerTask{
			handler: handler,
			ctx:     ctx,
			from:    from,
			to:      to,
			data:    data,
		}

		// Try to queue the task, with timeout to avoid blocking
		select {
		case s.handlerQueue <- task:
			// Successfully queued
		case <-time.After(100 * time.Millisecond):
			// Queue is full and timed out - log warning but continue
			log.Warn().
				Str("from", from).
				Strs("to", to).
				Int("queue_size", len(s.handlerQueue)).
				Int("queue_capacity", cap(s.handlerQueue)).
				Msg("[Mail] Notification handler queue is full, dropping task")
		case <-ctx.Done():
			// Context cancelled
			log.Warn().
				Err(ctx.Err()).
				Str("from", from).
				Strs("to", to).
				Msg("[Mail] Notification handler cancelled due to context")
			return
		case <-s.ctx.Done():
			// Server context cancelled
			log.Warn().
				Str("from", from).
				Strs("to", to).
				Msg("[Mail] Notification handler cancelled due to server shutdown")
			return
		}
	}
}

// startWorkerPool starts the worker pool for processing notification handler tasks
func (s *smtpServer) startWorkerPool() {
	// Start worker pool based on MaxConcurrentHandlers
	for i := 0; i < s.config.MaxConcurrentHandlers; i++ {
		s.wg.Add(1)
		go func(workerID int) {
			defer s.wg.Done()

			for {
				select {
				case task := <-s.handlerQueue:
					// Process the handler task
					if err := task.handler(task.ctx, task.from, task.to, task.data); err != nil {
						log.Error().
							Err(err).
							Str("from", task.from).
							Strs("to", task.to).
							Int("worker_id", workerID).
							Msg("[Mail] Notification handler failed")
					}
				case <-s.ctx.Done():
					log.Trace().
						Int("worker_id", workerID).
						Msg("[Mail] Notification handler worker stopping")
					return
				}
			}
		}(i)
	}
}

// smtpBackend implements the smtp.Backend interface
type smtpBackend struct {
	server *smtpServer
}

// NewSession creates a new SMTP session
func (b *smtpBackend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	log.Trace().
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

	log.Trace().
		Str("username", username).
		Msg("[Mail] SMTP PLAIN authentication attempt")

	if username == s.server.config.Username && password == s.server.config.Password {
		s.authenticated = true
		log.Trace().Str("username", username).Msg("[Mail] SMTP authentication successful")
		return nil
	}

	log.Warn().Str("username", username).Msg("[Mail] SMTP authentication failed")
	return smtp.ErrAuthFailed
}

// Mail handles the MAIL FROM command
func (s *smtpSession) Mail(from string, _ *smtp.MailOptions) error {
	if s.server.config.Auth && !s.authenticated {
		log.Warn().Str("from", from).Msg("[Mail] Unauthenticated MAIL command rejected")
		return smtp.ErrAuthRequired
	}

	log.Trace().Str("from", from).Msg("[Mail] MAIL FROM")
	s.from = from
	return nil
}

// Rcpt handles the RCPT TO command
func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.server.config.Auth && !s.authenticated {
		log.Warn().Str("to", to).Msg("[Mail] Unauthenticated RCPT command rejected")
		return smtp.ErrAuthRequired
	}

	s.to = append(s.to, to)
	return nil
}

// Data handles the email data
func (s *smtpSession) Data(r io.Reader) error {
	if s.server.config.Auth && !s.authenticated {
		log.Warn().Msg("[Mail] Unauthenticated DATA command rejected")
		return smtp.ErrAuthRequired
	}

	// Read message data
	data, err := io.ReadAll(r)
	if err != nil {
		return apperror.NewError("failed to read email data").AddError(err)
	}

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
