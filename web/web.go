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
//		web.Get().
//			WithHost("localhost").
//			WithPort(8088).
//			WithSecurityHeaders().
//			WithCORSHeaders().
//			WithGzip().
//			WithLogRequest().
//			WithHandlerFunc("/", handler).
//			StartAsync(done)
//
//		if err := <-done; err != nil {
//			panic(err)
//		}
//
//		err := web.Get().Stop().Error
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
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/Valentin-Kaiser/go-essentials/apperror"
	"github.com/Valentin-Kaiser/go-essentials/interruption"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var instance *Server

// Server represents a web server with a set of middlewares and handlers
type Server struct {
	// Error is set if the server fails to start
	Error       error
	server      *http.Server
	host        string
	port        uint16
	upgrader    websocket.Upgrader
	tlsConfig   *tls.Config
	mutex       sync.RWMutex
	handler     map[string]http.Handler
	middlewares []func(http.Handler) http.Handler
	websockets  map[string]func(http.ResponseWriter, *http.Request, *websocket.Conn)
	connections map[*websocket.Conn]bool
}

// Get creates a new instance of the web server
// It is a singleton and will return the same instance if called multiple times
func Get() *Server {
	if instance == nil {
		instance = &Server{
			port: 80,
			upgrader: websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true
				},
			},
			handler:     make(map[string]http.Handler),
			middlewares: make([]func(http.Handler) http.Handler, 0),
			websockets:  make(map[string]func(http.ResponseWriter, *http.Request, *websocket.Conn)),
			connections: make(map[*websocket.Conn]bool),
		}
	}
	return instance
}

// Start starts the web server
// It will listen on the address specified in the New function
// All middlewares and handlers that should be registered must be registered before calling this function
func (s *Server) Start() *Server {
	defer interruption.Handle()

	if s.Error != nil {
		return s
	}

	s.server = &http.Server{
		ErrorLog:          l.New(io.Discard, "", 0),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	r := NewRouter()
	s.registerMiddlewares(r)
	s.registerHandlers(r)
	s.registerWebsockets(r)
	s.server.Handler = r.ServeMux

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
// It will listen on the address specified in the New function
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
			return
		}

		done <- nil
	}()
}

// Stop stops the web server
// Close immediately closes all active connections in state. For a graceful shutdown, use Shutdown.
// Close does not attempt to close any hijacked connections, such as WebSockets.
func (s *Server) Stop() *Server {
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
func (s *Server) Shutdown() *Server {
	defer interruption.Handle()

	if s.Error != nil {
		return s
	}

	err := s.closeWSConnections().Error
	if err != nil {
		s.Error = apperror.Wrap(err)
		return s
	}

	log.Trace().Msgf("[Web] shutting down webserver...")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	err = s.server.Shutdown(shutdownCtx)
	if err != nil {
		s.Error = apperror.NewError("failed to shutdown webserver").AddError(err)
		return s
	}

	log.Trace().Msgf("[Web] server stopped")
	return s
}

// Restart gracefully shuts down the web server and starts it again
// It will wait for all active connections to finish before shutting down
func (s *Server) Restart() *Server {
	defer interruption.Handle()

	if s.Error != nil {
		return s
	}

	err := s.closeWSConnections().Error
	if err != nil {
		s.Error = apperror.Wrap(err)
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

// WithSecurityHeaders adds security headers to the server
func (s *Server) WithSecurityHeaders() *Server {
	s.middlewares = append(s.middlewares, securityHeader)
	return s
}

// WithCORSHeaders adds CORS headers to the server
func (s *Server) WithCORSHeaders() *Server {
	s.middlewares = append(s.middlewares, corsHeader)
	return s
}

// WithHeader adds a custom header to the server
func (s *Server) WithHeader(key, value string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.middlewares = append(s.middlewares, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, value)
			next.ServeHTTP(w, r)
		})
	})
	return s
}

func (s *Server) WithTLS(config *tls.Config) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.tlsConfig = config
	return s
}

// WithHeaders adds multiple custom headers to the server
func (s *Server) WithHeaders(headers map[string]string) *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.middlewares = append(s.middlewares, func(next http.Handler) http.Handler {
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
	s.middlewares = append(s.middlewares, gziphandler.GzipHandler)
	return s
}

// WithLogRequest adds a logging middleware to the server
func (s *Server) WithLogRequest() *Server {
	s.middlewares = append(s.middlewares, logRequest)
	return s
}

// WithCustomMiddleware adds a custom middleware to the server
func (s *Server) WithCustomMiddleware(middleware func(http.Handler) http.Handler) *Server {
	s.middlewares = append(s.middlewares, middleware)
	return s
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
	return s
}

// WithStaticFS adds a static file server to the server
// It will serve files from the static embed.FS at the specified entrypoints
func (s *Server) WithStaticFS(entrypoints []string, static embed.FS) *Server {
	staticHandler := s.createStaticHandler(static)
	for _, entrypoint := range entrypoints {
		err := s.WithHandler(entrypoint, staticHandler).Error
		if err != nil {
			s.Error = apperror.Wrap(err)
			return s
		}
	}
	return s
}

// WithWebsocket adds a websocket handler to the server
// It will return an error in the Error field if the path is already registered as a handler or a websocket
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
	return s
}

func (s *Server) registerMiddlewares(r *Router) {
	for _, middleware := range s.middlewares {
		r.Use(middleware)
	}
}

func (s *Server) registerHandlers(r *Router) {
	for entrypoint := range s.handler {
		r.Handle(entrypoint, s.handler[entrypoint])
	}
}

func (s *Server) registerWebsockets(r *Router) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for path := range s.websockets {
		r.HandleFunc(path, s.wsHandler)
	}
}

func (s *Server) createStaticHandler(static embed.FS) http.Handler {
	fs, err := fs.Sub(static, "static")
	if err != nil {
		log.Fatal().Err(apperror.Wrap(err)).Msg("[server-createStaticHandler] could not create static handler")
	}
	return gziphandler.GzipHandler(securityHeader(http.FileServer(http.FS(fs))))
}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(apperror.Wrap(err)).Msg("could not upgrade websocket connection")
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Error().Err(apperror.Wrap(err)).Msg("could not close websocket connection")
		}
	}()

	s.connections[conn] = true

	s.mutex.RLock()
	defer s.mutex.RUnlock()
	if handler, ok := s.websockets[r.URL.Path]; ok {
		handler(w, r, conn)
		return
	}
	log.Error().Msgf("no handler registered for %s", r.URL.Path)
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseUnsupportedData, ""))
}

func (s *Server) closeWSConnections() *Server {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for conn := range s.connections {
		err := conn.Close()
		if err != nil {
			s.Error = apperror.NewError("failed to close websocket connection").AddError(err)
			return s
		}
		delete(s.connections, conn)
	}

	return s
}
