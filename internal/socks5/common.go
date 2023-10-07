package socks5

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var (
	errStringTooLong        = errors.New("string too long")
	errNoSupportedAuth      = errors.New("no supported authentication mechanism")
	errUnrecognizedAddrType = errors.New("unrecognized address type")
)

const (
	maxUdpPacket = math.MaxUint16 - 28
)

const (
	socks5Version = 0x05
)

const (
	ConnectCommand   Command = 0x01
	AssociateCommand Command = 0x03
)

// Command is a SOCKS Command.
type Command byte

func (cmd Command) String() string {
	switch cmd {
	case ConnectCommand:
		return "socks connect"
	case AssociateCommand:
		return "socks associate"
	default:
		return "socks " + strconv.Itoa(int(cmd))
	}
}

const (
	successReply         reply = 0x00
	serverFailure        reply = 0x01
	ruleFailure          reply = 0x02
	networkUnreachable   reply = 0x03
	hostUnreachable      reply = 0x04
	connectionRefused    reply = 0x05
	ttlExpired           reply = 0x06
	commandNotSupported  reply = 0x07
	addrTypeNotSupported reply = 0x08
)

func errToReply(err error) reply {
	if err == nil {
		return successReply
	}
	msg := err.Error()
	resp := hostUnreachable
	if strings.Contains(msg, "refused") {
		resp = connectionRefused
	} else if strings.Contains(msg, "network is unreachable") {
		resp = networkUnreachable
	}
	return resp
}

// reply is a SOCKS Command reply code.
type reply byte

func (code reply) String() string {
	switch code {
	case successReply:
		return "succeeded"
	case serverFailure:
		return "general SOCKS server failure"
	case ruleFailure:
		return "connection not allowed by ruleset"
	case networkUnreachable:
		return "network unreachable"
	case hostUnreachable:
		return "host unreachable"
	case connectionRefused:
		return "connection refused"
	case ttlExpired:
		return "TTL expired"
	case commandNotSupported:
		return "Command not supported"
	case addrTypeNotSupported:
		return "address type not supported"
	default:
		return "unknown code: " + strconv.Itoa(int(code))
	}
}

const (
	ipv4Address = 0x01
	fqdnAddress = 0x03
	ipv6Address = 0x04
)

// address is a SOCKS-specific address.
// Either Name or IP is used exclusively.
type address struct {
	Name string // fully-qualified domain name
	IP   net.IP
	Port int
}

func (a *address) Network() string { return "socks5" }

func (a *address) String() string {
	if a == nil {
		return "<nil>"
	}
	return a.Address()
}

// Address returns a string suitable to dial; prefer returning IP-based
// address, fallback to Name
func (a address) Address() string {
	port := strconv.Itoa(a.Port)
	if 0 != len(a.IP) {
		return net.JoinHostPort(a.IP.String(), port)
	}
	return net.JoinHostPort(a.Name, port)
}

// authMethod is a SOCKS authentication method.
type authMethod byte

const (
	noAuth       authMethod = 0x00 // no authentication required
	noAcceptable authMethod = 0xff // no acceptable authentication methods
)

func readBytes(r io.Reader) ([]byte, error) {
	var buf [1]byte
	_, err := r.Read(buf[:])
	if err != nil {
		return nil, err
	}
	bytes := make([]byte, buf[0])
	_, err = io.ReadFull(r, bytes)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func writeBytes(w io.Writer, b []byte) error {
	_, err := w.Write([]byte{byte(len(b))})
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readByte(r io.Reader) (byte, error) {
	var buf [1]byte
	_, err := r.Read(buf[:])
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func readAddr(r io.Reader) (*address, error) {
	address := &address{}

	var addrType [1]byte
	if _, err := r.Read(addrType[:]); err != nil {
		return nil, err
	}

	switch addrType[0] {
	case ipv4Address:
		addr := make(net.IP, net.IPv4len)
		if _, err := io.ReadFull(r, addr); err != nil {
			return nil, err
		}
		address.IP = addr
	case ipv6Address:
		addr := make(net.IP, net.IPv6len)
		if _, err := io.ReadFull(r, addr); err != nil {
			return nil, err
		}
		address.IP = addr
	case fqdnAddress:
		if _, err := r.Read(addrType[:]); err != nil {
			return nil, err
		}
		addrLen := int(addrType[0])
		fqdn := make([]byte, addrLen)
		if _, err := io.ReadFull(r, fqdn); err != nil {
			return nil, err
		}
		address.Name = string(fqdn)
	default:
		return nil, errUnrecognizedAddrType
	}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return nil, err
	}
	address.Port = int(binary.BigEndian.Uint16(port[:]))
	return address, nil
}

func writeAddr(w io.Writer, addr *address) error {
	if addr == nil {
		_, err := w.Write([]byte{ipv4Address, 0, 0, 0, 0, 0, 0})
		if err != nil {
			return err
		}
		return nil
	}
	if addr.IP != nil {
		if ip4 := addr.IP.To4(); ip4 != nil {
			_, err := w.Write([]byte{ipv4Address})
			if err != nil {
				return err
			}
			_, err = w.Write(ip4)
			if err != nil {
				return err
			}
		} else if ip6 := addr.IP.To16(); ip6 != nil {
			_, err := w.Write([]byte{ipv6Address})
			if err != nil {
				return err
			}
			_, err = w.Write(ip6)
			if err != nil {
				return err
			}
		} else {
			_, err := w.Write([]byte{ipv4Address, 0, 0, 0, 0})
			if err != nil {
				return err
			}
		}
	} else if addr.Name != "" {
		if len(addr.Name) > 255 {
			return errStringTooLong
		}
		_, err := w.Write([]byte{fqdnAddress, byte(len(addr.Name))})
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(addr.Name))
		if err != nil {
			return err
		}
	} else {
		_, err := w.Write([]byte{ipv4Address, 0, 0, 0, 0})
		if err != nil {
			return err
		}
	}
	var p [2]byte
	binary.BigEndian.PutUint16(p[:], uint16(addr.Port))
	_, err := w.Write(p[:])
	return err
}

func writeAddrWithStr(w io.Writer, addr string) error {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip != nil {
		return writeAddr(w, &address{IP: ip, Port: port})
	}
	return writeAddr(w, &address{Name: host, Port: port})
}

func splitHostPort(address string) (string, int, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	portnum, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, err
	}
	if 0 > portnum || portnum > 0xffff {
		return "", 0, errors.New("port number out of range " + port)
	}
	return host, portnum, nil
}

// isClosedConnError reports whether err is an error from use of a closed
// network connection.
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	if strings.Contains(str, "use of closed network connection") {
		return true
	}

	if runtime.GOOS == "windows" {
		if oe, ok := err.(*net.OpError); ok && oe.Op == "read" {
			if se, ok := oe.Err.(*os.SyscallError); ok && se.Syscall == "wsarecv" {
				const WSAECONNABORTED = 10053
				const WSAECONNRESET = 10054
				if n := errno(se.Err); n == WSAECONNRESET || n == WSAECONNABORTED {
					return true
				}
			}
		}
	}
	return false
}

func errno(v error) uintptr {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Uintptr {
		return uintptr(rv.Uint())
	}
	return 0
}

// tunnel create tunnels for two io.ReadWriteCloser
func tunnel(ctx context.Context, c1, c2 io.ReadWriteCloser, buf1, buf2 []byte) error {
	ctx, cancel := context.WithCancel(ctx)
	var errs tunnelErr
	go func() {
		_, errs[0] = io.CopyBuffer(c1, c2, buf1)
		cancel()
	}()
	go func() {
		_, errs[1] = io.CopyBuffer(c2, c1, buf2)
		cancel()
	}()
	<-ctx.Done()
	errs[2] = c1.Close()
	errs[3] = c2.Close()
	errs[4] = ctx.Err()
	if errs[4] == context.Canceled {
		errs[4] = nil
	}
	return errs.FirstError()
}

type tunnelErr [5]error

func (t tunnelErr) FirstError() error {
	for _, err := range t {
		if err != nil {
			return err
		}
	}
	return nil
}

// BytesPool is an interface for getting and returning temporary
// bytes for use by io.CopyBuffer.
type BytesPool interface {
	Get() []byte
	Put([]byte)
}

type readStruct struct {
	data []byte
	err  error
}

type udpCustomConn struct {
	net.PacketConn
	assocTCPConn net.Conn
	lock         sync.Mutex
	sourceAddr   net.Addr
	targetAddr   net.Addr
	replyPrefix  []byte
	buf          [maxUdpPacket]byte
	firstRead    sync.Once
	frc          chan bool
	packetQueue  chan *readStruct
}

func (cc *udpCustomConn) RemoteAddr() net.Addr {
	return cc.targetAddr
}

func (cc *udpCustomConn) asyncReadPackets() {
	go func() {
		for {
			tempBuf := make([]byte, maxUdpPacket)
			n, addr, err := cc.ReadFrom(tempBuf)
			if err != nil {
				cc.packetQueue <- &readStruct{
					data: nil,
					err:  err,
				}
				break
			}
			cc.lock.Lock()
			if cc.sourceAddr == nil {
				cc.sourceAddr = addr
			}
			cc.lock.Unlock()
			packetData := tempBuf[:n]
			cc.packetQueue <- &readStruct{
				data: packetData,
				err:  nil,
			}
		}
	}()
}

func (cc *udpCustomConn) Read(b []byte) (int, error) {
	cc.lock.Lock()
	defer cc.lock.Unlock()

	// wait for packet data
	read := <-cc.packetQueue

	if read.err != nil {
		return 0, read.err
	}

	packetData := read.data

	if len(packetData) < 3 {
		return 0, errors.New("received packet too small")
	}
	reader := bytes.NewBuffer(packetData[3:])
	targetAddr, err := readAddr(reader)
	if err != nil {
		return 0, err
	}
	if cc.targetAddr == nil {
		cc.targetAddr = &net.UDPAddr{
			IP:   targetAddr.IP,
			Port: targetAddr.Port,
		}
	}
	if targetAddr.String() != cc.targetAddr.String() {
		return 0, fmt.Errorf("ignore non-target addresses %s", targetAddr.String())
	}
	copy(b, reader.Bytes())

	cc.firstRead.Do(func() {
		// ok we have source and destination address now user can handle new ProxyReq
		cc.frc <- true
	})

	return reader.Len(), nil
}

func (cc *udpCustomConn) Write(b []byte) (int, error) {
	cc.lock.Lock()
	defer cc.lock.Unlock()

	if cc.replyPrefix == nil {
		b := bytes.NewBuffer(make([]byte, 3, 16))
		err := writeAddrWithStr(b, cc.targetAddr.String())
		if err != nil {
			return 0, err
		}
		cc.replyPrefix = b.Bytes()
	}
	copy(b, cc.buf[len(cc.replyPrefix):len(cc.replyPrefix)+len(b)])
	return len(b), nil
}

func (cc *udpCustomConn) Close() error {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	udpErr := cc.Close()
	tcpErr := cc.assocTCPConn.Close()
	if udpErr != nil {
		return udpErr
	}
	return tcpErr
}
