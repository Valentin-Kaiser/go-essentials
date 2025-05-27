// web provides a configurable HTTP web server with built-in support for
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
	"embed"
	"fmt"
	"io"
	"io/fs"
	l "log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/interruption"
	"github.com/Valentin-Kaiser/go-core/zlog"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var instance = &Server{
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
	once:              sync.Once{},
	onHttpCode:        make(map[int]func(http.ResponseWriter, *http.Request)),
}

// Server represents a web server with a set of middlewares and handlers
type Server struct {
	// Error is the error that occurred during the server's operation
	// It will be nil if no error occurred
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
	once              sync.Once
	onHttpCode        map[int]func(http.ResponseWriter, *http.Request)
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

// WithOnHttpCode adds a custom handler for a specific HTTP status code
// It will be called after the request is handled and before the response is sent,
// and is called with the http.ResponseWriter and the http.Request
func (s *Server) WithOnHttpCode(code int, handler func(http.ResponseWriter, *http.Request)) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, ok := s.onHttpCode[code]
	if ok {
		s.Error = apperror.NewErrorf("http code %d is already registered", code)
		return s
	}

	s.onHttpCode[code] = handler
	s.router.OnStatus(code, handler)
	return s
}
