package sync

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// countingTask increments a counter when executed.
type countingTask struct {
	counter *atomic.Int64
	delay   time.Duration
}

func (t *countingTask) Execute(_ context.Context) error {
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
	t.counter.Add(1)
	return nil
}

// failingTask always returns an error.
type failingTask struct {
	err error
}

func (t *failingTask) Execute(_ context.Context) error {
	return t.err
}

func TestWorkerPool_ProcessesTasks(t *testing.T) {
	pool := NewConnectorWorkerPool(4)
	ctx := context.Background()
	pool.Start(ctx)

	var counter atomic.Int64
	taskCount := 20

	for i := 0; i < taskCount; i++ {
		err := pool.Submit(&countingTask{counter: &counter})
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}

	// Collect results
	for i := 0; i < taskCount; i++ {
		result := <-pool.Results()
		if result.Err != nil {
			t.Errorf("task error: %v", result.Err)
		}
	}

	pool.Stop()

	if counter.Load() != int64(taskCount) {
		t.Errorf("expected %d tasks executed, got %d", taskCount, counter.Load())
	}
}

func TestWorkerPool_ConcurrentExecution(t *testing.T) {
	pool := NewConnectorWorkerPool(4)
	ctx := context.Background()
	pool.Start(ctx)

	var counter atomic.Int64
	taskCount := 8

	start := time.Now()
	for i := 0; i < taskCount; i++ {
		err := pool.Submit(&countingTask{counter: &counter, delay: 50 * time.Millisecond})
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}

	// Collect all results
	for i := 0; i < taskCount; i++ {
		<-pool.Results()
	}
	elapsed := time.Since(start)

	pool.Stop()

	// With 4 workers and 8 tasks at 50ms each, total should be ~100ms, not 400ms
	if elapsed > 300*time.Millisecond {
		t.Errorf("tasks not running concurrently: took %v (expected ~100ms)", elapsed)
	}

	if counter.Load() != int64(taskCount) {
		t.Errorf("expected %d tasks executed, got %d", taskCount, counter.Load())
	}
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	pool := NewConnectorWorkerPool(2)
	ctx := context.Background()
	pool.Start(ctx)

	var counter atomic.Int64
	taskCount := 4

	for i := 0; i < taskCount; i++ {
		err := pool.Submit(&countingTask{counter: &counter, delay: 20 * time.Millisecond})
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}

	// Stop should wait for in-flight tasks to complete
	// Drain results first to avoid blocking workers
	done := make(chan struct{})
	go func() {
		for range pool.Results() {
		}
		close(done)
	}()

	pool.Stop()
	<-done

	if counter.Load() != int64(taskCount) {
		t.Errorf("graceful shutdown did not complete all tasks: expected %d, got %d", taskCount, counter.Load())
	}
}

func TestWorkerPool_ErrorHandling(t *testing.T) {
	pool := NewConnectorWorkerPool(2)
	ctx := context.Background()
	pool.Start(ctx)

	expectedErr := fmt.Errorf("connector timeout")
	err := pool.Submit(&failingTask{err: expectedErr})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	result := <-pool.Results()
	if result.Err == nil {
		t.Error("expected error in result")
	}
	if result.Err.Error() != expectedErr.Error() {
		t.Errorf("expected error %q, got %q", expectedErr, result.Err)
	}

	pool.Stop()
}

func TestWorkerPool_SubmitAfterStop(t *testing.T) {
	pool := NewConnectorWorkerPool(2)
	ctx := context.Background()
	pool.Start(ctx)

	// Drain results in background
	go func() {
		for range pool.Results() {
		}
	}()

	pool.Stop()

	err := pool.Submit(&countingTask{counter: &atomic.Int64{}})
	if err == nil {
		t.Error("expected error when submitting to stopped pool")
	}
}

func TestWorkerPool_SubmitBeforeStart(t *testing.T) {
	pool := NewConnectorWorkerPool(2)

	err := pool.Submit(&countingTask{counter: &atomic.Int64{}})
	if err == nil {
		t.Error("expected error when submitting to unstarted pool")
	}
}

func TestWorkerPool_DefaultWorkerCount(t *testing.T) {
	pool := NewConnectorWorkerPool(0)
	if pool.WorkerCount() != 4 {
		t.Errorf("expected default worker count 4, got %d", pool.WorkerCount())
	}
}

func TestWorkerPool_DoubleStart(t *testing.T) {
	pool := NewConnectorWorkerPool(2)
	ctx := context.Background()
	pool.Start(ctx)
	pool.Start(ctx) // Should be safe to call twice

	// Drain results in background
	go func() {
		for range pool.Results() {
		}
	}()

	pool.Stop()
}

func TestWorkerPool_DoubleStop(t *testing.T) {
	pool := NewConnectorWorkerPool(2)
	ctx := context.Background()
	pool.Start(ctx)

	// Drain results in background
	go func() {
		for range pool.Results() {
		}
	}()

	pool.Stop()
	pool.Stop() // Should be safe to call twice
}
