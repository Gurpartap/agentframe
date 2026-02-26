package agentreact_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

func TestReactEngine_EventPublishFailureDoesNotChangeCompletion(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "final answer",
		},
	})
	registry := newRegistry(map[string]handler{})
	events := newFailingEventSink(2, errors.New("sink unavailable"))
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	store := newRunStore()
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("publish-fail"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "hello",
		MaxSteps:   2,
	})
	if !errors.Is(runErr, agent.ErrEventPublish) {
		t.Fatalf("expected ErrEventPublish, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
	if events.Calls() != 5 {
		t.Fatalf("unexpected publish calls: got=%d want=5", events.Calls())
	}

	loaded, err := store.Load(context.Background(), result.State.ID)
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if !reflect.DeepEqual(loaded, result.State) {
		t.Fatalf("persisted state mismatch")
	}
}

func TestReactEngine_CombinesModelAndEventPublishErrors(t *testing.T) {
	t.Parallel()

	modelErr := errors.New("model unavailable")
	model := newScriptedModel(response{
		Err: modelErr,
	})
	registry := newRegistry(map[string]handler{})
	events := newFailingEventSink(2, errors.New("sink unavailable"))
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	store := newRunStore()
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("publish-fail"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "hello",
		MaxSteps:   2,
	})
	if !errors.Is(runErr, agent.ErrEventPublish) {
		t.Fatalf("expected ErrEventPublish, got %v", runErr)
	}
	if !errors.Is(runErr, modelErr) {
		t.Fatalf("expected model error, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
}

type failingEventSink struct {
	mu     sync.Mutex
	calls  int
	failAt int
	err    error
}

func newFailingEventSink(failAt int, err error) *failingEventSink {
	return &failingEventSink{
		failAt: failAt,
		err:    err,
	}
}

func (s *failingEventSink) Publish(_ context.Context, _ agent.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls == s.failAt {
		return s.err
	}
	return nil
}

func (s *failingEventSink) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
