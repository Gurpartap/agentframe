package agent_test

import (
	"context"
	"errors"
	"fmt"
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

func TestRunnerDispatch_SteerWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-steer-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   3,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
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

	wrapperEvents := testkit.NewEventSink()
	wrapperEngine := &engineSpy{}
	wrapperRunner := newDispatchRunnerWithEngine(t, wrapperStore, wrapperEvents, wrapperEngine)
	dispatchEvents := testkit.NewEventSink()
	dispatchEngine := &engineSpy{}
	dispatchRunner := newDispatchRunnerWithEngine(t, dispatchStore, dispatchEvents, dispatchEngine)

	wrapperResult, wrapperErr := wrapperRunner.Steer(context.Background(), runID, "steer now")
	if wrapperErr != nil {
		t.Fatalf("steer wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.SteerCommand{
		RunID:       runID,
		Instruction: "steer now",
	})
	if dispatchErr != nil {
		t.Fatalf("dispatch steer error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("steer wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if wrapperEngine.calls != 0 || dispatchEngine.calls != 0 {
		t.Fatalf("steer must not invoke engine: wrapper_calls=%d dispatch_calls=%d", wrapperEngine.calls, dispatchEngine.calls)
	}
	assertEventTypes(t, wrapperEvents.Events(), []agent.EventType{
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertEventTypes(t, dispatchEvents.Events(), []agent.EventType{
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertCommandKind(t, wrapperEvents.Events(), agent.CommandKindSteer)
	assertCommandKind(t, dispatchEvents.Events(), agent.CommandKindSteer)
}

func TestRunnerDispatch_FollowUpWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-followup-parity")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
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

	wrapperEvents := testkit.NewEventSink()
	wrapperRunner := newDispatchRunner(
		t,
		wrapperStore,
		wrapperEvents,
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "follow-up done",
			},
		},
	)
	dispatchEvents := testkit.NewEventSink()
	dispatchRunner := newDispatchRunner(
		t,
		dispatchStore,
		dispatchEvents,
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "follow-up done",
			},
		},
	)

	wrapperResult, wrapperErr := wrapperRunner.FollowUp(context.Background(), runID, "next prompt", 4, nil)
	if wrapperErr != nil {
		t.Fatalf("follow up wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.FollowUpCommand{
		RunID:      runID,
		UserPrompt: "next prompt",
		MaxSteps:   4,
	})
	if dispatchErr != nil {
		t.Fatalf("dispatch follow up error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("follow up wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	assertEventTypes(t, wrapperEvents.Events(), []agent.EventType{
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertEventTypes(t, dispatchEvents.Events(), []agent.EventType{
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertCommandKind(t, wrapperEvents.Events(), agent.CommandKindFollowUp)
	assertCommandKind(t, dispatchEvents.Events(), agent.CommandKindFollowUp)
}

func TestRunnerDispatch_RejectsNilUnknownAndInvalidCommands(t *testing.T) {
	t.Parallel()

	runner := newDispatchRunner(t, testkit.NewRunStore(), testkit.NewEventSink(), testkit.Response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
	})

	var nilCommand agent.Command
	var nilStart *agent.StartCommand
	var nilContinue *agent.ContinueCommand
	var nilCancel *agent.CancelCommand
	var nilSteer *agent.SteerCommand
	var nilFollowUp *agent.FollowUpCommand
	nilCases := []struct {
		name string
		cmd  agent.Command
	}{
		{name: "nil", cmd: nilCommand},
		{name: "nil_start", cmd: nilStart},
		{name: "nil_continue", cmd: nilContinue},
		{name: "nil_cancel", cmd: nilCancel},
		{name: "nil_steer", cmd: nilSteer},
		{name: "nil_follow_up", cmd: nilFollowUp},
	}
	for _, tc := range nilCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := runner.Dispatch(context.Background(), tc.cmd); !errors.Is(err, agent.ErrCommandNil) {
				t.Fatalf("expected ErrCommandNil, got %v", err)
			}
		})
	}

	if _, err := runner.Dispatch(context.Background(), unknownCommand{}); !errors.Is(err, agent.ErrCommandUnsupported) {
		t.Fatalf("expected ErrCommandUnsupported, got %v", err)
	}

	invalidCases := []struct {
		name string
		cmd  agent.Command
	}{
		{name: "invalid_start", cmd: invalidStartCommand{}},
		{name: "invalid_continue", cmd: invalidContinueCommand{}},
		{name: "invalid_cancel", cmd: invalidCancelCommand{}},
		{name: "invalid_steer", cmd: invalidSteerCommand{}},
		{name: "invalid_follow_up", cmd: invalidFollowUpCommand{}},
	}
	for _, tc := range invalidCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := runner.Dispatch(context.Background(), tc.cmd); !errors.Is(err, agent.ErrCommandInvalid) {
				t.Fatalf("expected ErrCommandInvalid, got %v", err)
			}
		})
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

func TestRunnerSteerThenFollowUp_TranscriptAppendOnly(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-steer-follow-up-append")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleSystem, Content: "system"},
			{Role: agent.RoleUser, Content: "original"},
		},
	}
	store := testkit.NewRunStore()
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	events := testkit.NewEventSink()
	runner := newDispatchRunner(
		t,
		store,
		events,
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "follow-up done",
			},
		},
	)

	steered, err := runner.Steer(context.Background(), runID, "steer instruction")
	if err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	prefix := agent.CloneMessages(steered.State.Messages)
	followed, err := runner.FollowUp(context.Background(), runID, "follow-up prompt", 3, nil)
	if err != nil {
		t.Fatalf("follow up returned error: %v", err)
	}
	if len(followed.State.Messages) <= len(prefix) {
		t.Fatalf("expected transcript growth after follow up")
	}
	if !reflect.DeepEqual(followed.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("follow up mutated transcript prefix")
	}
	if followed.State.Messages[len(prefix)].Role != agent.RoleUser || followed.State.Messages[len(prefix)].Content != "follow-up prompt" {
		t.Fatalf("unexpected follow-up appended message: %+v", followed.State.Messages[len(prefix)])
	}
}

func TestRunnerDispatch_TerminalStateImmutabilityForMutatingCommands(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-terminal-mutation-matrix")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusCompleted,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "frozen"},
		},
	}

	cases := []struct {
		name    string
		wantErr error
		call    func(context.Context, *agent.Runner) (agent.RunResult, error)
	}{
		{
			name:    "continue",
			wantErr: agent.ErrRunNotContinuable,
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.Continue(ctx, runID, 3, nil)
			},
		},
		{
			name:    "cancel",
			wantErr: agent.ErrRunNotCancellable,
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.Cancel(ctx, runID)
			},
		},
		{
			name:    "steer",
			wantErr: agent.ErrRunNotContinuable,
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.Steer(ctx, runID, "instruction")
			},
		},
		{
			name:    "follow_up",
			wantErr: agent.ErrRunNotContinuable,
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
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if !reflect.DeepEqual(result.State, persistedInitial) {
				t.Fatalf("result state mutated: got=%+v want=%+v", result.State, persistedInitial)
			}
			if engine.calls != 0 {
				t.Fatalf("engine should not execute on terminal rejection, calls=%d", engine.calls)
			}
			loaded, err := store.Load(context.Background(), runID)
			if err != nil {
				t.Fatalf("load state: %v", err)
			}
			if !reflect.DeepEqual(loaded, persistedInitial) {
				t.Fatalf("persisted state mutated: got=%+v want=%+v", loaded, persistedInitial)
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

type invalidContinueCommand struct{}

func (invalidContinueCommand) Kind() agent.CommandKind {
	return agent.CommandKindContinue
}

type invalidCancelCommand struct{}

func (invalidCancelCommand) Kind() agent.CommandKind {
	return agent.CommandKindCancel
}

type invalidSteerCommand struct{}

func (invalidSteerCommand) Kind() agent.CommandKind {
	return agent.CommandKindSteer
}

type invalidFollowUpCommand struct{}

func (invalidFollowUpCommand) Kind() agent.CommandKind {
	return agent.CommandKindFollowUp
}

func newDispatchRunner(
	t *testing.T,
	store *testkit.RunStore,
	events *testkit.EventSink,
	responses ...testkit.Response,
) *agent.Runner {
	t.Helper()

	engine := &scriptedEngine{
		events:    events,
		responses: append([]testkit.Response(nil), responses...),
	}
	return newDispatchRunnerWithEngine(t, store, events, engine)
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

func assertEventTypes(t *testing.T, events []agent.Event, want []agent.EventType) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(events), len(want))
	}
	for i := range want {
		if events[i].Type != want[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, events[i].Type, want[i])
		}
	}
}

func assertCommandKind(t *testing.T, events []agent.Event, want agent.CommandKind) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("no events emitted")
	}
	last := events[len(events)-1]
	if last.Type != agent.EventTypeCommandApplied {
		t.Fatalf("last event must be command_applied, got=%s", last.Type)
	}
	if last.CommandKind != want {
		t.Fatalf("unexpected command kind: got=%s want=%s", last.CommandKind, want)
	}
}

type scriptedEngine struct {
	index     int
	events    agent.EventSink
	responses []testkit.Response
}

func (s *scriptedEngine) Execute(ctx context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		next := state
		if transitionErr := agent.TransitionRunStatus(&next, agent.RunStatusCancelled); transitionErr != nil {
			return state, errors.Join(ctxErr, transitionErr)
		}
		next.Error = ctxErr.Error()
		_ = s.events.Publish(ctx, agent.Event{
			RunID:       next.ID,
			Step:        next.Step,
			Type:        agent.EventTypeRunCancelled,
			Description: ctxErr.Error(),
		})
		return next, ctxErr
	}

	if s.index >= len(s.responses) {
		return state, fmt.Errorf("script exhausted at step %d", s.index+1)
	}
	current := s.responses[s.index]
	s.index++
	if current.Err != nil {
		return state, current.Err
	}

	next := state
	if err := agent.TransitionRunStatus(&next, agent.RunStatusRunning); err != nil {
		return next, err
	}
	next.Step++

	message := agent.CloneMessage(current.Message)
	if message.Role == "" {
		message.Role = agent.RoleAssistant
	}
	next.Messages = append(next.Messages, message)
	_ = s.events.Publish(ctx, agent.Event{
		RunID:   next.ID,
		Step:    next.Step,
		Type:    agent.EventTypeAssistantMessage,
		Message: &message,
	})

	if len(message.ToolCalls) == 0 {
		if err := agent.TransitionRunStatus(&next, agent.RunStatusCompleted); err != nil {
			return next, err
		}
		next.Output = message.Content
		_ = s.events.Publish(ctx, agent.Event{
			RunID:       next.ID,
			Step:        next.Step,
			Type:        agent.EventTypeRunCompleted,
			Description: "assistant returned a final answer",
		})
	}

	return next, nil
}
