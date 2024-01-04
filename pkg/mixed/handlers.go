package mixed

import (
	"context"

	"github.com/bepass-org/proxy/pkg/statute"
)

// WithBinAddress sets the bind address for the proxy.
func WithBinAddress(binAddress string) Option {
	return func(p *Proxy) {
		p.bind = binAddress
		p.socks5Proxy.Bind = binAddress
		p.socks4Proxy.Bind = binAddress
		p.httpProxy.Bind = binAddress
	}
}

// WithLogger sets the logger for the proxy.
func WithLogger(logger statute.Logger) Option {
	return func(p *Proxy) {
		p.logger = logger
		p.socks5Proxy.Logger = logger
		p.socks4Proxy.Logger = logger
		p.httpProxy.Logger = logger
	}
}

// WithUserHandler sets the user-defined handler for the proxy.
func WithUserHandler(handler userHandler) Option {
	return func(p *Proxy) {
		p.userHandler = handler
		p.socks5Proxy.UserConnectHandle = statute.UserConnectHandler(handler)
		p.socks5Proxy.UserAssociateHandle = statute.UserAssociateHandler(handler)
		p.socks4Proxy.UserConnectHandle = statute.UserConnectHandler(handler)
		p.httpProxy.UserConnectHandle = statute.UserConnectHandler(handler)
	}
}

// WithUserTCPHandler sets the user-defined TCP handler for the proxy.
func WithUserTCPHandler(handler userHandler) Option {
	return func(p *Proxy) {
		p.userTCPHandler = handler
		p.socks5Proxy.UserConnectHandle = statute.UserConnectHandler(handler)
		p.socks4Proxy.UserConnectHandle = statute.UserConnectHandler(handler)
		p.httpProxy.UserConnectHandle = statute.UserConnectHandler(handler)
	}
}

// WithUserUDPHandler sets the user-defined UDP handler for the proxy.
func WithUserUDPHandler(handler userHandler) Option {
	return func(p *Proxy) {
		p.userUDPHandler = handler
		p.socks5Proxy.UserAssociateHandle = statute.UserAssociateHandler(handler)
	}
}

// WithUserDialFunc sets the user-defined dial function for the proxy.
func WithUserDialFunc(proxyDial statute.ProxyDialFunc) Option {
	return func(p *Proxy) {
		p.userDialFunc = proxyDial
		p.socks5Proxy.ProxyDial = proxyDial
		p.socks4Proxy.ProxyDial = proxyDial
		p.httpProxy.ProxyDial = proxyDial
	}
}

// WithUserListenPacketFunc sets the user-defined listen packet function for the proxy.
func WithUserListenPacketFunc(proxyListenPacket statute.ProxyListenPacket) Option {
	return func(p *Proxy) {
		p.socks5Proxy.ProxyListenPacket = proxyListenPacket
	}
}

// WithUserForwardAddressFunc sets the user-defined forward address function for the proxy.
func WithUserForwardAddressFunc(packetForwardAddress statute.PacketForwardAddress) Option {
	return func(p *Proxy) {
		p.socks5Proxy.PacketForwardAddress = packetForwardAddress
	}
}

// WithContext sets the context for the proxy.
func WithContext(ctx context.Context) Option {
	return func(p *Proxy) {
		p.ctx = ctx
		p.socks5Proxy.Context = ctx
		p.socks4Proxy.Context = ctx
		p.httpProxy.Context = ctx
	}
}

// WithBytesPool sets the byte pool for the proxy.
func WithBytesPool(bytesPool statute.BytesPool) Option {
	return func(p *Proxy) {
		p.socks5Proxy.BytesPool = bytesPool
		p.socks4Proxy.BytesPool = bytesPool
		p.httpProxy.BytesPool = bytesPool
	}
}
