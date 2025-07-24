package web_test

import (
	"crypto/tls"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Valentin-Kaiser/go-core/security"
	"github.com/Valentin-Kaiser/go-core/web"
)

func TestServerInstance(t *testing.T) {
	server1 := web.Instance()
	server2 := web.Instance()

	if server1 != server2 {
		t.Error("Instance() should return the same server instance (singleton)")
	}
}

func TestServerWithRedirect(t *testing.T) {
	server := web.New()

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
	server := web.New()
	result := server.WithRedirectToHTTPS(8080)

	if result != server {
		t.Error("WithRedirectToHTTPS() should return the same server instance")
	}

	if server.Error == nil {
		t.Error("WithRedirectToHTTPS() without TLS should set an error")
	}
}

func TestServerStartAsync(t *testing.T) {
	server := web.New()
	server.WithHost("localhost").WithPort(0) // Use any available port

	done := make(chan error, 1)

	// Start server asynchronously
	server.StartAsync(done)

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	if err := server.Stop(); err != nil {
		t.Logf("Warning: failed to stop server: %v", err)
	}

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
	server := web.New()
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

func TestServerErrorHandling(t *testing.T) {
	server := web.New()

	// Simulate an error condition
	server.Error = errors.New("test error")

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

func TestServerMemoryUsage(_ *testing.T) {
	// Test that creating many servers doesn't cause memory leaks
	for i := 0; i < 100; i++ {
		server := web.New()
		server.WithHost("localhost").
			WithPort(8080+uint16(i)).
			WithHandlerFunc(fmt.Sprintf("/test%d", i), func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
	}

	// If we get here without panicking, the test passes
}

func TestVaryHeaderMiddleware(t *testing.T) {
	// Test adding a single header
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := varyHeaderMiddleware("Accept-Encoding")
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	recorder := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(recorder, req)

	varyHeader := recorder.Header().Get("Vary")
	expected := "Accept-Encoding"
	if varyHeader != expected {
		t.Errorf("Expected Vary header to be '%s', got '%s'", expected, varyHeader)
	}
}

func TestVaryHeaderMiddleware_MultipleHeaders(t *testing.T) {
	// Test adding multiple headers
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := varyHeaderMiddleware("Accept-Language", "User-Agent")
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	recorder := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(recorder, req)

	varyHeaders := recorder.Header()["Vary"]
	if len(varyHeaders) == 0 {
		t.Error("Expected Vary headers to be set, but none were found")
	}

	// Check that both headers are present (either in one combined header or multiple headers)
	allHeaders := strings.Join(varyHeaders, ",")
	if !strings.Contains(allHeaders, "Accept-Language") {
		t.Errorf("Expected Vary headers to contain 'Accept-Language', got: %v", varyHeaders)
	}
	if !strings.Contains(allHeaders, "User-Agent") {
		t.Errorf("Expected Vary headers to contain 'User-Agent', got: %v", varyHeaders)
	}
}

func TestVaryHeaderMiddleware_WithExistingHeader(t *testing.T) {
	// Test combining with existing headers
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set an existing Vary header
		w.Header().Set("Vary", "Accept-Encoding")
		w.WriteHeader(http.StatusOK)
	})

	middleware := varyHeaderMiddleware("Accept-Language")
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	recorder := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(recorder, req)

	varyHeaders := recorder.Header()["Vary"]
	allHeaders := strings.Join(varyHeaders, ",")

	// Should contain both the existing and new headers
	if !strings.Contains(allHeaders, "Accept-Encoding") {
		t.Errorf("Expected Vary headers to contain existing 'Accept-Encoding', got: %v", varyHeaders)
	}
	if !strings.Contains(allHeaders, "Accept-Language") {
		t.Errorf("Expected Vary headers to contain new 'Accept-Language', got: %v", varyHeaders)
	}
}

func TestServerWithVaryHeader(t *testing.T) {
	server := New()

	// Add a single Vary header
	result := server.WithVaryHeader("Accept-Language")

	if result != server {
		t.Error("WithVaryHeader() should return the same server instance")
	}

	if server.Error != nil {
		t.Errorf("WithVaryHeader() should not set an error, got: %v", server.Error)
	}
}

func TestServerWithVaryHeaders(t *testing.T) {
	server := New()

	// Add multiple Vary headers
	headers := []string{"Accept-Language", "User-Agent"}
	result := server.WithVaryHeaders(headers)

	if result != server {
		t.Error("WithVaryHeaders() should return the same server instance")
	}

	if server.Error != nil {
		t.Errorf("WithVaryHeaders() should not set an error, got: %v", server.Error)
	}
}

func TestVaryHeaderIntegration(t *testing.T) {
	// Test that Vary headers work correctly when integrated with a server
	server := New()

	// Add vary headers and a test handler
	server.WithVaryHeader("Accept-Language").
		WithHandlerFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test"))
		})

	// Test by directly calling the router
	req := httptest.NewRequest("GET", "/test", nil)
	recorder := httptest.NewRecorder()

	// Call the router's ServeHTTP method directly
	server.router.ServeHTTP(recorder, req)

	// Check that the Vary header is set
	varyHeader := recorder.Header().Get("Vary")
	if !strings.Contains(varyHeader, "Accept-Language") {
		t.Errorf("Expected Vary header to contain 'Accept-Language', got: %s", varyHeader)
	}
}

// Benchmark tests
func BenchmarkServerWithHandlerFunc(b *testing.B) {
	server := web.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.WithHandlerFunc(fmt.Sprintf("/bench%d", i), func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
}
