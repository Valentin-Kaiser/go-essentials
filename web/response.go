package web

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// responseWriter is a wrapper around http.ResponseWriter that captures the status code
type responseWriter struct {
	w http.ResponseWriter
	r *http.Request
	// status is the HTTP status code to be sent
	status int
	// buf is a buffer to hold the response body before sending it
	buf bytes.Buffer
	// header is a custom header map to hold response headers
	header http.Header
	// written indicates if the response header has been written
	written bool
}

func newResponseWriter(w http.ResponseWriter, r *http.Request) *responseWriter {
	return &responseWriter{
		w:      w,
		r:      r,
		status: http.StatusOK, // Default status code
		header: make(http.Header),
		buf:    bytes.Buffer{},
	}
}

// Header returns the custom header map
func (rw *responseWriter) Header() http.Header {
	if rw.header == nil {
		rw.header = make(http.Header)
	}
	return rw.header
}

// WriteHeader captures the status code but does not send it immediately
// It is send when Flush is called
func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
}

// Write buffers the response body
func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.buf.Write(b)
}

// Status returns the status code of the response
func (rw *responseWriter) Status() string {
	return fmt.Sprintf("%d %s", rw.status, http.StatusText(rw.status))
}

// Hijack is a wrapper around the http.Hijacker interface
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}

// flush writes the buffered response to the original ResponseWriter
func (rw *responseWriter) flush() {
	for k, vv := range rw.header {
		for _, v := range vv {
			rw.w.Header().Add(k, v)
		}
	}
	rw.w.WriteHeader(rw.status)
	_, err := rw.w.Write(rw.buf.Bytes())
	if err != nil {
		http.Error(rw.w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func (rw *responseWriter) clear() {
	rw.buf.Reset()
}
