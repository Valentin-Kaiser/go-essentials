// Package web provides a configurable HTTP web server with built-in support for
// common middleware patterns, file serving, and WebSocket handling.
//
// This package offers a singleton Server instance that can be customized through
// fluent-style methods for setting host, port, headers, middleware, handlers, and WebSocket routes.
//
// Features include:
//   - Serving static files from embedded file systems
//   - Middleware support including security headers, CORS, gzip compression, and request logging
//   - Easy registration of HTTP handlers and handler functions
//   - WebSocket support with connection management and custom handlers
//   - Graceful shutdown and restart capabilities
//
// Example usage:
//
// package main
//
// import (
//
//	"net/http"
//
//	"github.com/Valentin-Kaiser/go-core/web"
//
// )
//
//	func main() {
//		done := make(chan error)
//		web.Server().
//			WithHost("localhost").
//			WithPort(8088).
//			WithSecurityHeaders().
//			WithCORSHeaders().
//			WithGzip().
//			WithLog().
//			WithHandlerFunc("/", handler).
//			StartAsync(done)
//
//		if err := <-done; err != nil {
//			panic(err)
//		}
//
//		err := web.Server().Stop()
//		if err != nil {
//			panic(err)
//		}
//	}
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//		w.Header().Set("Content-Type", "text/plain")
//		w.Write([]byte("Hello, World!"))
//	}
package web

import (
	"context"
	"crypto/tls"
	"crypto/x509/pkix"
	"embed"
	"fmt"
	"io"
	"io/fs"
	l "log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/interruption"
	"github.com/Valentin-Kaiser/go-core/security"
	"github.com/Valentin-Kaiser/go-core/zlog"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

var instance = &Server{
	port:   80,
	router: NewRouter(),
	upgrader: websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			return true
		},
	},
	readTimeout:       15 * time.Second,
	readHeaderTimeout: 5 * time.Second,
	writeTimeout:      15 * time.Second,
	idleTimeout:       120 * time.Second,
	errorLog:          l.New(io.Discard, "", 0),
	handler:           make(map[string]http.Handler),
	websockets:        make(map[string]func(http.ResponseWriter, *http.Request, *websocket.Conn)),
	once:              sync.Once{},
	onHTTPCode:        make(map[string]map[int]func(http.ResponseWriter, *http.Request)),
}

// Server represents a web server with a set of middlewares and handlers
type Server struct {
	// Error is the error that occurred during the server's operation
	// It will be nil if no error occurred
	Error             error
	server            *http.Server
	redirect          *http.Server // Server for protocol redirects (HTTP to HTTPS)
	router            *Router
	mutex             sync.RWMutex
	host              string
	port              uint16
	upgrader          websocket.Upgrader
	tlsConfig         *tls.Config
	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	errorLog          *l.Logger
	handler           map[string]http.Handler
	websockets        map[string]func(http.ResponseWriter, *http.Request, *websocket.Conn)
	once              sync.Once
	onHTTPCode        map[string]map[int]func(http.ResponseWriter, *http.Request)
}

// Instance returns the singleton instance of the web server
func Instance() *Server {
	return instance
}

// Start starts the web server
// All middlewares and handlers that should be registered must be registered before calling this function
func (s *Server) Start() *Server {
	defer interruption.Handle()

	if s.Error != nil {
		return s
	}

	s.server = &http.Server{
		ErrorLog:          s.errorLog,
		ReadTimeout:       s.readTimeout,
		ReadHeaderTimeout: s.readHeaderTimeout,
		WriteTimeout:      s.writeTimeout,
		IdleTimeout:       s.idleTimeout,
		Handler:           s.router,
	}

	if s.tlsConfig != nil {
		g, _ := errgroup.WithContext(context.Background())
		if s.redirect != nil {
			g.Go(func() error {
				log.Info().Msgf("[Web] redirecting HTTP to HTTPS at %s", s.redirect.Addr)
				err := s.redirect.ListenAndServe()
				if err != nil && err != http.ErrServerClosed {
					return apperror.NewError("failed to start redirect server").AddError(err)
				}
				return nil
			})
		}

		g.Go(func() error {
			s.server.TLSConfig = s.tlsConfig
			s.server.Addr = fmt.Sprintf("%s:%d", s.host, s.port)
			log.Info().Msgf("[Web] listening on https at %s", s.server.Addr)

			err := s.server.ListenAndServeTLS("", "")
			if err != nil && err != http.ErrServerClosed {
				return apperror.NewError("failed to start webserver").AddError(err)
			}
			return nil
		})

		if err := g.Wait(); err != nil {
			s.Error = err
		}
		return s
	}

	log.Info().Msgf("[Web] listening on http at %s:%d", s.host, s.port)
	s.server.Addr = fmt.Sprintf("%s:%d", s.host, s.port)
	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		s.Error = apperror.NewError("failed to start webserver").AddError(err)
	}

	return s
}

// StartAsync starts the web server asynchronously
// It will return immediately and the server will run in the background
func (s *Server) StartAsync(done chan error) {
	defer interruption.Handle()

	if s.Error != nil {
		done <- s.Error
		return
	}

	go func() {
		err := s.Start().Error
		if err != nil {
			done <- err
			s.Error = nil
			return
		}

		done <- nil
	}()
}

// Stop stops the web server
// Close does not attempt to close any hijacked connections, such as WebSockets.
func (s *Server) Stop() error {
	defer interruption.Handle()
	if s.server != nil {
		err := s.server.Close()
		if err != nil {
			return apperror.NewError("failed to stop webserver").AddError(err)
		}

		log.Trace().Msgf("[Web] server stopped")
	}
	return nil
}

// Shutdown gracefully shuts down the web server
// It will wait for all active connections to finish before shutting down
// Make sure the program doesn't exit and waits instead for Shutdown to return
func (s *Server) Shutdown() error {
	defer interruption.Handle()
	if s.server != nil {
		log.Trace().Msgf("[Web] shutting down webserver...")
		shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownRelease()

		err := s.server.Shutdown(shutdownCtx)
		if err != nil {
			return apperror.NewError("failed to shutdown webserver").AddError(err)
		}

		log.Trace().Msgf("[Web] server stopped")
	}
	return nil
}

// Restart gracefully shuts down the web server and starts it again
// It will wait for all active connections to finish before shutting down
func (s *Server) Restart() error {
	defer interruption.Handle()
	if s.server != nil {
		log.Trace().Msgf("[Web] restarting webserver...")
		shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownRelease()

		err := s.server.Shutdown(shutdownCtx)
		if err != nil {
			return apperror.NewError("failed to shutdown webserver").AddError(err)
		}
	}

	err := s.Start().Error
	if err != nil {
		return apperror.NewError("failed to start webserver").AddError(err)
	}

	return nil
}

// RestartAsync gracefully shuts down the web server and starts it again asynchronously
// It will wait for all active connections to finish before shutting down
func (s *Server) RestartAsync(done chan error) {
	defer interruption.Handle()

	if s.server != nil {
		log.Trace().Msgf("[Web] restarting webserver...")
		shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownRelease()

		err := s.server.Shutdown(shutdownCtx)
		if err != nil {
			done <- apperror.NewError("failed to shutdown webserver").AddError(err)
			return
		}
	}

	s.StartAsync(done)
}

// WithHandler adds a custom handler to the server
// It will return an error in the Error field if the path is already registered as a handler or a websocket
func (s *Server) WithHandler(path string, handler http.Handler) *Server {
	if s.Error != nil {
		return s
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.handler[path]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a handler", path)
		return s
	}
	if _, ok := s.websockets[path]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a websocket", path)
		return s
	}
	s.handler[path] = handler
	s.router.Handle(path, handler)
	return s
}

// WithHandlerFunc adds a custom handler function to the server
// It will return an error in the Error field if the path is already registered as a handler or a websocket
func (s *Server) WithHandlerFunc(path string, handler http.HandlerFunc) *Server {
	if s.Error != nil {
		return s
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.handler[path]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a handler", path)
		return s
	}
	if _, ok := s.websockets[path]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a websocket", path)
		return s
	}
	s.handler[path] = handler
	s.router.HandleFunc(path, handler)
	return s
}

// WithEmbedFS adds a static file server to the server
// It will serve files from the static embed.FS at the specified entrypoints
func (s *Server) WithEmbedFS(entrypoints []string, static embed.FS) *Server {
	if s.Error != nil {
		return s
	}

	fs, err := fs.Sub(static, "static")
	if err != nil {
		s.Error = apperror.NewError("failed to create sub fs").AddError(err)
		return s
	}
	for _, entrypoint := range entrypoints {
		err := s.WithHandler(entrypoint, http.FileServer(http.FS(fs))).Error
		if err != nil {
			s.Error = apperror.Wrap(err)
			return s
		}
	}
	return s
}

// WithFileServer serves files from the specified directory
// It will fail if the directory does not exist
func (s *Server) WithFileServer(entrypoints []string, path string) *Server {
	if s.Error != nil {
		return s
	}

	_, err := os.Stat(path)
	if err != nil {
		s.Error = apperror.NewErrorf("directory %s does not exist", path).AddError(err)
		return s
	}

	fs := http.FileServer(http.Dir(filepath.Clean(path)))
	for _, entrypoint := range entrypoints {
		err := s.WithHandler(entrypoint, http.StripPrefix(entrypoint, fs)).Error
		if err != nil {
			s.Error = apperror.Wrap(err)
			return s
		}
	}
	return s
}

// WithWebsocket adds a websocket handler to the server
// It will return an error in the Error field if the path is already registered as a handler or a websocket
// The handler function will be called with the http.ResponseWriter, http.Request and *websocket.Conn
// and will be responsible for handling and closing the connection
func (s *Server) WithWebsocket(path string, handler func(http.ResponseWriter, *http.Request, *websocket.Conn)) *Server {
	if s.Error != nil {
		return s
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.handler[path]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a handler", path)
		return s
	}
	if _, ok := s.websockets[path]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a websocket", path)
		return s
	}

	s.websockets[path] = handler
	s.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(apperror.Wrap(err)).Msg("could not upgrade websocket connection")
			return
		}

		handler(w, r, conn)
	})

	return s
}

// WithHost sets the address of the web server
func (s *Server) WithHost(address string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.host = address
	return s
}

// WithPort sets the port of the web server
func (s *Server) WithPort(port uint16) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.port = port
	return s
}

// WithTLS sets the TLS configuration for the web server
func (s *Server) WithTLS(config *tls.Config) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.tlsConfig = config
	return s
}

// WithSelfSignedTLS generates a self-signed TLS certificate for the web server
// It will use the host set by WithHost as the common name for the certificate
func (s *Server) WithSelfSignedTLS() *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Error != nil {
		return s
	}

	cert, pool, err := security.GenerateSelfSignedCertificate(pkix.Name{CommonName: s.host})
	if err != nil {
		s.Error = apperror.Wrap(err)
		return s
	}
	s.tlsConfig = security.NewTLSConfig(cert, pool, tls.NoClientCert)
	return s
}

// WithRedirectToHTTPS sets up a redirect server that redirects HTTP requests to HTTPS
// It must be called after WithPort and WithTLS or else it will return an error
func (s *Server) WithRedirectToHTTPS(port uint16) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Error != nil {
		return s
	}

	if s.tlsConfig == nil {
		s.Error = apperror.NewError("redirect server requires TLS configuration to be set")
		return s
	}

	if s.port == port {
		s.Error = apperror.NewErrorf("redirect server port %d cannot be the same as the main server port", port)
		return s
	}

	if s.redirect != nil {
		s.Error = apperror.NewErrorf("redirect server is already set to %d", port)
		return s
	}

	s.redirect = &http.Server{
		Addr: fmt.Sprintf("%s:%d", s.host, port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				log.Error().Err(apperror.Wrap(err)).Msg("failed to split host and port from request")
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			ip := net.ParseIP(h)
			if ip != nil && ip.To16() != nil && ip.To4() == nil {
				h = "[" + ip.String() + "]"
			}
			log.Trace().Msgf("[Web] redirecting HTTP request from %s to HTTPS", r.URL.Path)
			http.Redirect(w, r, fmt.Sprintf("https://%s:%d%s", h, s.port, r.URL.Path), http.StatusMovedPermanently)
		}),
		ErrorLog:          s.errorLog,
		ReadTimeout:       s.readTimeout,
		ReadHeaderTimeout: s.readHeaderTimeout,
		WriteTimeout:      s.writeTimeout,
		IdleTimeout:       s.idleTimeout,
	}

	return s
}

// WithSecurityHeaders adds security headers to the server
func (s *Server) WithSecurityHeaders() *Server {
	s.router.Use(MiddlewareOrderSecurity, securityHeaderMiddleware)
	return s
}

// WithCORSHeaders adds CORS headers to the server
func (s *Server) WithCORSHeaders() *Server {
	s.router.Use(MiddlewareOrderCors, corsHeaderMiddleware)
	return s
}

// WithHeader adds a custom header to the server
func (s *Server) WithHeader(key, value string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(MiddlewareOrderDefault, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, value)
			next.ServeHTTP(w, r)
		})
	})
	return s
}

// WithHeaders adds multiple custom headers to the server
func (s *Server) WithHeaders(headers map[string]string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(MiddlewareOrderDefault, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for key, value := range headers {
				w.Header().Set(key, value)
			}
			next.ServeHTTP(w, r)
		})
	})
	return s
}

// WithGzip enables gzip compression for the server
func (s *Server) WithGzip() *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(MiddlewareOrderGzip, gziphandler.GzipHandler)
	return s
}

// WithGzipLevel enables gzip compression with a specific level for the server
func (s *Server) WithGzipLevel(level int) *Server {
	if s.Error != nil {
		return s
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	handler, err := gziphandler.NewGzipLevelHandler(level)
	if err != nil {
		s.Error = apperror.NewError("failed to create gzip handler").AddError(err)
		return s
	}

	s.router.Use(MiddlewareOrderGzip, handler)
	return s
}

// WithLog adds a logging middleware to the server
func (s *Server) WithLog() *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(MiddlewareOrderLog, logMiddleware)
	return s
}

// WithRateLimit applies a rate limit to the given pattern
// The limiter allows `count` events per `period` duration
// Example: WithRateLimit("/api", 10, time.Minute) allows 10 requests per minute
func (s *Server) WithRateLimit(pattern string, count int, period time.Duration) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Error != nil {
		return s
	}

	if count <= 0 || period <= 0 {
		s.Error = apperror.NewErrorf("invalid rate limit parameters: count=%d, period=%v", count, period)
		return s
	}

	err := s.router.registerRateLimit(pattern, rate.Every(period/time.Duration(count)), count)
	if err != nil {
		s.Error = apperror.Wrap(err)
	}
	return s
}

// WithCanonicalRedirect sets a canonical redirect domain for the server
// If a request is made to the server with a different domain, it will be redirected to the canonical domain
// This is useful for SEO purposes and to avoid duplicate content issues
func (s *Server) WithCanonicalRedirect(domain string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Error != nil {
		return s
	}

	if s.router.canonicalDomain != "" {
		s.Error = apperror.NewErrorf("canonical redirect domain is already set to %s", s.router.canonicalDomain)
		return s
	}

	s.router.canonicalDomain = domain
	return s
}

// WithHoneypot registers a handler for the given pattern
// Clients that access this pattern will be blocked
// This is useful for detecting and blocking malicious bots or crawlers
func (s *Server) WithHoneypot(pattern string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Error != nil {
		return s
	}

	if _, ok := s.handler[pattern]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a handler", pattern)
		return s
	}
	if _, ok := s.websockets[pattern]; ok {
		s.Error = apperror.NewErrorf("path %s is already registered as a websocket", pattern)
		return s
	}

	s.router.HandleFunc(pattern, s.router.honeypot)
	return s
}

// WithOnHoneypot registers a callback function that will be called when a honeypot is triggered
// The callback will receives the blocklist used. Useful for persisting the blocklist
func (s *Server) WithOnHoneypot(callback func(map[string]*net.IPNet)) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Error != nil {
		return s
	}

	if s.router.honeypotCallback != nil {
		s.Error = apperror.NewError("honeypot callback is already set")
		return s
	}

	s.router.honeypotCallback = callback
	return s
}

// SetWhitelist registers a whitelist for the server
// Addresses in the whitelist will be ignored by the rate limit and honey pot
func (s *Server) SetWhitelist(list []string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.Error != nil {
		return s
	}

	err := s.router.setWhitelist(list)
	if err != nil {
		s.Error = apperror.Wrap(err)
	}
	return s
}

// SetBlacklist registers a blacklist for the server
// Addresses in the blacklist will be blocked from accessing the server
// This is useful for blocking known malicious IPs or ranges
func (s *Server) SetBlacklist(list []string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.Error != nil {
		return s
	}

	err := s.router.setBlacklist(list)
	if err != nil {
		s.Error = apperror.Wrap(err)
	}
	return s
}

// WithCustomMiddleware adds a custom middleware to the server
func (s *Server) WithCustomMiddleware(middleware Middleware) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(MiddlewareOrderDefault, middleware)
	return s
}

// WithCustomMiddlewareOrder adds a custom middleware to the server with a specific order
// The order is a int8 value that determines the order of the middleware 0 is the original handler,
// -128 is the beginning of the chain, and 127 is the end of the chain
func (s *Server) WithCustomMiddlewareOrder(order MiddlewareOrder, middleware Middleware) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(order, middleware)
	return s
}

// WithReadTimeout sets the read timeout for the server
// It will be used for all requests and connections
func (s *Server) WithReadTimeout(timeout time.Duration) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.readTimeout = timeout
	return s
}

// WithReadHeaderTimeout sets the read header timeout for the server
// It will be used for all requests and connections
func (s *Server) WithReadHeaderTimeout(timeout time.Duration) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.readHeaderTimeout = timeout
	return s
}

// WithWriteTimeout sets the write timeout for the server
// It will be used for all requests and connections
func (s *Server) WithWriteTimeout(timeout time.Duration) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.writeTimeout = timeout
	return s
}

// WithIdleTimeout sets the idle timeout for the server
// It will be used for all requests and connections
func (s *Server) WithIdleTimeout(timeout time.Duration) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.idleTimeout = timeout
	return s
}

// WithErrorLog sets the default error log for the server
// It will log errors at the Error level using the zerolog logger
func (s *Server) WithErrorLog() *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.errorLog = l.New(zlog.Logger().WithLevel(zerolog.ErrorLevel), "", 0)
	return s
}

// WithCustomErrorLog sets the error log for the server
func (s *Server) WithCustomErrorLog(logger *l.Logger) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.errorLog = logger
	return s
}

// WithOnHTTPCode adds a custom handler for a specific HTTP status code and pattern.
// It will be called after the request is handled and before the response is sent,
// and is called with the http.ResponseWriter and the http.Request
func (s *Server) WithOnHTTPCode(code int, pattern []string, handler func(http.ResponseWriter, *http.Request)) *Server {
	if s.Error != nil {
		return s
	}

	if code < http.StatusContinue || code > http.StatusNetworkAuthenticationRequired {
		s.Error = apperror.NewErrorf("invalid HTTP status code %d", code)
		return s
	}

	if len(pattern) == 0 {
		s.Error = apperror.NewError("no pattern provided for HTTP status code handler")
		return s
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, p := range pattern {
		_, ok := s.onHTTPCode[p]
		if !ok {
			s.onHTTPCode[p] = make(map[int]func(http.ResponseWriter, *http.Request))
		}

		_, ok = s.onHTTPCode[p][code]
		if ok {
			s.Error = apperror.NewErrorf("http code %d is already registered for path %s", code, p)
			return s
		}

		s.onHTTPCode[p][code] = handler
		s.router.OnStatus(p, code, handler)
	}
	return s
}
