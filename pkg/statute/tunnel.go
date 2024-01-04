package statute

import (
	"context"
	"io"
	"net"
	"os"
	"reflect"
	"runtime"
	"strings"
)

// isClosedConnError reports whether err is an error from the use of a closed
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

// errno extracts the numeric value from an error.
func errno(v error) uintptr {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Uintptr {
		return uintptr(rv.Uint())
	}
	return 0
}

// Tunnel creates bidirectional tunnels between two io.ReadWriteCloser instances.
func Tunnel(ctx context.Context, source, destination io.ReadWriteCloser, sourceBuffer, destinationBuffer []byte) error {
	var errs tunnelErr

	// Use the provided context directly
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		_, errs[0] = io.CopyBuffer(source, destination, sourceBuffer)
		cancel()
	}()

	go func() {
		_, errs[1] = io.CopyBuffer(destination, source, destinationBuffer)
		cancel()
	}()

	<-ctx.Done()

	// Close both source and destination, and check for errors
	errs[2] = source.Close()
	errs[3] = destination.Close()
	errs[4] = ctx.Err()

	// If the context was canceled, set it to nil in the error slice
	if errs[4] == context.Canceled {
		errs[4] = nil
	}

	// Return the first non-nil error, ignoring closed connection errors
	return errs.FirstError()
}

// tunnelErr is a type that aggregates multiple errors.
type tunnelErr [5]error

// FirstError returns the first non-nil error, ignoring closed connection errors.
func (t tunnelErr) FirstError() error {
	for _, err := range t {
		if err != nil {
			if isClosedConnError(err) {
				return nil
			}
			return err
		}
	}
	return nil
}
