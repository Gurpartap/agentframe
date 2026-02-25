package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"agentruntime/agent"
	eventinginmem "agentruntime/eventing/inmem"
	runstoreinmem "agentruntime/runstore/inmem"
)

func TestRunnerRun_PersistsAndCompletesWithEngine(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	var gotState agent.RunState
	var gotInput agent.EngineInput
	engine := &engineSpy{
		executeFn: func(_ context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
			gotState = agent.CloneRunState(state)
			gotInput = agent.EngineInput{MaxSteps: input.MaxSteps, Tools: append([]agent.ToolDefinition(nil), input.Tools...)}

			next := state
			next.Step = 1
			next.Status = agent.RunStatusCompleted
			next.Output = "done"
			return next, nil
		},
	}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	result, err := runner.Run(context.Background(), agent.RunInput{
		SystemPrompt: "system",
		UserPrompt:   "hello",
		MaxSteps:     3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup", Description: "Lookup information"},
		},
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if engine.calls != 1 {
		t.Fatalf("unexpected engine call count: %d", engine.calls)
	}
	if gotState.Status != agent.RunStatusPending {
		t.Fatalf("engine received unexpected status: %s", gotState.Status)
	}
	if gotState.Version != 1 {
		t.Fatalf("engine received unexpected version: %d", gotState.Version)
	}
	if len(gotState.Messages) != 2 {
		t.Fatalf("engine received unexpected message count: %d", len(gotState.Messages))
	}
	if gotState.Messages[0].Role != agent.RoleSystem || gotState.Messages[0].Content != "system" {
		t.Fatalf("unexpected first message: %+v", gotState.Messages[0])
	}
	if gotState.Messages[1].Role != agent.RoleUser || gotState.Messages[1].Content != "hello" {
		t.Fatalf("unexpected second message: %+v", gotState.Messages[1])
	}
	if gotInput.MaxSteps != 3 {
		t.Fatalf("engine received unexpected max steps: %d", gotInput.MaxSteps)
	}
	if len(gotInput.Tools) != 1 || gotInput.Tools[0].Name != "lookup" {
		t.Fatalf("engine received unexpected tools: %+v", gotInput.Tools)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Output != "done" {
		t.Fatalf("unexpected output: %q", result.State.Output)
	}
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}

	loaded, err := store.Load(context.Background(), result.State.ID)
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	if !reflect.DeepEqual(loaded, result.State) {
		t.Fatalf("saved state mismatch")
	}

	gotEvents := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	if len(gotEvents) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(gotEvents), len(wantTypes))
	}
	for i := range wantTypes {
		if gotEvents[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, gotEvents[i].Type, wantTypes[i])
		}
	}
	if gotEvents[2].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[2].CommandKind, agent.CommandKindStart)
	}
}

func TestRunnerRun_PropagatesEngineError(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	engine := &engineSpy{
		executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
			next := state
			next.Step = 1
			next.Status = agent.RunStatusMaxStepsExceeded
			next.Error = agent.ErrMaxStepsExceeded.Error()
			return next, agent.ErrMaxStepsExceeded
		},
	}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "do work",
		MaxSteps:   1,
	})
	if !errors.Is(err, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got: %v", err)
	}
	if result.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
	if result.State.Error != agent.ErrMaxStepsExceeded.Error() {
		t.Fatalf("unexpected error text: %q", result.State.Error)
	}

	loaded, loadErr := store.Load(context.Background(), result.State.ID)
	if loadErr != nil {
		t.Fatalf("load saved state: %v", loadErr)
	}
	if !reflect.DeepEqual(loaded, result.State) {
		t.Fatalf("saved state mismatch")
	}

	gotEvents := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	if len(gotEvents) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(gotEvents), len(wantTypes))
	}
	for i := range wantTypes {
		if gotEvents[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, gotEvents[i].Type, wantTypes[i])
		}
	}
	if gotEvents[2].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[2].CommandKind, agent.CommandKindStart)
	}
}

func TestRunnerRun_RejectsEmptyGeneratedRunID(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	engine := &engineSpy{}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: emptyIDGenerator{},
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
	if !errors.Is(runErr, agent.ErrInvalidRunID) {
		t.Fatalf("expected ErrInvalidRunID, got: %v", runErr)
	}
	if !reflect.DeepEqual(result, agent.RunResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if engine.calls != 0 {
		t.Fatalf("engine should not execute on generated run id validation failure, calls=%d", engine.calls)
	}
	if gotEvents := events.Events(); len(gotEvents) != 0 {
		t.Fatalf("unexpected events emitted: %d", len(gotEvents))
	}
	if _, loadErr := store.Load(context.Background(), ""); !errors.Is(loadErr, agent.ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound for empty run id persistence check, got %v", loadErr)
	}
}

type emptyIDGenerator struct{}

func (emptyIDGenerator) NewRunID(context.Context) (agent.RunID, error) {
	return "", nil
}
