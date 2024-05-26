package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
)

var (
	ErrNoPorts             = errors.New("at least one port should be specified")
	ErrForwardingCancelled = errors.New("forwarding has been cancelled")
)

type ForwardFn = func(conn io.ReadWriteCloser, port int)

type Forwarder struct {
	connCh    chan net.Conn
	forwardFn ForwardFn
	wg        *sync.WaitGroup
	listeners []net.Listener
	ports     []int
}

func NewForwarder(ports []int, fn ForwardFn) (*Forwarder, error) {
	if len(ports) == 0 {
		return nil, ErrNoPorts
	}

	return &Forwarder{
		connCh:    make(chan net.Conn),
		forwardFn: fn,
		wg:        &sync.WaitGroup{},
		listeners: make([]net.Listener, 0),
		ports:     ports,
	}, nil
}

func (f *Forwarder) Start(ctx context.Context) (<-chan struct{}, error) {
	select {
	case <-ctx.Done():
		return nil, ErrForwardingCancelled

	default:
	}

	for _, port := range f.ports {
		listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err != nil {
			f.cleanup()
			f.wg.Wait()

			return nil, err
		}

		f.listeners = append(f.listeners, listener)

		f.wg.Add(1)
		go f.acceptConns(ctx, listener)
	}

	f.wg.Add(1)
	go f.handleConns(ctx)

	// Make sure the channel gets closed when all cleanup finished
	doneCh := make(chan struct{})
	go func() {
		<-ctx.Done()
		f.cleanup()
		f.wg.Wait()

		fmt.Println("closing doneCh")
		close(doneCh)
		fmt.Println("closed doneCh")
	}()

	return doneCh, nil
}

func (f *Forwarder) cleanup() {
	for _, listener := range f.listeners {
		if listener == nil {
			continue
		}

		listener.Close()
	}

	close(f.connCh)
}

func (f *Forwarder) acceptConns(ctx context.Context, listener net.Listener) {
	defer f.wg.Done()

	select {
	case <-ctx.Done():
		return
	default:
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("received context cancel on acceptConns")
			return

		default:
			conn, err := listener.Accept()
			if err != nil {
				fmt.Printf("failed to accept connection due to: %v\n", err)
				continue
			}

			f.connCh <- conn
		}
	}
}

func (f *Forwarder) handleConns(ctx context.Context) {
	defer f.wg.Done()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("received context cancel on handleConns")
			return

		case conn := <-f.connCh:
			go f.handleConn(ctx, conn)
		}
	}
}

func (f *Forwarder) handleConn(ctx context.Context, conn net.Conn) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	_, port, _ := net.SplitHostPort(conn.LocalAddr().String())
	fmt.Printf("received conn on port %s with remote address: %s\n", port, conn.RemoteAddr().String())

	p, _ := strconv.Atoi(port)
	f.forwardFn(conn, p)
}
