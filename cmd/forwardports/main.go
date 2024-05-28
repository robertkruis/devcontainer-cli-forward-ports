package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/robertkruis/devcontainer-cli-forward-ports/internal/devcontainer"
)

func main() {
	loggingLevel := new(slog.LevelVar)

	dateString := time.Now().Format("20060102")
	logFile, err := os.OpenFile(fmt.Sprintf("forwardports-%s.log", dateString), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal("unable to access log file")
	}
	defer logFile.Close()

	logWriter := io.MultiWriter(os.Stdout, logFile)

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal("unable to determine working directory")
	}
	wdSlashed := filepath.ToSlash(wd)

	replacer := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key != slog.SourceKey {
			return a
		}

		source := a.Value.Any().(*slog.Source)
		// remove current working directory and only leave the relative path to the program
		if file, ok := strings.CutPrefix(source.File, wdSlashed); ok {
			source.File = file
		}

		return a
	}

	options := &slog.HandlerOptions{
		Level:       loggingLevel,
		ReplaceAttr: replacer,
	}

	// Define command line flags
	debug := flag.Bool("debug", false, "enables debug logging")
	workspace := flag.String("workspace-folder", wd, "the workspace folder on which to operate")
	flag.Parse()

	if *debug {
		loggingLevel.Set(slog.LevelDebug)
	}

	logger := slog.New(slog.NewTextHandler(logWriter, options))
	slog.SetDefault(logger)

	logger.Info(
		"starting forwarder",
		slog.Bool("debug", *debug),
		slog.String("workspace-folder", *workspace))

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	forwarder, err := devcontainer.NewDevContainerForwarder(signalCtx, *workspace)
	if err != nil {
		logger.Error(
			"failed to create new forwarder",
			slog.String("error", err.Error()))

		return
	}

	doneCh, err := forwarder.Start()
	if err != nil {
		logger.Error(
			"failed to start forwarder",
			slog.String("error", err.Error()))

		return
	}

	go func() {
		// Wait until the context is cancelled
		<-signalCtx.Done()
	}()

	<-doneCh
	logger.Info("stopped forwarder")
}
