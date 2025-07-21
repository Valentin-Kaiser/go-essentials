package web_test

import (
	"crypto/tls"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Valentin-Kaiser/go-core/security"
	"github.com/Valentin-Kaiser/go-core/web"
)

func TestServerInstance(t *testing.T) {
	t.Parallel()
	server1 := web.Instance()
	server2 := web.Instance()

	if server1 != server2 {
		t.Error("Instance() should return the same server instance (singleton)")
	}
}

func TestServerWithRedirect(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestServerMemoryUsage(t *testing.T) {
	t.Parallel()
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
