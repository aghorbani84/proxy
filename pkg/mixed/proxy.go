package mixed

import (
	"bufio"
	"context"
	"net"

	"github.com/bepass-org/proxy/pkg/http"
	"github.com/bepass-org/proxy/pkg/socks4"
	"github.com/bepass-org/proxy/pkg/socks5"
	"github.com/bepass-org/proxy/pkg/statute"
)

// userHandler is a function type for handling proxy requests.
type userHandler func(request *statute.ProxyRequest) error

// Proxy is a multiprotocol proxy server.
type Proxy struct {
	bind           string                // Address to listen on
	socks5Proxy    *socks5.Server        // SOCKS5 server with TCP and UDP support
	socks4Proxy    *socks4.Server        // SOCKS4 server with TCP support
	httpProxy      *http.Server          // HTTP proxy server with HTTP and HTTP-connect support
	userHandler    userHandler           // General handler for TCP and UDP requests
	userTCPHandler userHandler           // User-defined handler for TCP requests
	userUDPHandler userHandler           // User-defined handler for UDP requests
	userDialFunc   statute.ProxyDialFunc // User-defined dial function
	logger         statute.Logger        // Logger for error logs
	ctx            context.Context       // Default context
}

// NewProxy creates a new multiprotocol proxy server with options.
func NewProxy(options ...Option) *Proxy {
	p := &Proxy{
		bind:         statute.DefaultBindAddress,
		socks5Proxy:  socks5.NewServer(),
		socks4Proxy:  socks4.NewServer(),
		httpProxy:    http.NewServer(),
		userDialFunc: statute.DefaultProxyDial(),
		logger:       statute.DefaultLogger{},
		ctx:          statute.DefaultContext(),
	}

	for _, option := range options {
		option(p)
	}

	return p
}

// Option is a function type for configuring the Proxy.
type Option func(*Proxy)

// SwitchConn wraps a net.Conn and a bufio.Reader.
type SwitchConn struct {
	net.Conn
	reader *bufio.Reader
}

// NewSwitchConn creates a new SwitchConn.
func NewSwitchConn(conn net.Conn) *SwitchConn {
	return &SwitchConn{
		Conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// Read reads data into p, first from the bufio.Reader, then from the net.Conn.
func (c *SwitchConn) Read(p []byte) (n int, err error) {
	return c.reader.Read(p)
}

// ListenAndServe starts the proxy server and begins listening for incoming connections.
func (p *Proxy) ListenAndServe() error {
	p.logger.Debug("Serving on " + p.bind + " ...")
	ln, err := net.Listen("tcp", p.bind)
	if err != nil {
		p.logger.Error("Error listening on " + p.bind + ", " + err.Error())
		return err
	}
	defer func() {
		_ = ln.Close()
	}()

	ctx, cancel := context.WithCancel(p.ctx)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn, err := ln.Accept()
			if err != nil {
				p.logger.Error(err)
				continue
			}

			go func() {
				err := p.handleConnection(conn)
				if err != nil {
					p.logger.Error(err)
				}
			}()
		}
	}
}

// handleConnection handles incoming connections and routes them based on the detected protocol.
func (p *Proxy) handleConnection(conn net.Conn) error {
	switchConn := NewSwitchConn(conn)
	buf := make([]byte, 1)

	_, err := switchConn.Read(buf)
	if err != nil {
		return err
	}

	err = switchConn.reader.UnreadByte()
	if err != nil {
		return err
	}

	switch {
	case buf[0] == 5:
		err = p.socks5Proxy.ServeConn(switchConn)
	case buf[0] == 4:
		err = p.socks4Proxy.ServeConn(switchConn)
	default:
		err = p.httpProxy.ServeConn(switchConn)
	}

	return err
}
