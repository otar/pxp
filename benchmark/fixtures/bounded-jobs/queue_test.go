package boundedjobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	var active, maximum atomic.Int32
	jobs := make([]Job, 12)
	for index := range jobs {
		index := index
		jobs[index] = func(context.Context) (string, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for {
				seen := maximum.Load()
				if current <= seen || maximum.CompareAndSwap(seen, current) {
					break
				}
			}
			time.Sleep(time.Duration(12-index) * time.Millisecond)
			return fmt.Sprintf("%02d", index), nil
		}
	}
	got, err := Run(context.Background(), 3, jobs)
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Load() > 3 || len(got) != len(jobs) {
		t.Fatalf("maximum=%d results=%v", maximum.Load(), got)
	}
	for index := range got {
		if want := fmt.Sprintf("%02d", index); got[index] != want {
			t.Fatalf("results[%d]=%q want=%q", index, got[index], want)
		}
	}
}

func TestRunCancelsAndStopsScheduling(t *testing.T) {
	boom := errors.New("boom")
	started := make(chan int, 8)
	release := make(chan struct{})
	jobs := make([]Job, 8)
	for index := range jobs {
		index := index
		jobs[index] = func(ctx context.Context) (string, error) {
			started <- index
			if index == 1 {
				return "", boom
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-release:
				return fmt.Sprint(index), nil
			}
		}
	}
	_, err := Run(context.Background(), 2, jobs)
	if !errors.Is(err, boom) {
		t.Fatalf("error=%v want boom", err)
	}
	close(release)
	close(started)
	count := 0
	for range started {
		count++
	}
	if count > 2 {
		t.Fatalf("started %d jobs after early failure", count)
	}
}

func TestRunValidationAndParentCancellation(t *testing.T) {
	if _, err := Run(context.Background(), 0, nil); err == nil {
		t.Fatal("workers=0 succeeded")
	}
	got, err := Run(context.Background(), 1, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty Run() = %v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, 1, []Job{func(context.Context) (string, error) { return "unexpected", nil }}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Run() error=%v", err)
	}
}

func TestRunFirstObservedErrorByInputOrder(t *testing.T) {
	errZero := errors.New("zero")
	errOne := errors.New("one")
	barrier := make(chan struct{})
	var once sync.Once
	jobs := []Job{
		func(context.Context) (string, error) { <-barrier; return "", errZero },
		func(context.Context) (string, error) { once.Do(func() { close(barrier) }); return "", errOne },
	}
	if _, err := Run(context.Background(), 2, jobs); !errors.Is(err, errZero) {
		t.Fatalf("error=%v want first input error", err)
	}
}
