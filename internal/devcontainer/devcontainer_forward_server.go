package devcontainer

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type DevContainerForwardServer struct {
	listener   net.Listener
	shutdown   chan struct{}
	connection chan net.Conn
	forwardCh  chan<- net.Conn
	wg         sync.WaitGroup
}

func NewDevContainerForwardServer(port string, forwardCh chan<- net.Conn) (*DevContainerForwardServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%s", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on address 127.0.0.1:%s: %v", port, err)
	}

	return &DevContainerForwardServer{
		listener:   listener,
		shutdown:   make(chan struct{}),
		connection: make(chan net.Conn),
		forwardCh:  forwardCh,
	}, nil
}

func (server *DevContainerForwardServer) Start() {
	server.wg.Add(2)

	// Start two goroutines that will handle incoming connections, allowing
	// it to be cancelled by the Stop method
	go server.acceptConnections()
	go server.handleConnections()
}

func (server *DevContainerForwardServer) Stop() {
	close(server.shutdown)
	server.listener.Close()

	done := make(chan struct{})
	go func() {
		server.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return

	case <-time.After(time.Second):
		fmt.Println("timed out waiting for connections to finish")
		return
	}
}

func (server *DevContainerForwardServer) handleConnection(conn net.Conn) {
	// Pass the connection to the forward channel, and let the receiver handle the forwarding
	// It is up to the receiver to closes the connection when finished
	server.forwardCh <- conn
}

func (server *DevContainerForwardServer) handleConnections() {
	defer server.wg.Done()

	for {
		select {
		case <-server.shutdown:
			return

		case conn := <-server.connection:
			go server.handleConnection(conn)
		}
	}
}

func (server *DevContainerForwardServer) acceptConnections() {
	defer server.wg.Done()

	for {
		select {
		case <-server.shutdown:
			return

		default:
			conn, err := server.listener.Accept()
			if err != nil {
				fmt.Printf("failed to accept connection due to: %v\n", err)
				continue
			}

			// Add the connection to the channel so it will be handled correctly
			server.connection <- conn
		}
	}
}
