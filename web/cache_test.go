package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Valentin-Kaiser/go-core/version"
)

// TestCacheControlHeaders tests that the enhanced cache control headers are properly set
func TestCacheControlHeaders(t *testing.T) {
	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Apply the security middleware
	middlewareHandler := securityHeaderMiddleware(handler)

	// Create a request
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Execute the handler
	middlewareHandler.ServeHTTP(rr, req)

	// Check that the response code is correct
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check that the cache control header includes the new directives
	cacheControl := rr.Header().Get("Cache-Control")
	expectedCacheControl := "public, must-revalidate, max-age=86400, stale-while-revalidate=3600, stale-if-error=86400"
	if cacheControl != expectedCacheControl {
		t.Errorf("Cache-Control header = %v, want %v", cacheControl, expectedCacheControl)
	}

	// Check that the ETag header is set
	etag := rr.Header().Get("ETag")
	if etag != version.GitCommit {
		t.Errorf("ETag header = %v, want %v", etag, version.GitCommit)
	}
}

// TestETagValidation tests that If-None-Match header validation works correctly
func TestETagValidation(t *testing.T) {
	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Apply the security middleware
	middlewareHandler := securityHeaderMiddleware(handler)

	// Test case 1: Request with matching ETag (unquoted)
	req1, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req1.Header.Set("If-None-Match", version.GitCommit)

	rr1 := httptest.NewRecorder()
	middlewareHandler.ServeHTTP(rr1, req1)

	if status := rr1.Code; status != http.StatusNotModified {
		t.Errorf("handler returned wrong status code for matching ETag: got %v want %v", status, http.StatusNotModified)
	}

	// Test case 2: Request with matching ETag (quoted)
	req2, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("If-None-Match", `"`+version.GitCommit+`"`)

	rr2 := httptest.NewRecorder()
	middlewareHandler.ServeHTTP(rr2, req2)

	if status := rr2.Code; status != http.StatusNotModified {
		t.Errorf("handler returned wrong status code for matching quoted ETag: got %v want %v", status, http.StatusNotModified)
	}

	// Test case 3: Request with non-matching ETag
	req3, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req3.Header.Set("If-None-Match", "different-etag")

	rr3 := httptest.NewRecorder()
	middlewareHandler.ServeHTTP(rr3, req3)

	if status := rr3.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code for non-matching ETag: got %v want %v", status, http.StatusOK)
	}

	// Test case 4: Request without If-None-Match header
	req4, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr4 := httptest.NewRecorder()
	middlewareHandler.ServeHTTP(rr4, req4)

	if status := rr4.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code for request without If-None-Match: got %v want %v", status, http.StatusOK)
	}
}
