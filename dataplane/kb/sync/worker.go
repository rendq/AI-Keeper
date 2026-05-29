package sync

import (
	"context"
	"fmt"
	"sync"
)

// ConnectorTask defines the interface for a unit of work submitted to the worker pool.
type ConnectorTask interface {
	// Execute performs the task. Returns an error if execution fails.
	Execute(ctx context.Context) error
}

// TaskResult holds the outcome of a task execution.
type TaskResult struct {
	// Task is the original task that was executed.
	Task ConnectorTask
	// Err is non-nil if the task failed.
	Err error
}

// ConnectorWorkerPool manages a pool of goroutines that process connector tasks concurrently.
type ConnectorWorkerPool struct {
	workerCount int
	taskChan    chan ConnectorTask
	resultChan  chan TaskResult
	wg          sync.WaitGroup
	cancel      context.CancelFunc
	ctx         context.Context
	started     bool
	stopped     bool
	mu          sync.Mutex
}

// NewConnectorWorkerPool creates a new worker pool with the specified number of workers.
// The pool must be started with Start() before submitting tasks.
func NewConnectorWorkerPool(workerCount int) *ConnectorWorkerPool {
	if workerCount <= 0 {
		workerCount = 4
	}
	return &ConnectorWorkerPool{
		workerCount: workerCount,
		taskChan:    make(chan ConnectorTask, workerCount*2),
		resultChan:  make(chan TaskResult, workerCount*2),
	}
}

// Start launches the worker goroutines. It is safe to call only once.
func (p *ConnectorWorkerPool) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return
	}
	p.started = true
	p.ctx, p.cancel = context.WithCancel(ctx)

	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(p.ctx)
	}
}

// Submit adds a task to the worker pool for execution.
// Returns an error if the pool has been stopped or the context is cancelled.
func (p *ConnectorWorkerPool) Submit(task ConnectorTask) error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return fmt.Errorf("worker pool is stopped")
	}
	if !p.started {
		p.mu.Unlock()
		return fmt.Errorf("worker pool is not started")
	}
	p.mu.Unlock()

	select {
	case p.taskChan <- task:
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("worker pool context cancelled: %w", p.ctx.Err())
	}
}

// Results returns the channel where task results are published.
// Consumers should read from this channel to collect execution outcomes.
func (p *ConnectorWorkerPool) Results() <-chan TaskResult {
	return p.resultChan
}

// Stop performs a graceful shutdown of the worker pool.
// It closes the task channel, waits for in-flight tasks to complete,
// then closes the result channel.
func (p *ConnectorWorkerPool) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	// Close task channel so workers drain remaining tasks and exit.
	close(p.taskChan)
	// Wait for all workers to finish.
	p.wg.Wait()
	// Cancel the context.
	if p.cancel != nil {
		p.cancel()
	}
	// Close result channel after all workers are done.
	close(p.resultChan)
}

// WorkerCount returns the number of workers in the pool.
func (p *ConnectorWorkerPool) WorkerCount() int {
	return p.workerCount
}

// worker is the main loop for a single pool worker.
func (p *ConnectorWorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()
	for task := range p.taskChan {
		select {
		case <-ctx.Done():
			p.resultChan <- TaskResult{Task: task, Err: ctx.Err()}
			return
		default:
		}

		err := task.Execute(ctx)
		p.resultChan <- TaskResult{Task: task, Err: err}
	}
}
