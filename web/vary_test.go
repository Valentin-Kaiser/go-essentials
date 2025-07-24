package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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