package web_test

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/Valentin-Kaiser/go-core/web"
)

func TestRouterUnregister(t *testing.T) {
	router := web.NewRouter()

	// Register some test routes
	handler1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("handler1"))
	})
	handler2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("handler2"))
	})
	handler3 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("handler3"))
	})

	router.Handle("/test1", handler1)
	router.Handle("/test2", handler2)
	router.Handle("/test3", handler3)

	// Test that all routes are registered
	routes := router.GetRegisteredRoutes()
	expectedRoutes := []string{"/test1", "/test2", "/test3"}
	sort.Strings(routes)
	sort.Strings(expectedRoutes)

	if len(routes) != len(expectedRoutes) {
		t.Fatalf("Expected %d routes, got %d", len(expectedRoutes), len(routes))
	}

	for i, route := range routes {
		if route != expectedRoutes[i] {
			t.Errorf("Expected route %s, got %s", expectedRoutes[i], route)
		}
	}

	// Test that routes work before unregistering
	req := httptest.NewRequest("GET", "/test1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Body.String() != "handler1" {
		t.Errorf("Expected 'handler1', got '%s'", w.Body.String())
	}

	// Test unregistering a single route
	router.Unregister("/test1")

	// Verify route is unregistered
	routes = router.GetRegisteredRoutes()
	if len(routes) != 2 {
		t.Fatalf("Expected 2 routes after unregistering, got %d", len(routes))
	}

	// Test that unregistered route returns 404
	req = httptest.NewRequest("GET", "/test1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for unregistered route, got %d", w.Code)
	}

	// Test that other routes still work
	req = httptest.NewRequest("GET", "/test2", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Body.String() != "handler2" {
		t.Errorf("Expected 'handler2', got '%s'", w.Body.String())
	}
}

func TestRouterUnregisterMultiple(t *testing.T) {
	router := web.NewRouter()

	// Register test routes
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	})

	router.Handle("/test1", handler)
	router.Handle("/test2", handler)
	router.Handle("/test3", handler)
	router.Handle("/test4", handler)

	// Unregister multiple routes
	router.UnregisterMultiple([]string{"/test1", "/test3"})

	// Verify correct routes remain
	routes := router.GetRegisteredRoutes()
	if len(routes) != 2 {
		t.Fatalf("Expected 2 routes after unregistering, got %d", len(routes))
	}

	sort.Strings(routes)
	expectedRoutes := []string{"/test2", "/test4"}
	sort.Strings(expectedRoutes)

	for i, route := range routes {
		if route != expectedRoutes[i] {
			t.Errorf("Expected route %s, got %s", expectedRoutes[i], route)
		}
	}
}

func TestRouterUnregisterAll(t *testing.T) {
	router := web.NewRouter()

	// Register test routes
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	})

	router.Handle("/test1", handler)
	router.Handle("/test2", handler)
	router.Handle("/test3", handler)

	// Add status callback
	router.OnStatus("/test1", 200, func(w http.ResponseWriter, r *http.Request) {})

	// Unregister all routes
	router.UnregisterAll()

	// Verify all routes are unregistered
	routes := router.GetRegisteredRoutes()
	if len(routes) != 0 {
		t.Fatalf("Expected 0 routes after UnregisterAll, got %d", len(routes))
	}

	// Test that all routes return 404
	for _, path := range []string{"/test1", "/test2", "/test3"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404 for path %s after UnregisterAll, got %d", path, w.Code)
		}
	}
}

func TestRouterUnregisterWithStatusCallbacks(t *testing.T) {
	router := web.NewRouter()

	// Register a route with status callback
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	router.Handle("/test", handler)
	router.OnStatus("/test", 200, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "callback-executed")
	})

	// Verify status callback works before unregistering
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Header().Get("X-Custom") != "callback-executed" {
		t.Error("Status callback should have been executed")
	}

	// Unregister the route
	router.Unregister("/test")

	// Verify status callback is also removed
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 after unregistering, got %d", w.Code)
	}
}

func TestRouterConcurrentAccess(t *testing.T) {
	router := web.NewRouter()

	// Test concurrent registration and unregistration
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	})

	// Register initial routes
	for i := 0; i < 10; i++ {
		router.Handle("/test"+string(rune('0'+i)), handler)
	}

	// Test concurrent access
	done := make(chan bool, 3)

	// Goroutine 1: Make requests
	go func() {
		for i := 0; i < 100; i++ {
			req := httptest.NewRequest("GET", "/test0", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
		done <- true
	}()

	// Goroutine 2: Unregister and register routes
	go func() {
		for i := 0; i < 10; i++ {
			router.Unregister("/test" + string(rune('0'+i)))
			router.Handle("/test"+string(rune('0'+i)), handler)
		}
		done <- true
	}()

	// Goroutine 3: Get registered routes
	go func() {
		for i := 0; i < 50; i++ {
			router.GetRegisteredRoutes()
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify router is still functional
	routes := router.GetRegisteredRoutes()
	if len(routes) != 10 {
		t.Errorf("Expected 10 routes after concurrent operations, got %d", len(routes))
	}
}
