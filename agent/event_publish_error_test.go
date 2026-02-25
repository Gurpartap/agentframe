package agent_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"agentruntime/agent"
	runstoreinmem "agentruntime/runstore/inmem"
)

func TestRunnerRun_EventPublishFailureKeepsPersistedState(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := newFailingEventSink(1, errors.New("sink unavailable"))
	engine := &engineSpy{
		executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
			next := state
			if err := agent.TransitionRunStatus(&next, agent.RunStatusRunning); err != nil {
				t.Fatalf("transition to running: %v", err)
			}
			next.Step = 1
			next.Messages = append(next.Messages, agent.Message{
				Role:    agent.RoleAssistant,
				Content: "done",
			})
			if err := agent.TransitionRunStatus(&next, agent.RunStatusCompleted); err != nil {
				t.Fatalf("transition to completed: %v", err)
			}
			next.Output = "done"
			return next, nil
		},
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("event-fail"),
		RunStore:    store,
		Engine:      engine,
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
	if events.Calls() != 3 {
		t.Fatalf("unexpected publish calls: got=%d want=3", events.Calls())
	}

	loaded, err := store.Load(context.Background(), result.State.ID)
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if !reflect.DeepEqual(loaded, result.State) {
		t.Fatalf("persisted state mismatch")
	}
}

func TestRunnerRun_CombinesEngineAndEventPublishErrors(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := newFailingEventSink(1, errors.New("sink unavailable"))
	engine := &engineSpy{
		executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
			next := state
			if err := agent.TransitionRunStatus(&next, agent.RunStatusRunning); err != nil {
				t.Fatalf("transition to running: %v", err)
			}
			next.Step = 1
			if err := agent.TransitionRunStatus(&next, agent.RunStatusMaxStepsExceeded); err != nil {
				t.Fatalf("transition to max steps: %v", err)
			}
			next.Error = agent.ErrMaxStepsExceeded.Error()
			return next, agent.ErrMaxStepsExceeded
		},
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("event-fail"),
		RunStore:    store,
		Engine:      engine,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "hello",
		MaxSteps:   1,
	})
	if !errors.Is(runErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", runErr)
	}
	if !errors.Is(runErr, agent.ErrEventPublish) {
		t.Fatalf("expected ErrEventPublish, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
}

func TestRunnerCancel_EventPublishFailureKeepsPersistedState(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	initial := agent.RunState{
		ID:     "cancel-event-fail",
		Status: agent.RunStatusRunning,
		Step:   2,
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	events := newFailingEventSink(1, errors.New("sink unavailable"))
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("event-fail"),
		RunStore:    store,
		Engine:      &engineSpy{},
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, cancelErr := runner.Cancel(context.Background(), initial.ID)
	if !errors.Is(cancelErr, agent.ErrEventPublish) {
		t.Fatalf("expected ErrEventPublish, got %v", cancelErr)
	}
	if result.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
	if events.Calls() != 2 {
		t.Fatalf("unexpected publish calls: got=%d want=2", events.Calls())
	}

	loaded, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load cancelled state: %v", err)
	}
	if !reflect.DeepEqual(loaded, result.State) {
		t.Fatalf("persisted state mismatch")
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
