package web

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
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
