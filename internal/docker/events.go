package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

func Events(ctx context.Context, workspace, configFile string, since time.Duration) (<-chan string, error) {
	eventsCh := make(chan string)
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
		"{{.ID}},{{.Status}}",
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

			case line := <-outputCh:
				eventsCh <- line
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
