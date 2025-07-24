package web

import (
	"fmt"
	"net/http"

	"github.com/Valentin-Kaiser/go-core/flag"
	"github.com/Valentin-Kaiser/go-core/version"
	"github.com/rs/zerolog/log"
)

var (
	securityHeaders = map[string]string{
		"ETag":                      version.GitCommit,
		"Cache-Control":             "public, must-revalidate, max-age=86400, stale-while-revalidate=3600, stale-if-error=86400",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "1; mode=block",
		"Referrer-Policy":           "no-referrer-when-downgrade",
	}
	corsHeaders = map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type, Authorization, X-Real-IP",
	}
)

// Middleware is a function that takes an http.Handler and returns an http.Handler.
type Middleware func(http.Handler) http.Handler

// MiddlewareOrder defines the order in which middlewares are executed.
type MiddlewareOrder int8

const (
	// MiddlewareOrderDefault is the default execution order for middlewares.
	// It is invoked at the same level as the original handler.
	MiddlewareOrderDefault MiddlewareOrder = 0
	// MiddlewareOrderLow represents the lowest execution order.
	// Middlewares with negative values (-128 to -1) are called before the original handler.
	MiddlewareOrderLow MiddlewareOrder = -128
	// MiddlewareOrderHigh represents the highest execution order.
	// Middlewares with positive values (1 to 127) are called after the original handler.
	MiddlewareOrderHigh MiddlewareOrder = 127
	// MiddlewareOrderSecurity is a specific order typically used for security-related middlewares.
	// It is called before the handler.
	MiddlewareOrderSecurity MiddlewareOrder = -127
	// MiddlewareOrderCors is a specific order typically used for CORS-related middlewares.
	// It is called before the handler.
	MiddlewareOrderCors MiddlewareOrder = -126
	// MiddlewareOrderLog is a specific order typically used for logging middlewares.
	// It is called after the handler.
	MiddlewareOrderLog MiddlewareOrder = 126
	// MiddlewareOrderGzip is a specific order typically used for gzip compression middlewares.
	MiddlewareOrderGzip MiddlewareOrder = 127
)

// securityHeaderMiddlewareWithServer creates a security header middleware with access to server configuration
func securityHeaderMiddlewareWithServer(server *Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check If-None-Match header for ETag validation
			if inm := r.Header.Get("If-None-Match"); inm != "" {
				// Compare with current ETag (version.GitCommit)
				if inm == version.GitCommit || inm == `"`+version.GitCommit+`"` {
					// ETag matches, return 304 Not Modified
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}

			for key, value := range securityHeaders {
				// Skip Cache-Control if a custom one is set
				if key == "Cache-Control" && server.cacheControl != "" {
					continue
				}
				w.Header().Set(key, value)
			}

			// Set custom cache control if specified
			if server.cacheControl != "" {
				w.Header().Set("Cache-Control", server.cacheControl)
			}

			if flag.Debug {
				w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsHeaderMiddleware is a middleware that adds CORS headers to the response
// It is used to allow cross-origin requests from the client
func corsHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, value := range corsHeaders {
			w.Header().Set(key, value)
		}
		next.ServeHTTP(w, r)
	})
}

// varyHeaderMiddleware creates a middleware that adds Vary headers to the response
// It properly handles comma-separated values when multiple headers are specified
func varyHeaderMiddleware(headers ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Execute the handler first
			next.ServeHTTP(w, r)
			
			// Get existing Vary header after the handler has run
			existing := w.Header().Get("Vary")
			
			// If we have headers to add
			if len(headers) > 0 {
				// Combine with new headers
				var allHeaders []string
				if existing != "" {
					allHeaders = append(allHeaders, existing)
				}
				allHeaders = append(allHeaders, headers...)
				
				// Set the combined Vary header
				w.Header().Set("Vary", allHeaders[0])
				for _, header := range allHeaders[1:] {
					w.Header().Add("Vary", header)
				}
			}
		})
	}
}

// logMiddleware is a middleware that logs the request and response
// Must be used before the gzip middleware to ensure the response is logged correctly
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw, ok := w.(*ResponseWriter)
		if !ok {
			rw = newResponseWriter(w, r)
		}
		next.ServeHTTP(rw, r)

		loglevel := log.Debug
		if rw.status >= 400 {
			loglevel = log.Warn
		}
		if rw.status >= 500 {
			loglevel = log.Error
		}

		loglevel().
			Str("remote", r.RemoteAddr).
			Str("real-ip", r.Header.Get("X-Real-IP")).
			Str("host", r.Host).
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Str("user-agent", r.UserAgent()).
			Str("referer", r.Referer()).
			Str("status", fmt.Sprintf("%d %s", rw.status, http.StatusText(rw.status))).
			Msg("request")
	})
}
