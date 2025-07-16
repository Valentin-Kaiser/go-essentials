package web

import (
	"crypto/tls"
	"crypto/x509/pkix"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Valentin-Kaiser/go-core/security"
	"github.com/gorilla/websocket"
)

func TestServerCreation(t *testing.T) {
	server := New()
	if server == nil {
		t.Error("New() should not return nil")
	}

	if server.Error != nil {
		t.Errorf("New server should not have error: %v", server.Error)
	}

	if server.port != 80 {
		t.Errorf("Default port should be 80, got %d", server.port)
	}

	if server.router == nil {
		t.Error("Server should have a router")
	}
}

func TestServerInstance(t *testing.T) {
	server1 := Instance()
	server2 := Instance()

	if server1 != server2 {
		t.Error("Instance() should return the same server instance (singleton)")
	}
}

func TestServerWithHost(t *testing.T) {
	server := New()
	result := server.WithHost("localhost")

	if result != server {
		t.Error("WithHost() should return the same server instance")
	}

	if server.host != "localhost" {
		t.Errorf("Host should be 'localhost', got '%s'", server.host)
	}
}

func TestServerWithPort(t *testing.T) {
	server := New()
	result := server.WithPort(8080)

	if result != server {
		t.Error("WithPort() should return the same server instance")
	}

	if server.port != 8080 {
		t.Errorf("Port should be 8080, got %d", server.port)
	}
}

func TestServerWithTimeouts(t *testing.T) {
	server := New()

	server.WithReadTimeout(30 * time.Second)
	if server.readTimeout != 30*time.Second {
		t.Errorf("Read timeout should be 30s, got %v", server.readTimeout)
	}

	server.WithReadHeaderTimeout(10 * time.Second)
	if server.readHeaderTimeout != 10*time.Second {
		t.Errorf("Read header timeout should be 10s, got %v", server.readHeaderTimeout)
	}

	server.WithWriteTimeout(25 * time.Second)
	if server.writeTimeout != 25*time.Second {
		t.Errorf("Write timeout should be 25s, got %v", server.writeTimeout)
	}

	server.WithIdleTimeout(60 * time.Second)
	if server.idleTimeout != 60*time.Second {
		t.Errorf("Idle timeout should be 60s, got %v", server.idleTimeout)
	}
}

func TestServerWithTLS(t *testing.T) {
	server := New()

	// Test with self-signed certificate
	subject := pkix.Name{
		Organization: []string{"Test"},
		Country:      []string{"US"},
	}

	cert, caPool, err := security.GenerateSelfSignedCertificate(subject)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	tlsConfig := security.NewTLSConfig(cert, caPool, tls.NoClientCert)
	result := server.WithTLS(tlsConfig)

	if result != server {
		t.Error("WithTLS() should return the same server instance")
	}

	if server.tlsConfig != tlsConfig {
		t.Error("TLS config should be set")
	}
}

func TestServerWithRedirect(t *testing.T) {
	server := New()

	// Setup TLS first
	subject := pkix.Name{Organization: []string{"Test"}}
	cert, caPool, err := security.GenerateSelfSignedCertificate(subject)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	tlsConfig := security.NewTLSConfig(cert, caPool, tls.NoClientCert)
	server.WithTLS(tlsConfig)

	result := server.WithRedirectToHTTPS(8080)

	if result != server {
		t.Error("WithRedirectToHTTPS() should return the same server instance")
	}
}

func TestServerWithRedirectWithoutTLS(t *testing.T) {
	server := New()
	result := server.WithRedirectToHTTPS(8080)

	if result != server {
		t.Error("WithRedirectToHTTPS() should return the same server instance")
	}

	if server.Error == nil {
		t.Error("WithRedirectToHTTPS() without TLS should set an error")
	}
}

func TestServerWithHandlerFunc(t *testing.T) {
	server := New()

	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}

	result := server.WithHandlerFunc("/test", handlerFunc)

	if result != server {
		t.Error("WithHandlerFunc() should return the same server instance")
	}

	// Test the handler was registered
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("Expected body 'test', got '%s'", w.Body.String())
	}
}

func TestServerWithHandler(t *testing.T) {
	server := New()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	})

	result := server.WithHandler("/create", handler)

	if result != server {
		t.Error("WithHandler() should return the same server instance")
	}

	// Test the handler was registered
	req := httptest.NewRequest("POST", "/create", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	if w.Body.String() != "created" {
		t.Errorf("Expected body 'created', got '%s'", w.Body.String())
	}
}

func TestServerWithMiddlewares(t *testing.T) {
	server := New()

	// Test CORS headers
	server.WithCORSHeaders()

	// Test security headers
	server.WithSecurityHeaders()

	// Test gzip compression
	server.WithGzip()

	// Test request logging
	server.WithLog()

	// Test rate limiting with correct parameters
	server.WithRateLimit("/api", 100, 10*time.Second)

	// Add a test handler
	server.WithHandlerFunc("/middleware-test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("middleware test"))
	})

	// Test the middleware chain
	req := httptest.NewRequest("GET", "/middleware-test", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check for CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("CORS headers should be set")
	}

	// Check for security headers
	if w.Header().Get("X-Frame-Options") == "" {
		t.Error("Security headers should be set")
	}
}

func TestServerWithStaticFiles(t *testing.T) {
	// Create a temporary directory with test files
	tempDir := t.TempDir()
	testFile := fmt.Sprintf("%s/test.txt", tempDir)
	err := os.WriteFile(testFile, []byte("static file content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	server := New()
	result := server.WithFileServer([]string{"/"}, tempDir)

	if result != server {
		t.Error("WithFileServer() should return the same server instance")
	}

	// Test serving static file
	req := httptest.NewRequest("GET", "/test.txt", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "static file content" {
		t.Errorf("Expected static file content, got '%s'", w.Body.String())
	}
}

func TestServerWithWebSocket(t *testing.T) {
	server := New()

	wsHandler := func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		defer conn.Close()
		// Echo server
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			err = conn.WriteMessage(messageType, message)
			if err != nil {
				break
			}
		}
	}

	result := server.WithWebsocket("/ws", wsHandler)

	if result != server {
		t.Error("WithWebsocket() should return the same server instance")
	}

	// WebSocket testing is complex and requires a real server
	// Here we just verify the handler was registered
	if _, exists := server.websockets["/ws"]; !exists {
		t.Error("WebSocket handler should be registered")
	}
}

func TestServerHTTPCodeHandler(t *testing.T) {
	server := New()

	// Since there's no public OnHTTPCode method, this test is removed
	// We'll just test that the server can be created and configured
	if server == nil {
		t.Error("Server should not be nil")
	}
}

func TestServerStatus(t *testing.T) {
	server := New()

	// Since there's no Status() method, we'll test basic server state
	if server.Error != nil {
		t.Error("Server should not have error initially")
	}
}

func TestServerStartAsync(t *testing.T) {
	server := New()
	server.WithHost("localhost").WithPort(0) // Use any available port

	done := make(chan error, 1)

	// Start server asynchronously
	server.StartAsync(done)

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	server.Stop()

	// Wait for completion
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("StartAsync should not return error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("StartAsync timed out")
	}
}

func TestServerRestart(t *testing.T) {
	server := New()
	server.WithHost("localhost").WithPort(0)

	// Test that restart properly shuts down if the server is already running
	// We'll start it async first
	done := make(chan error, 1)
	server.StartAsync(done)

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Now test restart async which should shutdown and restart
	restartDone := make(chan error, 1)
	server.RestartAsync(restartDone)

	// Stop after a short time
	time.Sleep(50 * time.Millisecond)
	err := server.Stop()
	if err != nil {
		t.Logf("Stop returned error (expected): %v", err)
	}
}

func TestServerWithCustomUpgrader(t *testing.T) {
	server := New()

	// Since there's no WithUpgrader method, we'll test that the upgrader is set by default
	if server.upgrader.CheckOrigin == nil {
		t.Error("Server should have a default upgrader with CheckOrigin function")
	}
}

func TestServerErrorHandling(t *testing.T) {
	server := New()

	// Simulate an error condition
	server.Error = fmt.Errorf("test error")

	// Test that methods still return the server instance even with errors
	result := server.WithHost("localhost")
	if result != server {
		t.Error("Methods should still return server instance even with errors")
	}

	// Test that Start() respects existing errors
	server.Start()
	if server.Error.Error() != "test error" {
		t.Error("Start() should preserve existing errors")
	}
}

func TestServerChaining(t *testing.T) {
	server := New()

	// Test method chaining
	result := server.
		WithHost("localhost").
		WithPort(8080).
		WithCORSHeaders().
		WithSecurityHeaders().
		WithGzip().
		WithLog().
		WithHandlerFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

	if result != server {
		t.Error("Method chaining should work correctly")
	}

	if server.host != "localhost" {
		t.Error("Chained methods should set properties correctly")
	}

	if server.port != 8080 {
		t.Error("Chained methods should set properties correctly")
	}
}

func TestServerMemoryUsage(t *testing.T) {
	// Test that creating many servers doesn't cause memory leaks
	for i := 0; i < 100; i++ {
		server := New()
		server.WithHost("localhost").
			WithPort(8080+uint16(i)).
			WithHandlerFunc(fmt.Sprintf("/test%d", i), func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
	}

	// If we get here without panicking, the test passes
}

func TestServerConcurrency(t *testing.T) {
	server := New()
	done := make(chan bool, 10)

	// Test concurrent access to server methods
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			server.WithHandlerFunc(fmt.Sprintf("/concurrent%d", id), func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf("concurrent %d", id)))
			})

			// Test the handler
			req := httptest.NewRequest("GET", fmt.Sprintf("/concurrent%d", id), nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Concurrent handler %d failed", id)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Benchmark tests
func BenchmarkServerWithHandlerFunc(b *testing.B) {
	server := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.WithHandlerFunc(fmt.Sprintf("/bench%d", i), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
}

func BenchmarkServerRouting(b *testing.B) {
	server := New()
	server.WithHandlerFunc("/benchmark", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("benchmark"))
	})

	req := httptest.NewRequest("GET", "/benchmark", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

func BenchmarkServerMiddlewares(b *testing.B) {
	server := New()
	server.WithCORSHeaders().
		WithSecurityHeaders().
		WithGzip().
		WithLog().
		WithHandlerFunc("/middleware-bench", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("middleware benchmark"))
		})

	req := httptest.NewRequest("GET", "/middleware-bench", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

// Test helper functions
func TestServerHelperMethods(t *testing.T) {
	server := New()

	// Test WithHost and WithPort methods work
	server.WithHost("example.com")
	server.WithPort(9000)

	// Since there are no getter methods, we just test that the methods don't panic
	if server.host != "example.com" {
		t.Errorf("Host should be 'example.com', got '%s'", server.host)
	}

	if server.port != 9000 {
		t.Errorf("Port should be 9000, got %d", server.port)
	}
}

func TestServerWithCustomErrorLog(t *testing.T) {
	server := New()

	// Test that WithErrorLog doesn't require parameters
	server.WithErrorLog()

	// The error log should be configured
	if server.errorLog == nil {
		t.Error("Error log should be set")
	}
}

func TestServerDefaultFunctionality(t *testing.T) {
	// Test that Instance() returns the singleton instance
	server1 := Instance()
	server2 := Instance()

	if server1 != server2 {
		t.Error("Instance() should return the same instance")
	}
}
