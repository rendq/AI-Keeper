package knowledgebase

import (
	"context"
	"errors"
	"sync"
	"time"

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
)

// PipelineInfo summarises the state of the indexing pipeline after a
// successful [Pipeline.Index] call. The reconciler copies these values
// into `status.{chunkCount, indexSizeBytes, lastIndexedAt}`.
type PipelineInfo struct {
	// ChunkCount is the number of chunks the pipeline produced.
	ChunkCount int64

	// IndexSizeBytes is the on-disk size of the index in bytes.
	IndexSizeBytes int64

	// LastIndexedAt is the timestamp of the pipeline's last successful
	// run. Defaults to time.Now() in [NoopPipeline].
	LastIndexedAt time.Time
}

// Pipeline is the abstraction over the indexing pipeline (chunking →
// embedding → enrichment → vector store upsert). The KnowledgeBase
// controller drives the pipeline once per reconcile in P0; the full
// scheduled flow lands in P1.
//
// Implementations MUST be idempotent: every call may be retried.
type Pipeline interface {
	// Index runs (or refreshes) the indexing pipeline for the supplied
	// KnowledgeBase and returns the pipeline's current view. The error
	// is non-nil only on transport / programming failures.
	Index(ctx context.Context, kb *datav1alpha1.KnowledgeBase) (PipelineInfo, error)
}

// NoopPipeline is the in-memory [Pipeline] used by unit tests.
type NoopPipeline struct {
	mu sync.Mutex

	// Info is the value returned to callers. Default: a stub with
	// non-zero counters so the Indexed gate flips True.
	Info PipelineInfo

	// Err is the error returned to callers. Default: nil.
	Err error

	// Calls is incremented on every successful call.
	Calls int

	// Last captures the KB the last successful call observed.
	Last *datav1alpha1.KnowledgeBase
}

// NewNoopPipeline returns a NoopPipeline with non-zero counters so the
// Indexed gate flips True on the first reconcile.
func NewNoopPipeline() *NoopPipeline {
	return &NoopPipeline{
		Info: PipelineInfo{
			ChunkCount:     500,
			IndexSizeBytes: 8 * 1024 * 1024,
			LastIndexedAt:  time.Now(),
		},
	}
}

// Index implements [Pipeline].
func (n *NoopPipeline) Index(_ context.Context, kb *datav1alpha1.KnowledgeBase) (PipelineInfo, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Err != nil {
		return PipelineInfo{}, n.Err
	}
	n.Calls++
	if kb != nil {
		n.Last = kb.DeepCopy()
	}
	info := n.Info
	if info.LastIndexedAt.IsZero() {
		info.LastIndexedAt = time.Now()
	}
	return info, nil
}

// Snapshot returns the recorded call count under the mutex.
func (n *NoopPipeline) Snapshot() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.Calls
}

// ErrPipelineUnavailable is the canonical sentinel for tests that
// want to simulate a transient pipeline outage.
var ErrPipelineUnavailable = errors.New("knowledgebase: pipeline unavailable")

// Compile-time interface assertions.
var _ Pipeline = (*NoopPipeline)(nil)
