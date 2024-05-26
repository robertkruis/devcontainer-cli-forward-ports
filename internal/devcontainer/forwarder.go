package devcontainer

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/robertkruis/devcontainer-cli-forward-ports/internal/docker"
	"github.com/robertkruis/devcontainer-cli-forward-ports/internal/network"
)

type DevContainerForwarder struct {
	forwarder   *network.Forwarder
	ctx         context.Context
	cancelFn    context.CancelFunc
	workspace   string
	jsonPath    string
	containerId string
	remoteUser  string
	ports       []int
}

func NewDevContainerForwarder(ctx context.Context, workspace string) (*DevContainerForwarder, error) {
	jsonPath := filepath.Join(workspace, ".devcontainer", "devcontainer.json")

	config, err := loadForwardPortsConfig(jsonPath)
	if err != nil {
		fmt.Printf("failed to load %s: %v\n", jsonPath, err)
		return nil, err
	}

	cancelCtx, cancelFn := context.WithCancel(ctx)
	devcontainerForwarder := &DevContainerForwarder{
		ctx:         cancelCtx,
		cancelFn:    cancelFn,
		workspace:   workspace,
		jsonPath:    jsonPath,
		containerId: "",
		remoteUser:  config.RemoteUser,
		ports:       config.ForwardPorts,
	}

	forwarder, err := network.NewForwarder(
		config.ForwardPorts,
		devcontainerForwarder.forwardToContainer)
	if err != nil {
		fmt.Printf("failed to create forwarder: %v\n", err)
		return nil, err
	}

	devcontainerForwarder.forwarder = forwarder

	return devcontainerForwarder, nil
}

func (f *DevContainerForwarder) Start() (<-chan struct{}, error) {
	eventsCh, err := docker.Events(f.ctx, f.workspace, f.jsonPath, 10*time.Second)
	if err != nil {
		f.cancelFn()
		return nil, err
	}

	forwardDoneCh, err := f.forwarder.Start(f.ctx)
	if err != nil {
		f.cancelFn()
		return nil, err
	}

	go f.handleEvents(f.ctx, eventsCh)

	doneCh := make(chan struct{})
	go func() {
		<-f.ctx.Done()
		<-eventsCh
		<-forwardDoneCh
		close(doneCh)
	}()

	return doneCh, nil
}

func (f *DevContainerForwarder) handleEvents(ctx context.Context, events <-chan docker.EventMessage) {
	containerRestartTimer := time.NewTimer(0)
	if !containerRestartTimer.Stop() {
		<-containerRestartTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-events:
			// fmt.Println("docker event:", event)

			if event.Action == "start" {
				fmt.Printf("container %s started, allow forwarding\n", event.ID)
				f.remoteUser = event.RemoteUser
				f.containerId = event.ID
			}

			if event.Action == "die" {
				fmt.Printf("container %s died, maybe stop forwarding\n", event.ID)
				containerRestartTimer.Reset(5 * time.Second)
			}

			if event.Action == "restart" {
				fmt.Printf("container %s restarted, allow forwarding\n", event.ID)

				if !containerRestartTimer.Stop() {
					<-containerRestartTimer.C
				}
			}

		case <-containerRestartTimer.C:
			fmt.Printf("container failed to (re)start, bailing out\n")
			f.cancelFn()
			fmt.Println("cancel forwarding and event handling")
			return
		}
	}
}

func (f *DevContainerForwarder) forwardToContainer(conn io.ReadWriteCloser, port int) {
	err := docker.ForwardPort(conn, f.containerId, strconv.Itoa(port), f.remoteUser)
	if err != nil {
		fmt.Printf("failed to forward port %d to container: %v\n", port, err)
	}
}
