package guardrail

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// RuleSetUpdate represents a rule set change notification pushed via pub/sub.
type RuleSetUpdate struct {
	// Rules is the new complete set of guardrail rules.
	Rules []Rule `json:"rules"`
	// Version is a monotonically increasing version for dedup.
	Version int64 `json:"version"`
	// Timestamp is when the update was published.
	Timestamp time.Time `json:"timestamp"`
}

// Subscriber abstracts the pub/sub subscription mechanism.
// Implementations can use Redis, NATS, or any message bus.
type Subscriber interface {
	// Subscribe returns a channel that receives raw rule update messages.
	// The channel is closed when ctx is cancelled.
	Subscribe(ctx context.Context, subject string) (<-chan []byte, error)
}

// ReloadWatcher watches for guardrail rule updates via pub/sub and applies them to the engine.
// It ensures all Agent Runtime instances receive new rules within 60 seconds.
type ReloadWatcher struct {
	engine     *Engine
	subscriber Subscriber
	subject    string
	log        logr.Logger

	mu             sync.RWMutex
	currentVersion int64
	lastReload     time.Time
}

// NewReloadWatcher creates a watcher that listens for rule updates and applies them to the engine.
func NewReloadWatcher(engine *Engine, subscriber Subscriber, subject string, log logr.Logger) *ReloadWatcher {
	return &ReloadWatcher{
		engine:     engine,
		subscriber: subscriber,
		subject:    subject,
		log:        log,
	}
}

// Start begins watching for rule updates. Blocks until ctx is cancelled.
func (w *ReloadWatcher) Start(ctx context.Context) error {
	msgCh, err := w.subscriber.Subscribe(ctx, w.subject)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgCh:
			if !ok {
				return nil
			}
			w.handleUpdate(msg)
		}
	}
}

// handleUpdate parses and applies a rule update if it's newer than current version.
func (w *ReloadWatcher) handleUpdate(data []byte) {
	var update RuleSetUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		w.log.Error(err, "failed to unmarshal guardrail rule update")
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Dedup: only apply if version is newer
	if update.Version <= w.currentVersion {
		w.log.V(1).Info("skipping stale guardrail update",
			"current", w.currentVersion, "received", update.Version)
		return
	}

	w.engine.SetRules(update.Rules)
	w.currentVersion = update.Version
	w.lastReload = time.Now()

	w.log.Info("guardrail rules reloaded",
		"version", update.Version,
		"ruleCount", len(update.Rules))
}

// CurrentVersion returns the last applied rule version.
func (w *ReloadWatcher) CurrentVersion() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.currentVersion
}

// LastReload returns the timestamp of the last successful reload.
func (w *ReloadWatcher) LastReload() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastReload
}
