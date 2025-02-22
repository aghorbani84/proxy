package http

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
)

// copyBuffer is a helper function to copy data between two net.Conn objects.
func copyBuffer(dst, src net.Conn, buf []byte) (int64, error) {
	return io.CopyBuffer(dst, src, buf)
}

// responseWriter is a custom implementation of http.ResponseWriter.
type responseWriter struct {
	conn    net.Conn
	headers http.Header
	status  int
	written bool
}

// NewHTTPResponseWriter creates a new instance of responseWriter.
func NewHTTPResponseWriter(conn net.Conn) http.ResponseWriter {
	return &responseWriter{
		conn:    conn,
		headers: http.Header{},
		status:  http.StatusOK,
	}
}

// Header returns the headers map.
func (rw *responseWriter) Header() http.Header {
	return rw.headers
}

// WriteHeader writes the HTTP status line and headers.
func (rw *responseWriter) WriteHeader(statusCode int) {
	if rw.written {
		return
	}
	rw.status = statusCode
	rw.written = true

	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = fmt.Sprintf("status code %d", statusCode)
	}
	fmt.Fprintf(rw.conn, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
	rw.headers.Write(rw.conn)
	rw.conn.Write([]byte("\r\n"))
}

// Write writes the data to the connection.
func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.conn.Write(data)
}

// customConn is a wrapper around net.Conn with additional functionality.
type customConn struct {
	net.Conn
	req         *http.Request
	initialData []byte
	once        sync.Once
}

// Read reads data from the connection.
func (c *customConn) Read(p []byte) (n int, err error) {
	c.once.Do(func() {
		buf := &bytes.Buffer{}
		err = c.req.Write(buf)
		if err == nil {
			c.initialData = buf.Bytes()
		}
	})

	if len(c.initialData) > 0 {
		n = copy(p, c.initialData)
		c.initialData = nil
		return
	}

	return c.Conn.Read(p)
}
