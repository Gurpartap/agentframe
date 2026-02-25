package inmem

import (
	"context"
	"sync"

	"agentruntime/agent"
)

// EventSink stores emitted events for tests and debugging.
type EventSink struct {
	mu     sync.RWMutex
	events []agent.Event
}

func NewEventSink() *EventSink {
	return &EventSink{
		events: make([]agent.Event, 0),
	}
}

func (s *EventSink) Publish(_ context.Context, event agent.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneEvent(event))
	return nil
}

func (s *EventSink) Events() []agent.Event {
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
		msg := agent.CloneMessage(*in.Message)
		out.Message = &msg
	}
	if in.ToolResult != nil {
		result := *in.ToolResult
		out.ToolResult = &result
	}
	return out
}
