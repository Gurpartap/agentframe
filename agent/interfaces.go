package agent

import "context"

// RunStore persists and reloads run state for continuation and observability.
// Save uses optimistic concurrency based on RunState.Version and bumps it by one on success.
type RunStore interface {
	Save(ctx context.Context, state RunState) error
	Load(ctx context.Context, runID RunID) (RunState, error)
}

// EventSink receives normalized runtime events.
type EventSink interface {
	Publish(ctx context.Context, event Event) error
}

// IDGenerator creates run IDs at the runtime boundary.
type IDGenerator interface {
	NewRunID(ctx context.Context) (RunID, error)
}
