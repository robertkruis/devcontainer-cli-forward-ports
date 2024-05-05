package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/robertkruis/devcontainer-cli-forward-ports/internal/docker"
)

func main() {
	id, _ := docker.GetContainerID("d:\\dev\\devcontainer")
	remoteUser, _ := docker.GetRemoteUser(id, "")

	fmt.Printf("container: %s\nremote user: %s\n", id, remoteUser)

	var wg sync.WaitGroup

	l, _ := net.Listen("tcp", "localhost:5432")

	stopCh := docker.MonitorContainer(id)
	wg.Add(1)
	go func(listener net.Listener) {
		defer wg.Done()
		<-stopCh
		fmt.Println("docker container stopped")
		listener.Close()
	}(l)

	for {
		c, err := l.Accept()
		if err != nil {
			fmt.Printf("failed to accept connection: %v\n", err)
			break
		}

		docker.ForwardPort(c, id, "5432", remoteUser)
	}

	wg.Wait()
}
