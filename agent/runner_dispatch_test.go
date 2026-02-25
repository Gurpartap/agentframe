package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"agentruntime/agent"
	"agentruntime/agent/internal/testkit"
)

func TestRunnerDispatch_StartWrapperParity(t *testing.T) {
	t.Parallel()

	input := agent.RunInput{
		RunID:        "dispatch-start-run",
		SystemPrompt: "Be concise.",
		UserPrompt:   "hello",
		MaxSteps:     3,
	}
	wrapperRunner := newDispatchRunner(t, testkit.NewRunStore(), testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
	})
	dispatchRunner := newDispatchRunner(t, testkit.NewRunStore(), testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
	})

	wrapperResult, wrapperErr := wrapperRunner.Run(context.Background(), input)
	if wrapperErr != nil {
		t.Fatalf("run wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.StartCommand{Input: input})
	if dispatchErr != nil {
		t.Fatalf("dispatch start error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("start wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if dispatchResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected dispatch status: %s", dispatchResult.State.Status)
	}
	if dispatchResult.State.Version != 2 {
		t.Fatalf("unexpected dispatch version: %d", dispatchResult.State.Version)
	}
}

func TestRunnerDispatch_ContinueWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-continue-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "continue"},
		},
	}

	wrapperStore := testkit.NewRunStore()
	if err := wrapperStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed wrapper store: %v", err)
	}
	dispatchStore := testkit.NewRunStore()
	if err := dispatchStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed dispatch store: %v", err)
	}

	wrapperRunner := newDispatchRunner(t, wrapperStore, testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "continued"},
	})
	dispatchRunner := newDispatchRunner(t, dispatchStore, testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "continued"},
	})

	wrapperResult, wrapperErr := wrapperRunner.Continue(context.Background(), runID, 3, nil)
	if wrapperErr != nil {
		t.Fatalf("continue wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    runID,
		MaxSteps: 3,
	})
	if dispatchErr != nil {
		t.Fatalf("dispatch continue error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("continue wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if dispatchResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected dispatch status: %s", dispatchResult.State.Status)
	}
	if dispatchResult.State.Step != 1 {
		t.Fatalf("unexpected dispatch step: %d", dispatchResult.State.Step)
	}
	if dispatchResult.State.Version != 2 {
		t.Fatalf("unexpected dispatch version: %d", dispatchResult.State.Version)
	}
}

func TestRunnerDispatch_CancelWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-cancel-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusRunning,
		Step:   7,
	}

	wrapperStore := testkit.NewRunStore()
	if err := wrapperStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed wrapper store: %v", err)
	}
	dispatchStore := testkit.NewRunStore()
	if err := dispatchStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed dispatch store: %v", err)
	}

	wrapperRunner := newDispatchRunner(t, wrapperStore, testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "unused"},
	})
	dispatchRunner := newDispatchRunner(t, dispatchStore, testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "unused"},
	})

	wrapperResult, wrapperErr := wrapperRunner.Cancel(context.Background(), runID)
	if wrapperErr != nil {
		t.Fatalf("cancel wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.CancelCommand{RunID: runID})
	if dispatchErr != nil {
		t.Fatalf("dispatch cancel error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("cancel wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if dispatchResult.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected dispatch status: %s", dispatchResult.State.Status)
	}
	if dispatchResult.State.Step != initial.Step {
		t.Fatalf("unexpected dispatch step: got=%d want=%d", dispatchResult.State.Step, initial.Step)
	}
	if dispatchResult.State.Version != 2 {
		t.Fatalf("unexpected dispatch version: %d", dispatchResult.State.Version)
	}
}

func TestRunnerDispatch_RejectsNilUnknownAndInvalidCommands(t *testing.T) {
	t.Parallel()

	runner := newDispatchRunner(t, testkit.NewRunStore(), testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
	})

	var nilCommand agent.Command
	if _, err := runner.Dispatch(context.Background(), nilCommand); !errors.Is(err, agent.ErrCommandNil) {
		t.Fatalf("expected ErrCommandNil for nil command, got %v", err)
	}

	var nilStart *agent.StartCommand
	if _, err := runner.Dispatch(context.Background(), nilStart); !errors.Is(err, agent.ErrCommandNil) {
		t.Fatalf("expected ErrCommandNil for nil start command, got %v", err)
	}

	if _, err := runner.Dispatch(context.Background(), unknownCommand{}); !errors.Is(err, agent.ErrCommandUnsupported) {
		t.Fatalf("expected ErrCommandUnsupported, got %v", err)
	}

	if _, err := runner.Dispatch(context.Background(), invalidStartCommand{}); !errors.Is(err, agent.ErrCommandInvalid) {
		t.Fatalf("expected ErrCommandInvalid, got %v", err)
	}
}

type unknownCommand struct{}

func (unknownCommand) Kind() agent.CommandKind {
	return agent.CommandKind("unknown")
}

type invalidStartCommand struct{}

func (invalidStartCommand) Kind() agent.CommandKind {
	return agent.CommandKindStart
}

func newDispatchRunner(
	t *testing.T,
	store *testkit.RunStore,
	events *testkit.EventSink,
	responses ...testkit.Response,
) *agent.Runner {
	t.Helper()

	model := testkit.NewScriptedModel(responses...)
	registry := testkit.NewRegistry(map[string]testkit.Handler{})
	loop, err := agent.NewReactLoop(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: testkit.NewCounterIDGenerator("dispatch"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner
}
