package boundedjobs

import (
	"context"
	"fmt"
)

type Job func(context.Context) (string, error)

// Run executes jobs with bounded concurrency and stable result ordering.
func Run(ctx context.Context, workers int, jobs []Job) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
