package web

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
)

// ResponseWriter is a wrapper around http.ResponseWriter that captures the status code
type ResponseWriter struct {
	w http.ResponseWriter
	r *http.Request
	// status is the HTTP status code to be sent
	status int
	// buf is a buffer to hold the response body before sending it
	buf bytes.Buffer
	// history is a slice to hold the response body history
	history [][]byte
	// header is a custom header map to hold response headers
	header http.Header
}

func newResponseWriter(w http.ResponseWriter, r *http.Request) *ResponseWriter {
	return &ResponseWriter{
		w:       w,
		r:       r,
		status:  http.StatusOK, // Default status code
		header:  make(http.Header),
		buf:     bytes.Buffer{},
		history: make([][]byte, 0),
	}
}

// Header returns the custom header map
func (rw *ResponseWriter) Header() http.Header {
	if rw.header == nil {
		rw.header = make(http.Header)
	}
	return rw.header
}

// WriteHeader captures the status code but does not send it immediately
// It is send when Flush is called
func (rw *ResponseWriter) WriteHeader(status int) {
	rw.status = status
}

// Write buffers the response body
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	return rw.buf.Write(b)
}

// History returns the history of response bodies written
func (rw *ResponseWriter) History() [][]byte {
	return rw.history
}

// Status returns the status code of the response
func (rw *ResponseWriter) Status() int {
	return rw.status
}

// Hijack is a wrapper around the http.Hijacker interface
func (rw *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}

// flush writes the buffered response to the original ResponseWriter
func (rw *ResponseWriter) flush() {
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

func (rw *ResponseWriter) clear() {
	rw.history = append(rw.history, rw.buf.Bytes())
	rw.buf.Reset()
}
