package testkit

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"

	"agentruntime/agent"
)

// Response configures one model turn in a scripted sequence.
type Response struct {
	Message agent.Message
	Err     error
}

// ScriptedModel is a deterministic model adapter for runtime tests.
type ScriptedModel struct {
	mu        sync.Mutex
	index     int
	responses []Response
}

func NewScriptedModel(responses ...Response) *ScriptedModel {
	cloned := make([]Response, len(responses))
	copy(cloned, responses)
	return &ScriptedModel{
		responses: cloned,
	}
}

var _ agent.Model = (*ScriptedModel)(nil)

func (m *ScriptedModel) Generate(_ context.Context, _ agent.ModelRequest) (agent.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.index >= len(m.responses) {
		return agent.Message{}, fmt.Errorf("script exhausted at step %d", m.index+1)
	}
	current := m.responses[m.index]
	m.index++
	if current.Err != nil {
		return agent.Message{}, current.Err
	}
	msg := agent.CloneMessage(current.Message)
	if msg.Role == "" {
		msg.Role = agent.RoleAssistant
	}
	return msg, nil
}

// Handler executes business logic for one tool call.
type Handler func(ctx context.Context, arguments map[string]any) (string, error)

// Registry is a minimal map-backed tool executor.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry(initial map[string]Handler) *Registry {
	handlers := make(map[string]Handler, len(initial))
	maps.Copy(handlers, initial)
	return &Registry{handlers: handlers}
}

func (r *Registry) Register(name string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

func (r *Registry) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	r.mu.RLock()
	handler, ok := r.handlers[call.Name]
	r.mu.RUnlock()
	if !ok {
		return agent.ToolResult{}, fmt.Errorf("tool %q is not registered", call.Name)
	}
	content, err := handler(ctx, call.Arguments)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: content,
	}, nil
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
	s.state[runState.ID] = agent.CloneRunState(runState)
	return nil
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
