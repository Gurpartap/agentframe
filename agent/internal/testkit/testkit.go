package testkit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"agentruntime/agent"
)

// Response configures one model turn in a scripted sequence.
type Response struct {
	Message agent.Message
	Err     error
}

// RunStore is a simple in-memory implementation for local development and tests.
type RunStore struct {
	mu    sync.RWMutex
	state map[agent.RunID]agent.RunState
}

func NewRunStore() *RunStore {
	return &RunStore{
		state: map[agent.RunID]agent.RunState{},
	}
}

func (s *RunStore) Save(_ context.Context, runState agent.RunState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.state[runState.ID]
	switch {
	case !exists:
		if runState.Version != 0 {
			return fmt.Errorf(
				"%w: run %q expected version 0 on create, got %d",
				agent.ErrRunVersionConflict,
				runState.ID,
				runState.Version,
			)
		}
		next := agent.CloneRunState(runState)
		next.Version = 1
		s.state[runState.ID] = next
		return nil
	case runState.Version != current.Version:
		return fmt.Errorf(
			"%w: run %q expected version %d, got %d",
			agent.ErrRunVersionConflict,
			runState.ID,
			current.Version,
			runState.Version,
		)
	default:
		next := agent.CloneRunState(runState)
		next.Version = current.Version + 1
		s.state[runState.ID] = next
		return nil
	}
}

func (s *RunStore) Load(_ context.Context, runID agent.RunID) (agent.RunState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runState, ok := s.state[runID]
	if !ok {
		return agent.RunState{}, agent.ErrRunNotFound
	}
	return agent.CloneRunState(runState), nil
}

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

// CounterIDGenerator provides deterministic in-process run IDs.
type CounterIDGenerator struct {
	prefix  string
	counter atomic.Uint64
}

func NewCounterIDGenerator(prefix string) *CounterIDGenerator {
	if prefix == "" {
		prefix = "run"
	}
	return &CounterIDGenerator{
		prefix: prefix,
	}
}

func (g *CounterIDGenerator) NewRunID(_ context.Context) (agent.RunID, error) {
	next := g.counter.Add(1)
	return agent.RunID(fmt.Sprintf("%s-%06d", g.prefix, next)), nil
}
