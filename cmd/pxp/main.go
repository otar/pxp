package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/otar/pxp/internal/cli"
	"github.com/otar/pxp/internal/interrupt"
)

func main() {
	ctx, cancel := context.WithCancelCause(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case received := <-signals:
			cancel(&interrupt.Cause{Signal: received.(syscall.Signal)})
			// Stop further input and release the process-owned descriptor. The
			// CLI also selects plan reads against ctx because an in-flight Unix
			// pipe read is not guaranteed to wake merely because another goroutine
			// closes the descriptor.
			_ = os.Stdin.Close()
		case <-done:
		}
	}()
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error[internal]: determine working directory: %v\n", err)
		close(done)
		signal.Stop(signals)
		cancel(nil)
		os.Exit(cli.ExitInternal)
	}

	code := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cwd)
	close(done)
	signal.Stop(signals)
	cancel(nil)
	os.Exit(code)
}
