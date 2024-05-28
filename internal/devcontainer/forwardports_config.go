package devcontainer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type forwardPortsConfig struct {
	Name         string `json:"name"`
	RemoteUser   string `json:"remoteUser"`
	ForwardPorts []int  `json:"forwardPorts"`
}

func (fpc forwardPortsConfig) String() string {
	return fmt.Sprintf(
		"name=%s, remote user=%s, forward ports=%v",
		fpc.Name,
		fpc.RemoteUser,
		fpc.ForwardPorts,
	)
}

func loadForwardPortsConfig(jsonPath string) (*forwardPortsConfig, error) {
	file, err := os.Open(jsonPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	contents := normalizeJson(file)
	return extractForwardPortsConfig(contents)
}

func normalizeJson(file *os.File) string {
	var contents string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}

		contents += line
	}

	return contents
}

func extractForwardPortsConfig(jsonStr string) (*forwardPortsConfig, error) {
	var config forwardPortsConfig

	err := json.Unmarshal([]byte(jsonStr), &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
