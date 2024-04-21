package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Gets the remote user configured for the devcontainer, or uses the
// provided `remoteUser` when it is defined.
func GetRemoteUser(containerId, remoteUser string) (string, error) {
	if remoteUser != "" {
		return remoteUser, nil
	}

	cmd := exec.Command(
		"docker",
		"inspect",
		"-f",
		"{{ index .Config.Labels \"devcontainer.metadata\" }}",
		containerId,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		return "", err
	}

	var metadata []map[string]any
	if err = json.Unmarshal(output, &metadata); err != nil {
		return "", err
	}

	for _, item := range metadata {
		if item["remoteUser"] == nil || item["remoteUser"] == "" {
			continue
		}

		return item["remoteUser"].(string), nil
	}

	return "", nil
}

// Gets the ID of the devcontainer belonging to the provided workspace.
func GetContainerID(workspace string) (string, error) {
	localFolder := filepath.Join(workspace)
	configFile := filepath.Join(workspace, ".devcontainer/devcontainer.json")

	cmd := exec.Command(
		"docker",
		"ps",
		"-q",
		"--filter",
		fmt.Sprintf("label=devcontainer.local_folder=%s", localFolder),
		"--filter",
		fmt.Sprintf("label=devcontainer.config_file=%s", configFile),
		"--filter",
		"status=running",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.New(string(output))
	}

	if len(output) == 0 {
		return "", errors.New("failed to find container ID")
	}

	return strings.TrimSpace(string(output)), nil
}

// Forwards a port into the container
func ForwardPort(client io.ReadWriteCloser, containerId, port, remoteUser string) error {
	cmd := exec.Command(
		"docker",
		"exec",
		"-i",
		containerId,
		"bash",
		"-c",
		fmt.Sprintf("su - %s -c 'socat - TCP:localhost:%s'", remoteUser, port),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}
	defer cmd.Wait()

	// Forward the connection to docker
	forward(client, stdout, stdin)

	return nil
}

// Monitors the container for activity and closes the returned channel when the
// container stopped running, indicating the forwarding should stop
func MonitorContainer(containerId string) <-chan struct{} {
	// Used to notify the caller that the container has stopped running
	stopCh := make(chan struct{})

	go func() {
		defer func() {
			log.Println("stopped monitoring container:", containerId)
		}()

		for {
			running := expectContainer(containerId, ".State.Running", "true")
			restarting := expectContainer(containerId, ".State.Restarting", "true")
			creating := expectContainer(containerId, ".State.Status", "created")

			if (!creating && !restarting) && !running {
				log.Println("should stop running forwarded ports")
				close(stopCh)

				return
			}

			time.Sleep(1 * time.Second)
		}
	}()

	return stopCh
}

func forward(client io.ReadWriteCloser, stdout io.ReadCloser, stdin io.WriteCloser) {
	defer client.Close()
	defer stdout.Close()
	defer stdin.Close()

	done := make(chan struct{}, 2)

	go func() {
		// Copy from docker to client
		io.Copy(client, stdout)
		done <- struct{}{}
	}()

	go func() {
		// Copy from client to docker
		io.Copy(stdin, client)
		done <- struct{}{}
	}()

	// Wait until streaming has finished
	<-done
}

// Checks whether the container is in the expected state. This is checked using a `docker inspect` call
// and checking the `field` for the expected `value`
func expectContainer(containerId, field, value string) bool {
	cmd := exec.Command(
		"docker",
		"inspect",
		"-f",
		fmt.Sprintf("{{%s}}", field),
		containerId,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("failed to inspect container: %v\n", err)
		return false
	}

	return strings.TrimSpace(string(output)) == value
}
