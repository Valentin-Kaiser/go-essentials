package web

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/Valentin-Kaiser/go-essentials/flag"
	"github.com/Valentin-Kaiser/go-essentials/version"
	"github.com/rs/zerolog/log"
)

// ResponseWriter is a wrapper around http.ResponseWriter that captures the status code
type ResponseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code and calls the original WriteHeader method
// It is used to log the status code of the response
func (w *ResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Status returns the status code of the response
func (w *ResponseWriter) Status() string {
	return fmt.Sprintf("%d %s", w.status, http.StatusText(w.status))
}

// Hijack is a wrapper around the http.Hijacker interface
// It is used to hijack the connection and get a net.Conn and bufio.ReadWriter
func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}

// securityHeader is a middleware that adds security headers to the response
// It is used to prevent attacks like XSS, clickjacking, etc.
func securityHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders := map[string]string{
			"ETag":                      version.GitCommit,
			"Cache-Control":             "public, must-revalidate, max-age=86400",
			"Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
			"X-Content-Type-Options":    "nosniff",
			"X-Frame-Options":           "DENY",
			"X-XSS-Protection":          "1; mode=block",
			"Referrer-Policy":           "no-referrer-when-downgrade",
		}
		for key, value := range securityHeaders {
			w.Header().Set(key, value)
		}
		if flag.Debug {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		}
		next.ServeHTTP(w, r)
	})
}

func corsHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corsHeaders := map[string]string{
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
			"Access-Control-Allow-Headers": "Content-Type, Authorization, X-Real-IP",
		}
		for key, value := range corsHeaders {
			w.Header().Set(key, value)
		}
		next.ServeHTTP(w, r)
	})
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &ResponseWriter{ResponseWriter: w, status: http.StatusOK}
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
			Str("status", rw.Status()).
			Msg("request")
	})
}

func logRequestFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logRequest(next).ServeHTTP(w, r)
	}
}
