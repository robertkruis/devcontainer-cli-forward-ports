package devcontainer

import (
	"context"
	"io"
	"log/slog"
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
		slog.Error(
			"failed to load devcontainer.json",
			slog.String("path", jsonPath),
			slog.String("error", err.Error()))

		return nil, err
	}

	slog.Debug(
		"loaded forwarding configuration from devcontainer.json",
		slog.String("path", jsonPath),
		slog.String("config", config.String()))

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
		slog.Error(
			"failed to create forwarder",
			slog.String("error", err.Error()))

		return nil, err
	}

	devcontainerForwarder.forwarder = forwarder

	return devcontainerForwarder, nil
}

func (f *DevContainerForwarder) Start() (<-chan struct{}, error) {
	eventsCh, err := docker.Events(f.ctx, f.workspace, f.jsonPath, 10*time.Second)
	if err != nil {
		f.cancelFn()

		slog.Error(
			"failed to get docker events",
			slog.String("error", err.Error()))

		return nil, err
	}

	forwardDoneCh, err := f.forwarder.Start(f.ctx)
	if err != nil {
		f.cancelFn()

		slog.Error(
			"failed to start forwarding",
			slog.String("error", err.Error()))

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
			switch event.Action {
			case "start":
				slog.Debug(
					"container started, forwarding",
					slog.String("container-id", event.ID),
					slog.String("remote-user", event.RemoteUser))

				f.remoteUser = event.RemoteUser
				f.containerId = event.ID

			case "die":
				slog.Debug(
					"container died, waiting for restart",
					slog.String("container-id", event.ID))

				containerRestartTimer.Reset(5 * time.Second)

			case "restart":
				slog.Debug(
					"container (re)started, forwarding",
					slog.String("container-id", event.ID))

				if !containerRestartTimer.Stop() {
					<-containerRestartTimer.C
				}
			}

		case <-containerRestartTimer.C:
			slog.Debug(
				"container failed to (re)start, stopping forwarding",
				slog.String("container-id", f.containerId))

			f.cancelFn()
			return
		}
	}
}

func (f *DevContainerForwarder) forwardToContainer(conn io.ReadWriteCloser, port int) {
	err := docker.ForwardPort(conn, f.containerId, strconv.Itoa(port), f.remoteUser)
	if err != nil {
		slog.Error(
			"failed to forward port to container",
			slog.String("container-id", f.containerId),
			slog.String("remote-user", f.remoteUser),
			slog.String("error", err.Error()))
	}
}
