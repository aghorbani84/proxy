package socks4

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/bepass-org/proxy/pkg/statute"
)

// Server is accepting connections and handling the details of the SOCKS4 protocol
type Server struct {
	Bind              string
	ProxyDial         statute.ProxyDialFunc
	UserConnectHandle statute.UserConnectHandler
	Logger            statute.Logger
	Context           context.Context
	BytesPool         statute.BytesPool
}

func NewServer(options ...ServerOption) *Server {
	s := &Server{
		ProxyDial: statute.DefaultProxyDial(),
		Logger:    statute.DefaultLogger{},
		Context:   statute.DefaultContext(),
	}

	for _, option := range options {
		option(s)
	}

	return s
}

// ServerOption is a functional option for configuring the Server.
type ServerOption func(*Server)

// ListenAndServe starts accepting connections on the specified address.
func (s *Server) ListenAndServe() error {
	s.Logger.Debug("Serving on " + s.Bind + " ...")

	ln, err := net.Listen("tcp", s.Bind)
	if err != nil {
		s.Logger.Error("Error listening on " + s.Bind + ", " + err.Error())
		return err
	}
	defer func() {
		_ = ln.Close()
	}()

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

// ServeConn handles the SOCKS4 protocol for a single connection.
func (s *Server) ServeConn(conn net.Conn) error {
	version, err := readByte(conn)
	if err != nil {
		return err
	}
	if version != socks4Version {
		return fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	req := &request{
		Version: socks4Version,
		Conn:    conn,
	}

	cmd, err := readByte(conn)
	if err != nil {
		return err
	}
	req.Command = Command(cmd)

	addr, err := readAddrAndUser(conn)
	if err != nil {
		if err := sendReply(req.Conn, rejectedReply, nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return err
	}
	req.DestinationAddr = &addr.address
	req.Username = addr.Username
	return s.handle(req)
}

// ServerOption functions for configuring the Server.

// WithLogger sets the logger for the Server.
func WithLogger(logger statute.Logger) ServerOption {
	return func(s *Server) {
		s.Logger = logger
	}
}

// WithBind sets the address to listen on for the Server.
func WithBind(bindAddress string) ServerOption {
	return func(s *Server) {
		s.Bind = bindAddress
	}
}

// WithConnectHandle sets the user handler for handling TCP CONNECT requests.
func WithConnectHandle(handler statute.UserConnectHandler) ServerOption {
	return func(s *Server) {
		s.UserConnectHandle = handler
	}
}

// WithProxyDial sets the proxyDial function for establishing transport connections.
func WithProxyDial(proxyDial statute.ProxyDialFunc) ServerOption {
	return func(s *Server) {
		s.ProxyDial = proxyDial
	}
}

// WithContext sets the default context for the Server.
func WithContext(ctx context.Context) ServerOption {
	return func(s *Server) {
		s.Context = ctx
	}
}

// WithBytesPool sets the bytes pool for temporary buffers used by io.CopyBuffer.
func WithBytesPool(bytesPool statute.BytesPool) ServerOption {
	return func(s *Server) {
		s.BytesPool = bytesPool
	}
}

// handle processes the SOCKS4 request based on the command type.
func (s *Server) handle(req *request) error {
	switch req.Command {
	case ConnectCommand:
		return s.handleConnect(req)
	default:
		if err := sendReply(req.Conn, rejectedReply, nil); err != nil {
			return err
		}
		return fmt.Errorf("unsupported Command: %v", req.Command)
	}
}

// handleConnect handles the SOCKS4 CONNECT command.
func (s *Server) handleConnect(req *request) error {
	if s.UserConnectHandle == nil {
		return s.embedHandleConnect(req)
	}

	if err := sendReply(req.Conn, grantedReply, nil); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}
	host := req.DestinationAddr.IP.String()
	if req.DestinationAddr.Name != "" {
		host = req.DestinationAddr.Name
	}

	proxyReq := &statute.ProxyRequest{
		Conn:        req.Conn,
		Reader:      io.Reader(req.Conn),
		Writer:      io.Writer(req.Conn),
		Network:     "tcp",
		Destination: req.DestinationAddr.String(),
		DestHost:    host,
		DestPort:    int32(req.DestinationAddr.Port),
	}

	return s.UserConnectHandle(proxyReq)
}

// embedHandleConnect is the default handler for SOCKS4 CONNECT if UserConnectHandle is not set.
func (s *Server) embedHandleConnect(req *request) error {
	defer func() {
		_ = req.Conn.Close()
	}()
	target, err := s.ProxyDial(s.Context, "tcp", req.DestinationAddr.Address())
	if err != nil {
		if err := sendReply(req.Conn, rejectedReply, nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %w", req.DestinationAddr, err)
	}
	defer func() {
		_ = target.Close()
	}()
	local := target.LocalAddr().(*net.TCPAddr)
	bind := address{IP: local.IP, Port: local.Port}
	if err := sendReply(req.Conn, grantedReply, &bind); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
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
	return statute.Tunnel(s.Context, target, req.Conn, buf1, buf2)
}

// sendReply sends the SOCKS4 reply to the client.
func sendReply(w io.Writer, resp reply, addr *address) error {
	_, err := w.Write([]byte{0, byte(resp)})
	if err != nil {
		return err
	}
	err = writeAddr(w, addr)
	return err
}

// request represents a SOCKS4 request.
type request struct {
	Version         uint8
	Command         Command
	DestinationAddr *address
	Username        string
	Conn            net.Conn
}
