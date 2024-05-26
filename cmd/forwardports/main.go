package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/robertkruis/devcontainer-cli-forward-ports/internal/devcontainer"
)

func main() {
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	forwarder, err := devcontainer.NewDevContainerForwarder(signalCtx, "d:\\dev\\devcontainer")
	if err != nil {
		panic(err)
	}

	doneCh, err := forwarder.Start()
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		<-signalCtx.Done()
		fmt.Println("CTRL+C")
	}()

	<-doneCh
	fmt.Println("forwarder stopped")
}
