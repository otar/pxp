//go:build darwin || linux

package interrupt

import (
	"context"
	"errors"
	"fmt"
	"syscall"
)

type Cause struct {
	Signal syscall.Signal
}

func (e *Cause) Error() string {
	return fmt.Sprintf("received %s", e.Signal)
}

// Signal returns the originating signal recorded as the context cancellation
// cause. If cancellation did not originate from a signal, ok is false.
func Signal(ctx context.Context) (signal syscall.Signal, ok bool) {
	var cause *Cause
	if errors.As(context.Cause(ctx), &cause) {
		return cause.Signal, true
	}
	return 0, false
}

func ExitCode(ctx context.Context) int {
	if signal, ok := Signal(ctx); ok && signal == syscall.SIGTERM {
		return 143
	}
	return 130
}
