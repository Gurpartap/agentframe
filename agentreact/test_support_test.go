package agentreact_test

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"

	"agentruntime/agent"
	"agentruntime/agentreact"
)

type response struct {
	Message agent.Message
	Err     error
}

type scriptedModel struct {
	mu        sync.Mutex
	index     int
	responses []response
}

func newScriptedModel(responses ...response) *scriptedModel {
	cloned := make([]response, len(responses))
	copy(cloned, responses)
	return &scriptedModel{responses: cloned}
}

var _ agentreact.Model = (*scriptedModel)(nil)

func (m *scriptedModel) Generate(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
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

type handler func(ctx context.Context, arguments map[string]any) (string, error)

type registry struct {
	mu       sync.RWMutex
	handlers map[string]handler
}

func newRegistry(initial map[string]handler) *registry {
	handlers := make(map[string]handler, len(initial))
	maps.Copy(handlers, initial)
	return &registry{handlers: handlers}
}

func (r *registry) Register(name string, h handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = h
}

func (r *registry) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	r.mu.RLock()
	h, ok := r.handlers[call.Name]
	r.mu.RUnlock()
	if !ok {
		return agent.ToolResult{}, fmt.Errorf("tool %q is not registered", call.Name)
	}
	content, err := h(ctx, call.Arguments)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: content,
	}, nil
}

type runStore struct {
	mu    sync.RWMutex
	state map[agent.RunID]agent.RunState
}

func newRunStore() *runStore {
	return &runStore{state: map[agent.RunID]agent.RunState{}}
}

func (s *runStore) Save(_ context.Context, runState agent.RunState) error {
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

func (s *runStore) Load(_ context.Context, runID agent.RunID) (agent.RunState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runState, ok := s.state[runID]
	if !ok {
		return agent.RunState{}, agent.ErrRunNotFound
	}
	return agent.CloneRunState(runState), nil
}

type eventSink struct {
	mu     sync.RWMutex
	events []agent.Event
}

func newEventSink() *eventSink {
	return &eventSink{events: make([]agent.Event, 0)}
}

func (s *eventSink) Publish(_ context.Context, event agent.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneEvent(event))
	return nil
}

func (s *eventSink) Events() []agent.Event {
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

type counterIDGenerator struct {
	prefix  string
	counter atomic.Uint64
}

func newCounterIDGenerator(prefix string) *counterIDGenerator {
	if prefix == "" {
		prefix = "run"
	}
	return &counterIDGenerator{prefix: prefix}
}

func (g *counterIDGenerator) NewRunID(_ context.Context) (agent.RunID, error) {
	next := g.counter.Add(1)
	return agent.RunID(fmt.Sprintf("%s-%06d", g.prefix, next)), nil
}
