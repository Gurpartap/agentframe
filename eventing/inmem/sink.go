package inmem

import (
	"context"
	"sync"

	"agentruntime/agent"
)

// Sink captures runtime events in memory and exposes deterministic snapshots.
type Sink struct {
	mu     sync.RWMutex
	events []agent.Event
}

var _ agent.EventSink = (*Sink)(nil)

func New() *Sink {
	return &Sink{events: make([]agent.Event, 0)}
}

func (s *Sink) Publish(ctx context.Context, event agent.Event) error {
	if ctx == nil {
		return agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err := agent.ValidateEvent(event); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, cloneEvent(event))
	return nil
}

func (s *Sink) Events() []agent.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]agent.Event, len(s.events))
	for i := range s.events {
		out[i] = cloneEvent(s.events[i])
	}
	return out
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
