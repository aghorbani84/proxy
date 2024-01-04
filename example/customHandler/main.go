package main

import (
	"fmt"
	"io"
	"log"
	"net"

	"github.com/bepass-org/proxy/pkg/mixed"
	"github.com/bepass-org/proxy/pkg/statute"
)

func main() {
	// Create a new mixed proxy with custom options
	proxy := mixed.NewProxy(
		mixed.WithBinAddress("127.0.0.1:1080"),
		mixed.WithUserHandler(generalHandler),
	)

	// Start the proxy server
	_ = proxy.ListenAndServe()
}

// generalHandler is a user-defined handler function for processing proxy requests.
func generalHandler(req *statute.ProxyRequest) error {
	fmt.Println("Handling request to", req.Destination)

	// Establish a connection to the destination address
	conn, err := net.Dial(req.Network, req.Destination)
	if err != nil {
		return err
	}

	// Start a goroutine to copy data from the incoming connection to the destination
	go func() {
		_, err := io.Copy(conn, req.Conn)
		if err != nil {
			log.Println(err)
		}
	}()

	// Copy data from the destination to the incoming connection
	_, err = io.Copy(req.Conn, conn)
	return err
}
