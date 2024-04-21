package devcontainer

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/robertkruis/devcontainer-cli-forward-ports/internal/docker"
)

type DevContainerForwarder struct {
	forwardPortsConfig *forwardPortsConfig
	forwardCh          chan net.Conn
	containerStoppedCh <-chan struct{}
	workspace          string
	jsonPath           string
	containerId        string
	remoteUser         string
	forwardServers     []*DevContainerForwardServer
}

func NewDevContainerForwarder(workspace string) (*DevContainerForwarder, error) {
	jsonPath := filepath.Join(workspace, ".devcontainer", "devcontainer.json")

	config, err := loadForwardPortsConfig(jsonPath)
	if err != nil {
		fmt.Printf("failed to load %s: %v", jsonPath, err)
		return nil, err
	}

	return &DevContainerForwarder{
		workspace:          workspace,
		jsonPath:           jsonPath,
		forwardPortsConfig: config,
		forwardServers:     make([]*DevContainerForwardServer, len(config.ForwardPorts)),
		forwardCh:          make(chan net.Conn, 1),
	}, nil
}

func (forwarder *DevContainerForwarder) Start() error {
	defer forwarder.cleanup()

	containerId, err := docker.GetContainerID(forwarder.workspace)
	if err != nil {
		return err
	}
	fmt.Println("containerId:", containerId)
	forwarder.containerId = containerId

	remoteUser, err := docker.GetRemoteUser(containerId, forwarder.forwardPortsConfig.RemoteUser)
	if err != nil {
		return err
	}
	forwarder.remoteUser = remoteUser

	for i, port := range forwarder.forwardPortsConfig.ForwardPorts {
		server, err := NewDevContainerForwardServer(strconv.Itoa(port), forwarder.forwardCh)
		if err != nil {
			fmt.Printf("failed to creating forward server on port %d due to: %v\n", port, err)
			return err
		}

		// Assign the server and start it
		forwarder.forwardServers[i] = server
		server.Start()

		// Handle forwarded request in its own goroutine
		go forwarder.handleForward(strconv.Itoa(port))
	}

	// Monitor the container and wait until it stops
	containerStoppedCh := docker.MonitorContainer(containerId)
	forwarder.containerStoppedCh = containerStoppedCh
	<-forwarder.containerStoppedCh

	return nil
}

func (forwarder *DevContainerForwarder) cleanup() error {
	close(forwarder.forwardCh)

	// Stop all instances of the started servers
	for i, server := range forwarder.forwardServers {
		if server == nil {
			continue
		}

		fmt.Println("stopping server", i)
		server.Stop()
		fmt.Println("stopped server", i)
	}

	return nil
}

func (forwarder *DevContainerForwarder) handleForward(port string) {
	fmt.Printf("handleForward(%s)\n", port)

	for {
		select {
		case conn := <-forwarder.forwardCh:
			docker.ForwardPort(conn, forwarder.containerId, port, forwarder.remoteUser)

		case <-forwarder.containerStoppedCh:
			fmt.Printf("handleForward(%s) received quit signal\n", port)
			return
		}
	}
}
