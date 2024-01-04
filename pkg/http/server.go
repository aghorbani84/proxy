package http

import (
	"bufio"
	"context"
	"github.com/bepass-org/proxy/pkg/statute"
	"io"
	"net"
	"net/http"
	"strconv"
)

// Server represents an HTTP proxy server.
type Server struct {
	Bind              string
	ProxyDial         statute.ProxyDialFunc
	UserConnectHandle statute.UserConnectHandler
	Logger            statute.Logger
	Context           context.Context
	BytesPool         statute.BytesPool
}

// NewServer creates a new HTTP proxy server with the provided options.
func NewServer(options ...ServerOption) *Server {
	s := &Server{
		Bind:      statute.DefaultBindAddress,
		ProxyDial: statute.DefaultProxyDial(),
		Logger:    statute.DefaultLogger{},
		Context:   statute.DefaultContext(),
	}

	for _, option := range options {
		option(s)
	}

	return s
}

// ServerOption is a function that configures the HTTP proxy server.
type ServerOption func(*Server)

// ListenAndServe starts the HTTP proxy server and listens for incoming connections.
func (s *Server) ListenAndServe() error {
	s.Logger.Debug("Serving on " + s.Bind + " ...")

	ln, err := net.Listen("tcp", s.Bind)
	if err != nil {
		s.Logger.Error("Error listening on " + s.Bind + ", " + err.Error())
		return err
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(s.Context)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn, err := ln.Accept()
			if err != nil {
				s.Logger.Error(err)
				continue
			}
			go func() {
				err := s.ServeConn(conn)
				if err != nil {
					s.Logger.Error(err)
				}
			}()
		}
	}
}

// WithLogger sets the logger for the HTTP proxy server.
func WithLogger(logger statute.Logger) ServerOption {
	return func(s *Server) {
		s.Logger = logger
	}
}

// WithBind sets the bind address for the HTTP proxy server.
func WithBind(bindAddress string) ServerOption {
	return func(s *Server) {
		s.Bind = bindAddress
	}
}

// WithConnectHandle sets the user-defined connection handler for the HTTP proxy server.
func WithConnectHandle(handler statute.UserConnectHandler) ServerOption {
	return func(s *Server) {
		s.UserConnectHandle = handler
	}
}

// WithProxyDial sets the proxy dial function for the HTTP proxy server.
func WithProxyDial(proxyDial statute.ProxyDialFunc) ServerOption {
	return func(s *Server) {
		s.ProxyDial = proxyDial
	}
}

// WithContext sets the context for the HTTP proxy server.
func WithContext(ctx context.Context) ServerOption {
	return func(s *Server) {
		s.Context = ctx
	}
}

// WithBytesPool sets the byte pool for the HTTP proxy server.
func WithBytesPool(bytesPool statute.BytesPool) ServerOption {
	return func(s *Server) {
		s.BytesPool = bytesPool
	}
}

// ServeConn handles an incoming connection to the HTTP proxy server.
func (s *Server) ServeConn(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}

	return s.handleHTTP(conn, req, req.Method == http.MethodConnect)
}

// handleHTTP handles an HTTP request and invokes the user-defined connection handler.
func (s *Server) handleHTTP(conn net.Conn, req *http.Request, isConnectMethod bool) error {
	if s.UserConnectHandle == nil {
		return s.embedHandleHTTP(conn, req, isConnectMethod)
	}

	if isConnectMethod {
		_, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			return err
		}
	} else {
		cConn := &customConn{
			Conn: conn,
			req:  req,
		}
		conn = cConn
	}

	targetAddr := req.URL.Host
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		host = targetAddr
		portStr = getPortForScheme(req.URL.Scheme, isConnectMethod)
		targetAddr = net.JoinHostPort(host, portStr)
	}

	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		return err
	}
	port := int32(portInt)

	proxyReq := &statute.ProxyRequest{
		Conn:        conn,
		Reader:      io.Reader(conn),
		Writer:      io.Writer(conn),
		Network:     "tcp",
		Destination: targetAddr,
		DestHost:    host,
		DestPort:    port,
	}

	return s.UserConnectHandle(proxyReq)
}

// getPortForScheme returns the default port based on the scheme and whether it's a CONNECT method.
func getPortForScheme(scheme string, isConnectMethod bool) string {
	if scheme == "https" || isConnectMethod {
		return "443"
	}
	return "80"
}

// embedHandleHTTP handles an HTTP request when no user-defined connection handler is provided.
func (s *Server) embedHandleHTTP(conn net.Conn, req *http.Request, isConnectMethod bool) error {
	defer conn.Close()

	targetAddr := req.URL.Host
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		host = targetAddr
		portStr = getPortForScheme(req.URL.Scheme, isConnectMethod)
		targetAddr = net.JoinHostPort(host, portStr)
	}

	target, err := s.ProxyDial(s.Context, "tcp", targetAddr)
	if err != nil {
		http.Error(
			NewHTTPResponseWriter(conn),
			err.Error(),
			http.StatusServiceUnavailable,
		)
		return err
	}
	defer target.Close()

	if isConnectMethod {
		_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			return err
		}
	} else {
		err = req.Write(target)
		if err != nil {
			return err
		}
	}

	var buf1, buf2 []byte
	if s.BytesPool != nil {
		buf1 = s.BytesPool.Get()
		buf2 = s.BytesPool.Get()
		defer func() {
			s.BytesPool.Put(buf1)
			s.BytesPool.Put(buf2)
		}()
	} else {
		buf1 = make([]byte, 32*1024)
		buf2 = make([]byte, 32*1024)
	}
	return statute.Tunnel(s.Context, target, conn, buf1, buf2)
}
