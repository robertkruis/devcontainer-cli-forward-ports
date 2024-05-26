package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type EventMessage struct {
	Status     string
	ID         string
	Action     string
	RemoteUser string
}

func (msg *EventMessage) UnmarshalJSON(data []byte) error {
	var objmap map[string]*json.RawMessage

	err := json.Unmarshal(data, &objmap)
	if err != nil {
		return err
	}

	err = json.Unmarshal(*objmap["status"], &msg.Status)
	if err != nil {
		return err
	}

	err = json.Unmarshal(*objmap["id"], &msg.ID)
	if err != nil {
		return err
	}

	err = json.Unmarshal(*objmap["Action"], &msg.Action)
	if err != nil {
		return err
	}

	var actormap map[string]*json.RawMessage
	err = json.Unmarshal(*objmap["Actor"], &actormap)
	if err != nil {
		return err
	}

	var attributes map[string]*json.RawMessage
	err = json.Unmarshal(*actormap["Attributes"], &attributes)
	if err != nil {
		return err
	}

	var metadata string
	err = json.Unmarshal(*attributes["devcontainer.metadata"], &metadata)
	if err != nil {
		return err
	}

	var metaobj []map[string]*json.RawMessage
	err = json.Unmarshal([]byte(metadata), &metaobj)
	if err != nil {
		return err
	}

	for _, item := range metaobj {
		if item["remoteUser"] == nil {
			continue
		}

		err = json.Unmarshal(*item["remoteUser"], &msg.RemoteUser)
		if err != nil {
			return err
		}

		break
	}

	return nil
}

func Events(ctx context.Context, workspace, configFile string, since time.Duration) (<-chan EventMessage, error) {
	eventsCh := make(chan EventMessage)
	outputCh := make(chan string, 1)
	errCh := make(chan string, 1)

	cleanup := func() {
		close(eventsCh)
		close(outputCh)
		close(errCh)
	}

	cmd := exec.CommandContext(
		ctx,
		"docker",
		"events",
		"-f",
		"type=container",
		"-f",
		fmt.Sprintf("label=devcontainer.local_folder=%s", workspace),
		"-f",
		fmt.Sprintf("label=devcontainer.config_file=%s", configFile),
		"--format",
		"json",
		// "{{.ID}},{{.Status}},{{ index .Actor.Attributes \"devcontainer.metadata\" | json}}",
		"--since",
		since.String(),
	)

	cmd.Stdout = newOutputStream(outputCh)
	cmd.Stderr = newOutputStream(errCh)

	// Bail out in case of an error
	if err := cmd.Start(); err != nil {
		cleanup()

		return nil, err
	}

	startedCh := make(chan struct{})
	go func(outputCh chan string) {
		close(startedCh)

		for {
			select {
			case <-ctx.Done():
				cleanup()
				return

			case line, ok := <-outputCh:
				if !ok {
					return
				}

				// fmt.Println("line:", line, ", ok:", ok)
				var msg EventMessage
				json.Unmarshal([]byte(line), &msg)
				// fmt.Printf("%#v\n", msg)
				eventsCh <- msg
			}
		}
	}(outputCh)

	// Wait until the started signal is closed so we can return the channel or error
	<-startedCh

	// Bail out if the context is cancelled
	select {
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()

	case errLine := <-errCh:
		cleanup()

		return nil, errors.New(errLine)

	// Wait 500ms to allow the errCh to be notified of any startup errors
	case <-time.After(500 * time.Millisecond):
		break
	}

	return eventsCh, nil
}

type ErrLineBufferOverflow struct {
	Line       string
	BufferSize int
	BufferFree int
}

func (e ErrLineBufferOverflow) Error() string {
	return fmt.Sprintf(
		"line does not contain newline and is %d bytes too long for buffer (buffer size: %d)",
		len(e.Line)-e.BufferSize,
		e.BufferSize)
}

const (
	DEFAULT_LINE_BUFFER_SIZE = 16384
	DEFAULT_STREAM_CHAN_SIZE = 1000
)

type outputStream struct {
	streamCh chan string
	buf      []byte
	bufSize  int
	lastChar int
}

func newOutputStream(streamCh chan string) *outputStream {
	return &outputStream{
		streamCh: streamCh,
		bufSize:  DEFAULT_LINE_BUFFER_SIZE,
		buf:      make([]byte, DEFAULT_LINE_BUFFER_SIZE),
		lastChar: 0,
	}
}

func (rw *outputStream) Write(p []byte) (int, error) {
	n := len(p)
	firstChar := 0

	for {
		newlineOffset := bytes.IndexByte(p[firstChar:], '\n')
		if newlineOffset < 0 {
			break
		}

		lastChar := firstChar + newlineOffset
		if newlineOffset > 0 && p[newlineOffset-1] == '\r' {
			lastChar -= 1
		}

		var line string
		if rw.lastChar > 0 {
			line = string(rw.buf[0:rw.lastChar])
			rw.lastChar = 0
		}

		line += string(p[firstChar:lastChar])
		rw.streamCh <- line

		firstChar += newlineOffset + 1
	}

	if firstChar < n {
		remaining := len(p[firstChar:])
		bufFree := len(rw.buf[rw.lastChar:])

		if remaining > bufFree {
			var line string
			if rw.lastChar > 0 {
				line = string(rw.buf[0:rw.lastChar])
			}

			line += string(p[firstChar:])
			err := ErrLineBufferOverflow{
				Line:       line,
				BufferSize: rw.bufSize,
				BufferFree: bufFree,
			}
			n = firstChar

			return n, err
		}

		copy(rw.buf[rw.lastChar:], p[firstChar:])
		rw.lastChar += remaining
	}

	return n, nil
}

func (rw *outputStream) Lines() <-chan string {
	return rw.streamCh
}

func (rw *outputStream) Flush() {
	if rw.lastChar != 0 {
		return
	}

	line := string(rw.buf[0:rw.lastChar])
	rw.streamCh <- line
}
