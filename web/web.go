// Package web provides a configurable HTTP web server with built-in support for
// common middleware patterns, static file serving, API endpoints, and WebSocket handling.
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
//	"github.com/Valentin-Kaiser/go-essentials/web"
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
//		err := web.Server().Stop().Error
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
	"embed"
	"fmt"
	"io"
	"io/fs"
	l "log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/Valentin-Kaiser/go-essentials/apperror"
	"github.com/Valentin-Kaiser/go-essentials/interruption"
	"github.com/Valentin-Kaiser/go-essentials/zlog"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var instance = &server{
	port:   80,
	router: NewRouter(),
	upgrader: websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
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
}

// server represents a web server with a set of middlewares and handlers
type server struct {
	Error             error
	server            *http.Server
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
}

// Server returns the singleton instance of the web server
func Server() *server {
	return instance
}

// Start starts the web server
// All middlewares and handlers that should be registered must be registered before calling this function
func (s *server) Start() *server {
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
		s.server.TLSConfig = s.tlsConfig
		log.Info().Msgf("[Web] listening on https at %s:%d", s.host, s.port)
		s.server.Addr = fmt.Sprintf("%s:%d", s.host, s.port)
		err := s.server.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			s.Error = apperror.NewError("failed to start webserver").AddError(err)
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
func (s *server) StartAsync(done chan error) {
	defer interruption.Handle()

	if s.Error != nil {
		done <- s.Error
		return
	}

	go func() {
		err := s.Start().Error
		if err != nil {
			done <- err
			return
		}

		done <- nil
	}()
}

// Stop stops the web server
// Close immediately closes all active connections in state. For a graceful shutdown, use Shutdown.
// Close does not attempt to close any hijacked connections, such as WebSockets.
func (s *server) Stop() *server {
	defer interruption.Handle()

	if s.Error != nil {
		return s
	}

	err := s.server.Close()
	if err != nil {
		s.Error = apperror.NewError("failed to stop webserver").AddError(err)
		return s
	}

	log.Trace().Msgf("[Web] server stopped")
	return s
}

// Shutdown gracefully shuts down the web server
// It will wait for all active connections to finish before shutting down
// Make sure the program doesn't exit and waits instead for Shutdown to return
func (s *server) Shutdown() *server {
	defer interruption.Handle()

	if s.Error != nil {
		return s
	}

	log.Trace().Msgf("[Web] shutting down webserver...")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	err := s.server.Shutdown(shutdownCtx)
	if err != nil {
		s.Error = apperror.NewError("failed to shutdown webserver").AddError(err)
		return s
	}

	log.Trace().Msgf("[Web] server stopped")
	return s
}

// Restart gracefully shuts down the web server and starts it again
// It will wait for all active connections to finish before shutting down
func (s *server) Restart() *server {
	defer interruption.Handle()

	if s.Error != nil {
		return s
	}

	log.Trace().Msgf("[Web] restarting webserver...")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := s.server.Shutdown(shutdownCtx); err != nil {
		s.Error = apperror.NewError("failed to shutdown webserver").AddError(err)
		return s
	}

	s.Start()
	return s
}

// WithHandler adds a custom handler to the server
// It will return an error in the Error field if the path is already registered as a handler or a websocket
func (s *server) WithHandler(path string, handler http.Handler) *server {
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
	s.router.Handle(path, handler)
	return s
}

// WithHandlerFunc adds a custom handler function to the server
// It will return an error in the Error field if the path is already registered as a handler or a websocket
func (s *server) WithHandlerFunc(path string, handler http.HandlerFunc) *server {
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
	s.router.HandleFunc(path, handler)
	return s
}

// WithEmbedFS adds a static file server to the server
// It will serve files from the static embed.FS at the specified entrypoints
func (s *server) WithEmbedFS(entrypoints []string, static embed.FS) *server {
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
func (s *server) WithFileServer(entrypoints []string, path string) *server {
	_, err := os.Stat(path)
	if err != nil {
		s.Error = apperror.NewErrorf("directory %s does not exist", path).AddError(err)
		return s
	}

	fs := http.FileServer(http.Dir(path))
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
func (s *server) WithWebsocket(path string, handler func(http.ResponseWriter, *http.Request, *websocket.Conn)) *server {
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
func (s *server) WithHost(address string) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.host = address
	return s
}

// WithPort sets the port of the web server
func (s *server) WithPort(port uint16) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.port = port
	return s
}

func (s *server) WithTLS(config *tls.Config) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.tlsConfig = config
	return s
}

// WithSecurityHeaders adds security headers to the server
func (s *server) WithSecurityHeaders() *server {
	s.router.Use(securityHeaderMiddleware)
	return s
}

// WithCORSHeaders adds CORS headers to the server
func (s *server) WithCORSHeaders() *server {
	s.router.Use(corsHeaderMiddleware)
	return s
}

// WithHeader adds a custom header to the server
func (s *server) WithHeader(key, value string) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, value)
			next.ServeHTTP(w, r)
		})
	})
	return s
}

// WithHeaders adds multiple custom headers to the server
func (s *server) WithHeaders(headers map[string]string) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(func(next http.Handler) http.Handler {
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
func (s *server) WithGzip() *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(gziphandler.GzipHandler)
	return s
}

func (s *server) WithGzipLevel(level int) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	handler, err := gziphandler.NewGzipLevelHandler(level)
	if err != nil {
		s.Error = apperror.NewError("failed to create gzip handler").AddError(err)
		return s
	}

	s.router.Use(handler)
	return s
}

// WithLog adds a logging middleware to the server
func (s *server) WithLog() *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(logMiddleware)
	return s
}

// WithCustomMiddleware adds a custom middleware to the server
func (s *server) WithCustomMiddleware(middleware Middleware) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.router.Use(middleware)
	return s
}

// WithReadTimeout sets the read timeout for the server
// It will be used for all requests and connections
func (s *server) WithReadTimeout(timeout time.Duration) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.readTimeout = timeout
	return s
}

// WithReadHeaderTimeout sets the read header timeout for the server
// It will be used for all requests and connections
func (s *server) WithReadHeaderTimeout(timeout time.Duration) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.readHeaderTimeout = timeout
	return s
}

// WithWriteTimeout sets the write timeout for the server
// It will be used for all requests and connections
func (s *server) WithWriteTimeout(timeout time.Duration) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.writeTimeout = timeout
	return s
}

// WithIdleTimeout sets the idle timeout for the server
// It will be used for all requests and connections
func (s *server) WithIdleTimeout(timeout time.Duration) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.idleTimeout = timeout
	return s
}

func (s *server) WithErrorLog() *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.errorLog = l.New(zlog.Logger().WithLevel(zerolog.ErrorLevel), "", 0)
	return s
}

// WithCustomErrorLog sets the error log for the server
func (s *server) WithCustomErrorLog(logger *l.Logger) *server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.errorLog = logger
	return s
}
