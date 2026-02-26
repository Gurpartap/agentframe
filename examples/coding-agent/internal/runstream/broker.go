package runstream

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Gurpartap/agentframe/agent"
)

const DefaultHistoryLimit = 32

var (
	ErrCursorInvalid = errors.New("stream cursor is invalid")
	ErrCursorExpired = errors.New("stream cursor expired")
)

type StreamEvent struct {
	ID    int64       `json:"id"`
	Event agent.Event `json:"event"`
}

type Broker struct {
	mu           sync.RWMutex
	historyLimit int
	runs         map[agent.RunID]*runHistory
}

type runHistory struct {
	nextID int64
	events []StreamEvent
}

var _ agent.EventSink = (*Broker)(nil)

func New(historyLimit int) *Broker {
	if historyLimit <= 0 {
		historyLimit = DefaultHistoryLimit
	}
	return &Broker{
		historyLimit: historyLimit,
		runs:         make(map[agent.RunID]*runHistory),
	}
}

func (b *Broker) Publish(ctx context.Context, event agent.Event) error {
	if ctx == nil {
		return agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err := agent.ValidateEvent(event); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	history := b.runLocked(event.RunID)
	next := StreamEvent{
		ID:    history.nextID,
		Event: cloneEvent(event),
	}
	history.nextID++
	history.events = append(history.events, next)
	if len(history.events) > b.historyLimit {
		drop := len(history.events) - b.historyLimit
		history.events = history.events[drop:]
	}
	return nil
}

func (b *Broker) EventsAfter(runID agent.RunID, cursor int64) ([]StreamEvent, error) {
	if runID == "" {
		return nil, fmt.Errorf("%w: run_id is required", agent.ErrInvalidRunID)
	}
	if cursor < 0 {
		return nil, fmt.Errorf("%w: cursor must be non-negative", ErrCursorInvalid)
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	history, ok := b.runs[runID]
	if !ok {
		if cursor == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: no events for run %q", ErrCursorInvalid, runID)
	}

	if cursor >= history.nextID {
		return nil, fmt.Errorf(
			"%w: cursor=%d is beyond latest id=%d",
			ErrCursorInvalid,
			cursor,
			history.nextID-1,
		)
	}

	if len(history.events) > 0 {
		oldestAvailable := history.events[0].ID - 1
		if cursor < oldestAvailable {
			return nil, fmt.Errorf(
				"%w: cursor=%d oldest_available=%d",
				ErrCursorExpired,
				cursor,
				oldestAvailable,
			)
		}
	}

	start := 0
	for start < len(history.events) && history.events[start].ID <= cursor {
		start++
	}

	out := make([]StreamEvent, len(history.events)-start)
	for i := start; i < len(history.events); i++ {
		out[i-start] = cloneStreamEvent(history.events[i])
	}
	return out, nil
}

func (b *Broker) runLocked(runID agent.RunID) *runHistory {
	history, ok := b.runs[runID]
	if ok {
		return history
	}
	history = &runHistory{
		nextID: 1,
		events: make([]StreamEvent, 0, b.historyLimit),
	}
	b.runs[runID] = history
	return history
}

func cloneStreamEvent(in StreamEvent) StreamEvent {
	return StreamEvent{
		ID:    in.ID,
		Event: cloneEvent(in.Event),
	}
}

func cloneEvent(in agent.Event) agent.Event {
	out := in
	if in.Message != nil {
		message := agent.CloneMessage(*in.Message)
		out.Message = &message
	}
	if in.ToolResult != nil {
		result := *in.ToolResult
		out.ToolResult = &result
	}
	return out
}
