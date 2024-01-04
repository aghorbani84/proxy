package statute

import (
	"context"
	"fmt"
	"io"
	"net"
)

// Logger is the interface for logging messages.
type Logger interface {
	Debug(v ...interface{})
	Error(v ...interface{})
}

// DefaultLogger is a simple logger that prints messages to the standard output.
type DefaultLogger struct{}

// Debug prints debug messages to the standard output.
func (l DefaultLogger) Debug(v ...interface{}) {
	fmt.Println(v...)
}

// Error prints error messages to the standard output.
func (l DefaultLogger) Error(v ...interface{}) {
	fmt.Println(v...)
}

// ProxyRequest contains information about a proxy request.
type ProxyRequest struct {
	Conn        net.Conn
	Reader      io.Reader
	Writer      io.Writer
	Network     string
	Destination string
	DestHost    string
	DestPort    int32
}

// UserConnectHandler is a function type for handling CONNECT requests.
type UserConnectHandler func(request *ProxyRequest) error

// UserAssociateHandler is a function type for handling UDP ASSOCIATE requests.
type UserAssociateHandler func(request *ProxyRequest) error

// ProxyDialFunc is a function type for establishing transport connections.
type ProxyDialFunc func(ctx context.Context, network string, address string) (net.Conn, error)

// DefaultProxyDial returns the default implementation of ProxyDialFunc.
func DefaultProxyDial() ProxyDialFunc {
	var dialer net.Dialer
	return dialer.DialContext
}

// ProxyListenPacket is a function type for establishing transport connections using packets.
type ProxyListenPacket func(ctx context.Context, network string, address string) (net.PacketConn, error)

// DefaultProxyListenPacket returns the default implementation of ProxyListenPacket.
func DefaultProxyListenPacket() ProxyListenPacket {
	var listener net.ListenConfig
	return listener.ListenPacket
}

// PacketForwardAddress is a function type for forwarding packets and obtaining the local address.
type PacketForwardAddress func(ctx context.Context, destinationAddr string,
	packet net.PacketConn, conn net.Conn) (net.IP, int, error)

// BytesPool is an interface for getting and returning temporary bytes for use by io.CopyBuffer.
type BytesPool interface {
	Get() []byte
	Put([]byte)
}

// DefaultContext returns the default context.
func DefaultContext() context.Context {
	return context.Background()
}

// DefaultBindAddress is the default bind address for the server.
const DefaultBindAddress = "127.0.0.1:1080"
