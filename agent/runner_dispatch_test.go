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

func TestRunnerSteer_AppendsTranscriptWithoutEngineExecution(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-steer-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   4,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "original"},
		},
	}

	store := testkit.NewRunStore()
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load initial: %v", err)
	}

	events := testkit.NewEventSink()
	engine := &engineSpy{}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	result, err := runner.Steer(context.Background(), runID, "new direction")
	if err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	if engine.calls != 0 {
		t.Fatalf("engine should not execute for steer, calls=%d", engine.calls)
	}
	if result.State.Status != persistedInitial.Status {
		t.Fatalf("unexpected status: got=%s want=%s", result.State.Status, persistedInitial.Status)
	}
	if result.State.Step != persistedInitial.Step {
		t.Fatalf("unexpected step: got=%d want=%d", result.State.Step, persistedInitial.Step)
	}
	if result.State.Version != persistedInitial.Version+1 {
		t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version+1)
	}
	if len(result.State.Messages) != len(persistedInitial.Messages)+1 {
		t.Fatalf("unexpected message count: got=%d want=%d", len(result.State.Messages), len(persistedInitial.Messages)+1)
	}
	appended := result.State.Messages[len(result.State.Messages)-1]
	if appended.Role != agent.RoleUser || appended.Content != "new direction" {
		t.Fatalf("unexpected appended message: %+v", appended)
	}

	gotEvents := events.Events()
	if len(gotEvents) != 2 {
		t.Fatalf("unexpected event count: %d", len(gotEvents))
	}
	if gotEvents[0].Type != agent.EventTypeRunCheckpoint {
		t.Fatalf("unexpected first event type: %s", gotEvents[0].Type)
	}
	if gotEvents[1].Type != agent.EventTypeCommandApplied {
		t.Fatalf("unexpected second event type: %s", gotEvents[1].Type)
	}
	if gotEvents[1].CommandKind != agent.CommandKindSteer {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[1].CommandKind, agent.CommandKindSteer)
	}
}

func TestRunnerFollowUp_AppendsTranscriptAndInvokesEngine(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-follow-up-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "original"},
		},
	}

	store := testkit.NewRunStore()
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load initial: %v", err)
	}

	events := testkit.NewEventSink()
	engine := &engineSpy{
		executeFn: func(_ context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
			if len(state.Messages) != len(persistedInitial.Messages)+1 {
				t.Fatalf("unexpected message count received by engine: got=%d want=%d", len(state.Messages), len(persistedInitial.Messages)+1)
			}
			last := state.Messages[len(state.Messages)-1]
			if last.Role != agent.RoleUser || last.Content != "follow up prompt" {
				t.Fatalf("engine received unexpected appended message: %+v", last)
			}
			if input.MaxSteps != 5 {
				t.Fatalf("unexpected max steps: %d", input.MaxSteps)
			}
			if len(input.Tools) != 1 || input.Tools[0].Name != "lookup" {
				t.Fatalf("unexpected tools: %+v", input.Tools)
			}
			next := state
			next.Step++
			next.Status = agent.RunStatusCompleted
			next.Output = "done"
			return next, nil
		},
	}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	result, err := runner.FollowUp(context.Background(), runID, "follow up prompt", 5, []agent.ToolDefinition{{Name: "lookup"}})
	if err != nil {
		t.Fatalf("follow up returned error: %v", err)
	}
	if engine.calls != 1 {
		t.Fatalf("engine should execute exactly once for follow up, calls=%d", engine.calls)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Step != persistedInitial.Step+1 {
		t.Fatalf("unexpected step: got=%d want=%d", result.State.Step, persistedInitial.Step+1)
	}
	if result.State.Version != persistedInitial.Version+1 {
		t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version+1)
	}
	if len(result.State.Messages) != len(persistedInitial.Messages)+1 {
		t.Fatalf("unexpected message count: got=%d want=%d", len(result.State.Messages), len(persistedInitial.Messages)+1)
	}
	appended := result.State.Messages[len(result.State.Messages)-1]
	if appended.Role != agent.RoleUser || appended.Content != "follow up prompt" {
		t.Fatalf("unexpected appended message: %+v", appended)
	}

	gotEvents := events.Events()
	if len(gotEvents) != 2 {
		t.Fatalf("unexpected event count: %d", len(gotEvents))
	}
	if gotEvents[0].Type != agent.EventTypeRunCheckpoint {
		t.Fatalf("unexpected first event type: %s", gotEvents[0].Type)
	}
	if gotEvents[1].Type != agent.EventTypeCommandApplied {
		t.Fatalf("unexpected second event type: %s", gotEvents[1].Type)
	}
	if gotEvents[1].CommandKind != agent.CommandKindFollowUp {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[1].CommandKind, agent.CommandKindFollowUp)
	}
}

func TestRunnerSteerFollowUp_TerminalStateRejected(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-terminal-steer-follow-up")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusCompleted,
		Step:   2,
	}

	cases := []struct {
		name string
		call func(context.Context, *agent.Runner) (agent.RunResult, error)
	}{
		{
			name: "steer",
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.Steer(ctx, runID, "instruction")
			},
		},
		{
			name: "follow_up",
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.FollowUp(ctx, runID, "prompt", 3, nil)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewRunStore()
			if err := store.Save(context.Background(), initial); err != nil {
				t.Fatalf("seed store: %v", err)
			}
			persistedInitial, err := store.Load(context.Background(), runID)
			if err != nil {
				t.Fatalf("load initial: %v", err)
			}

			events := testkit.NewEventSink()
			engine := &engineSpy{}
			runner := newDispatchRunnerWithEngine(t, store, events, engine)

			result, err := tc.call(context.Background(), runner)
			if !errors.Is(err, agent.ErrRunNotContinuable) {
				t.Fatalf("expected ErrRunNotContinuable, got %v", err)
			}
			if result.State.Status != persistedInitial.Status {
				t.Fatalf("unexpected status: got=%s want=%s", result.State.Status, persistedInitial.Status)
			}
			if result.State.Version != persistedInitial.Version {
				t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version)
			}
			if engine.calls != 0 {
				t.Fatalf("engine should not execute on terminal rejection, calls=%d", engine.calls)
			}
			if gotEvents := events.Events(); len(gotEvents) != 0 {
				t.Fatalf("unexpected events emitted: %d", len(gotEvents))
			}
		})
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

type engineSpy struct {
	calls     int
	executeFn func(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error)
}

func (s *engineSpy) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	s.calls++
	if s.executeFn == nil {
		return state, nil
	}
	return s.executeFn(ctx, state, input)
}

func newDispatchRunnerWithEngine(
	t *testing.T,
	store *testkit.RunStore,
	events *testkit.EventSink,
	engine agent.Engine,
) *agent.Runner {
	t.Helper()

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: testkit.NewCounterIDGenerator("dispatch"),
		RunStore:    store,
		Engine:      engine,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner
}
