package web

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
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
	// header is a custom header map to hold response headers
	header http.Header
	// wroteHeader indicates whether WriteHeader has been called
	wroteHeader bool
}

// Header returns the custom header map
func (rw *ResponseWriter) Header() http.Header {
	if rw.header == nil {
		rw.header = make(http.Header)
	}
	return rw.header
}

// WriteHeader captures the status code but does not send it immediately
func (rw *ResponseWriter) WriteHeader(status int) {
	if rw.wroteHeader {
		fmt.Println("WriteHeader called multiple times, ignoring subsequent calls")
		return
	}
	rw.status = status
	rw.wroteHeader = true
}

// Write buffers the response body
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	return rw.buf.Write(b)
}

// Flush writes the buffered response to the original ResponseWriter
func (rw *ResponseWriter) Flush() {
	for k, vv := range rw.header {
		for _, v := range vv {
			rw.w.Header().Add(k, v)
		}
	}
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	_, err := rw.w.Write(rw.buf.Bytes())
	if err != nil {
		http.Error(rw.w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (rw *ResponseWriter) Clear() {
	rw.buf.Reset()
	rw.status = http.StatusOK
	rw.wroteHeader = false
	rw.header = make(http.Header)
}

// Status returns the status code of the response
func (rw *ResponseWriter) Status() string {
	return fmt.Sprintf("%d %s", rw.status, http.StatusText(rw.status))
}

// Hijack is a wrapper around the http.Hijacker interface
func (rw *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}
