package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
)

func TestRunnerCancel_NonTerminalStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status agent.RunStatus
	}{
		{name: "pending", status: agent.RunStatusPending},
		{name: "running", status: agent.RunStatusRunning},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := runstoreinmem.New()
			events := eventinginmem.New()
			runner := newLifecycleRunner(t, store, events)

			initial := agent.RunState{
				ID:     agent.RunID("run-cancel-" + tc.name),
				Step:   3,
				Status: tc.status,
				Messages: []agent.Message{
					{Role: agent.RoleUser, Content: "hello"},
				},
			}
			if err := store.Save(context.Background(), initial); err != nil {
				t.Fatalf("save initial state: %v", err)
			}
			persistedInitial, err := store.Load(context.Background(), initial.ID)
			if err != nil {
				t.Fatalf("load initial state: %v", err)
			}

			result, err := runner.Cancel(context.Background(), initial.ID)
			if err != nil {
				t.Fatalf("cancel returned error: %v", err)
			}
			if result.State.Status != agent.RunStatusCancelled {
				t.Fatalf("unexpected status: %s", result.State.Status)
			}
			if result.State.Step != initial.Step {
				t.Fatalf("unexpected step: got=%d want=%d", result.State.Step, initial.Step)
			}
			if result.State.Version != persistedInitial.Version+1 {
				t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version+1)
			}
			if !reflect.DeepEqual(result.State.Messages, initial.Messages) {
				t.Fatalf("cancel changed transcript")
			}

			loaded, err := store.Load(context.Background(), initial.ID)
			if err != nil {
				t.Fatalf("load cancelled state: %v", err)
			}
			if !reflect.DeepEqual(loaded, result.State) {
				t.Fatalf("saved cancelled state mismatch")
			}

			gotEvents := events.Events()
			if len(gotEvents) != 2 {
				t.Fatalf("unexpected event count: %d", len(gotEvents))
			}
			if gotEvents[0].Type != agent.EventTypeRunCancelled {
				t.Fatalf("unexpected event type: %s", gotEvents[0].Type)
			}
			if gotEvents[0].RunID != initial.ID {
				t.Fatalf("unexpected event run id: %s", gotEvents[0].RunID)
			}
			if gotEvents[0].Step != initial.Step {
				t.Fatalf("unexpected event step: %d", gotEvents[0].Step)
			}
			if gotEvents[1].Type != agent.EventTypeCommandApplied {
				t.Fatalf("unexpected second event type: %s", gotEvents[1].Type)
			}
			if gotEvents[1].CommandKind != agent.CommandKindCancel {
				t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[1].CommandKind, agent.CommandKindCancel)
			}
		})
	}
}

func TestRunnerCancel_TerminalStatesRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status agent.RunStatus
	}{
		{name: "completed", status: agent.RunStatusCompleted},
		{name: "failed", status: agent.RunStatusFailed},
		{name: "cancelled", status: agent.RunStatusCancelled},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := runstoreinmem.New()
			events := eventinginmem.New()
			runner := newLifecycleRunner(t, store, events)

			initial := agent.RunState{
				ID:     agent.RunID("run-terminal-cancel-" + tc.name),
				Step:   2,
				Status: tc.status,
			}
			if err := store.Save(context.Background(), initial); err != nil {
				t.Fatalf("save initial state: %v", err)
			}
			persistedInitial, err := store.Load(context.Background(), initial.ID)
			if err != nil {
				t.Fatalf("load initial state: %v", err)
			}

			result, err := runner.Cancel(context.Background(), initial.ID)
			if !errors.Is(err, agent.ErrRunNotCancellable) {
				t.Fatalf("expected ErrRunNotCancellable, got %v", err)
			}
			if result.State.Status != tc.status {
				t.Fatalf("unexpected status: got=%s want=%s", result.State.Status, tc.status)
			}
			if result.State.Version != persistedInitial.Version {
				t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version)
			}

			loaded, err := store.Load(context.Background(), initial.ID)
			if err != nil {
				t.Fatalf("load state: %v", err)
			}
			if loaded.ID != persistedInitial.ID || loaded.Status != persistedInitial.Status || loaded.Step != persistedInitial.Step || loaded.Version != persistedInitial.Version {
				t.Fatalf("terminal state changed after cancel: got=%+v want=%+v", loaded, persistedInitial)
			}

			if gotEvents := events.Events(); len(gotEvents) != 0 {
				t.Fatalf("unexpected cancellation events for terminal state: %d", len(gotEvents))
			}
		})
	}
}

func TestRunnerContinue_TerminalStatesRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status agent.RunStatus
	}{
		{name: "completed", status: agent.RunStatusCompleted},
		{name: "failed", status: agent.RunStatusFailed},
		{name: "cancelled", status: agent.RunStatusCancelled},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := runstoreinmem.New()
			events := eventinginmem.New()
			runner := newLifecycleRunner(t, store, events)

			initial := agent.RunState{
				ID:     agent.RunID("run-terminal-continue-" + tc.name),
				Step:   4,
				Status: tc.status,
			}
			if err := store.Save(context.Background(), initial); err != nil {
				t.Fatalf("save initial state: %v", err)
			}
			persistedInitial, err := store.Load(context.Background(), initial.ID)
			if err != nil {
				t.Fatalf("load initial state: %v", err)
			}

			result, err := runner.Continue(context.Background(), initial.ID, 3, nil, nil)
			if !errors.Is(err, agent.ErrRunNotContinuable) {
				t.Fatalf("expected ErrRunNotContinuable, got %v", err)
			}
			if result.State.Status != tc.status {
				t.Fatalf("unexpected status: got=%s want=%s", result.State.Status, tc.status)
			}
			if result.State.Version != persistedInitial.Version {
				t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version)
			}

			if gotEvents := events.Events(); len(gotEvents) != 0 {
				t.Fatalf("unexpected events when continue is rejected: %d", len(gotEvents))
			}
		})
	}
}

func TestRunnerRun_PreCancelledContext(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	runner := newLifecycleRunner(t, store, events)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := runner.Run(ctx, agent.RunInput{
		UserPrompt: "hello",
		MaxSteps:   3,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if result.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Error != context.Canceled.Error() {
		t.Fatalf("unexpected error text: %q", result.State.Error)
	}
	if result.State.Step != 0 {
		t.Fatalf("unexpected step: %d", result.State.Step)
	}
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
	if len(result.State.Messages) != 1 || result.State.Messages[0].Role != agent.RoleUser {
		t.Fatalf("unexpected transcript: %+v", result.State.Messages)
	}

	gotEvents := events.Events()
	if len(gotEvents) != 4 {
		t.Fatalf("unexpected event count: %d", len(gotEvents))
	}
	if gotEvents[0].Type != agent.EventTypeRunStarted {
		t.Fatalf("unexpected first event type: %s", gotEvents[0].Type)
	}
	if gotEvents[1].Type != agent.EventTypeRunCancelled {
		t.Fatalf("unexpected second event type: %s", gotEvents[1].Type)
	}
	if gotEvents[2].Type != agent.EventTypeRunCheckpoint {
		t.Fatalf("unexpected third event type: %s", gotEvents[2].Type)
	}
	if gotEvents[3].Type != agent.EventTypeCommandApplied {
		t.Fatalf("unexpected fourth event type: %s", gotEvents[3].Type)
	}
	if gotEvents[3].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[3].CommandKind, agent.CommandKindStart)
	}
	if gotEvents[1].Description != context.Canceled.Error() {
		t.Fatalf("unexpected cancelled event description: %q", gotEvents[1].Description)
	}
}

func TestRunnerContinue_CancelledContext(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	runner := newLifecycleRunner(t, store, events)

	initial := agent.RunState{
		ID:     agent.RunID("run-continue-cancelled-context"),
		Status: agent.RunStatusPending,
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load initial state: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := runner.Continue(ctx, initial.ID, 3, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if result.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Error != context.Canceled.Error() {
		t.Fatalf("unexpected error text: %q", result.State.Error)
	}
	if result.State.Step != persistedInitial.Step {
		t.Fatalf("unexpected step: got=%d want=%d", result.State.Step, persistedInitial.Step)
	}
	if result.State.Version != persistedInitial.Version+1 {
		t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version+1)
	}

	loaded, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load continued state: %v", err)
	}
	if !reflect.DeepEqual(loaded, result.State) {
		t.Fatalf("saved state mismatch: got=%+v want=%+v", loaded, result.State)
	}

	gotEvents := events.Events()
	if len(gotEvents) != 3 {
		t.Fatalf("unexpected event count: %d", len(gotEvents))
	}
	if gotEvents[0].Type != agent.EventTypeRunCancelled {
		t.Fatalf("unexpected first event type: %s", gotEvents[0].Type)
	}
	if gotEvents[1].Type != agent.EventTypeRunCheckpoint {
		t.Fatalf("unexpected second event type: %s", gotEvents[1].Type)
	}
	if gotEvents[2].Type != agent.EventTypeCommandApplied {
		t.Fatalf("unexpected third event type: %s", gotEvents[2].Type)
	}
	if gotEvents[2].CommandKind != agent.CommandKindContinue {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[2].CommandKind, agent.CommandKindContinue)
	}
}

func TestRunnerFollowUp_CancelledContext(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	runner := newLifecycleRunner(t, store, events)

	initial := agent.RunState{
		ID:     agent.RunID("run-follow-up-cancelled-context"),
		Status: agent.RunStatusPending,
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load initial state: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := runner.FollowUp(ctx, initial.ID, "follow-up prompt", 3, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if result.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Error != context.Canceled.Error() {
		t.Fatalf("unexpected error text: %q", result.State.Error)
	}
	if result.State.Step != persistedInitial.Step {
		t.Fatalf("unexpected step: got=%d want=%d", result.State.Step, persistedInitial.Step)
	}
	if result.State.Version != persistedInitial.Version+1 {
		t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version+1)
	}

	loaded, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load follow-up state: %v", err)
	}
	if !reflect.DeepEqual(loaded, result.State) {
		t.Fatalf("saved state mismatch: got=%+v want=%+v", loaded, result.State)
	}

	gotEvents := events.Events()
	assertEventTypes(t, gotEvents, []agent.EventType{
		agent.EventTypeRunCancelled,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertCommandKind(t, gotEvents, agent.CommandKindFollowUp)
}

func TestRunnerContinue_SuspendedRunRequiresResolution(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	engine := &engineSpy{}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	initial := agent.RunState{
		ID:     agent.RunID("run-suspended-missing-resolution"),
		Status: agent.RunStatusSuspended,
		Step:   2,
		PendingRequirement: &agent.PendingRequirement{
			ID:     "req-1",
			Kind:   agent.RequirementKindApproval,
			Origin: agent.RequirementOriginModel,
		},
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load initial state: %v", err)
	}

	result, err := runner.Continue(context.Background(), initial.ID, 3, nil, nil)
	if !errors.Is(err, agent.ErrResolutionRequired) {
		t.Fatalf("expected ErrResolutionRequired, got %v", err)
	}
	if !reflect.DeepEqual(result.State, persistedInitial) {
		t.Fatalf("result state mismatch: got=%+v want=%+v", result.State, persistedInitial)
	}
	if engine.calls != 0 {
		t.Fatalf("engine should not execute on resolution-required rejection, calls=%d", engine.calls)
	}
	if gotEvents := events.Events(); len(gotEvents) != 0 {
		t.Fatalf("unexpected events on rejection: %d", len(gotEvents))
	}
}

func TestRunnerContinue_NonSuspendedRejectsResolution(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	engine := &engineSpy{}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	initial := agent.RunState{
		ID:     agent.RunID("run-non-suspended-unexpected-resolution"),
		Status: agent.RunStatusPending,
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load initial state: %v", err)
	}

	result, err := runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    initial.ID,
		MaxSteps: 3,
		Resolution: &agent.Resolution{
			RequirementID: "req-1",
			Kind:          agent.RequirementKindApproval,
			Outcome:       agent.ResolutionOutcomeApproved,
		},
	})
	if !errors.Is(err, agent.ErrResolutionUnexpected) {
		t.Fatalf("expected ErrResolutionUnexpected, got %v", err)
	}
	if !reflect.DeepEqual(result.State, persistedInitial) {
		t.Fatalf("result state mismatch: got=%+v want=%+v", result.State, persistedInitial)
	}
	if engine.calls != 0 {
		t.Fatalf("engine should not execute on unexpected-resolution rejection, calls=%d", engine.calls)
	}
	if gotEvents := events.Events(); len(gotEvents) != 0 {
		t.Fatalf("unexpected events on rejection: %d", len(gotEvents))
	}
}

func TestRunnerContinue_SuspendedRejectsInvalidResolution(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	engine := &engineSpy{}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	initial := agent.RunState{
		ID:     agent.RunID("run-suspended-invalid-resolution"),
		Status: agent.RunStatusSuspended,
		Step:   1,
		PendingRequirement: &agent.PendingRequirement{
			ID:     "req-1",
			Kind:   agent.RequirementKindApproval,
			Origin: agent.RequirementOriginModel,
		},
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load initial state: %v", err)
	}

	result, err := runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    initial.ID,
		MaxSteps: 3,
		Resolution: &agent.Resolution{
			RequirementID: "req-wrong",
			Kind:          agent.RequirementKindApproval,
			Outcome:       agent.ResolutionOutcomeApproved,
		},
	})
	if !errors.Is(err, agent.ErrResolutionInvalid) {
		t.Fatalf("expected ErrResolutionInvalid, got %v", err)
	}
	if !reflect.DeepEqual(result.State, persistedInitial) {
		t.Fatalf("result state mismatch: got=%+v want=%+v", result.State, persistedInitial)
	}
	if engine.calls != 0 {
		t.Fatalf("engine should not execute on invalid-resolution rejection, calls=%d", engine.calls)
	}
	if gotEvents := events.Events(); len(gotEvents) != 0 {
		t.Fatalf("unexpected events on rejection: %d", len(gotEvents))
	}
}

func TestRunnerSteerAndFollowUp_SuspendedRejected(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	engine := &engineSpy{}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	initial := agent.RunState{
		ID:     agent.RunID("run-suspended-steer-followup-rejected"),
		Status: agent.RunStatusSuspended,
		PendingRequirement: &agent.PendingRequirement{
			ID:     "req-1",
			Kind:   agent.RequirementKindUserInput,
			Origin: agent.RequirementOriginModel,
		},
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), initial.ID)
	if err != nil {
		t.Fatalf("load initial state: %v", err)
	}

	steerResult, steerErr := runner.Steer(context.Background(), initial.ID, "new direction")
	if !errors.Is(steerErr, agent.ErrResolutionRequired) {
		t.Fatalf("expected steer ErrResolutionRequired, got %v", steerErr)
	}
	if !reflect.DeepEqual(steerResult.State, persistedInitial) {
		t.Fatalf("steer result state mismatch: got=%+v want=%+v", steerResult.State, persistedInitial)
	}

	followUpResult, followUpErr := runner.FollowUp(context.Background(), initial.ID, "follow up", 3, nil)
	if !errors.Is(followUpErr, agent.ErrResolutionRequired) {
		t.Fatalf("expected follow-up ErrResolutionRequired, got %v", followUpErr)
	}
	if !reflect.DeepEqual(followUpResult.State, persistedInitial) {
		t.Fatalf("follow-up result state mismatch: got=%+v want=%+v", followUpResult.State, persistedInitial)
	}
	if engine.calls != 0 {
		t.Fatalf("engine should not execute for suspended steer/follow-up rejections, calls=%d", engine.calls)
	}
	if gotEvents := events.Events(); len(gotEvents) != 0 {
		t.Fatalf("unexpected events on rejection: %d", len(gotEvents))
	}
}

func TestRunnerRunContinue_SuspendedResolutionFlow(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	events := eventinginmem.New()
	engine := &engineSpy{}
	engine.executeFn = func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
		switch engine.calls {
		case 1:
			next := state
			if err := agent.TransitionRunStatus(&next, agent.RunStatusRunning); err != nil {
				t.Fatalf("transition to running: %v", err)
			}
			next.Step++
			requirement := &agent.PendingRequirement{
				ID:     "req-approval",
				Kind:   agent.RequirementKindApproval,
				Origin: agent.RequirementOriginModel,
				Prompt: "approve execution",
			}
			next.Messages = append(next.Messages, agent.Message{
				Role:        agent.RoleAssistant,
				Content:     "approval required",
				Requirement: requirement,
			})
			next.PendingRequirement = requirement
			if err := agent.TransitionRunStatus(&next, agent.RunStatusSuspended); err != nil {
				t.Fatalf("transition to suspended: %v", err)
			}
			return next, nil
		case 2:
			if state.Status != agent.RunStatusRunning {
				t.Fatalf("continue should execute from running status, got %s", state.Status)
			}
			if state.PendingRequirement != nil {
				t.Fatalf("continue should clear pending requirement before execution")
			}
			next := state
			next.Step++
			message := agent.Message{
				Role:    agent.RoleAssistant,
				Content: "approved and completed",
			}
			next.Messages = append(next.Messages, message)
			if err := agent.TransitionRunStatus(&next, agent.RunStatusCompleted); err != nil {
				t.Fatalf("transition to completed: %v", err)
			}
			next.Output = message.Content
			return next, nil
		default:
			t.Fatalf("unexpected engine call count: %d", engine.calls)
			return state, nil
		}
	}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	runResult, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "run-suspend-continue-flow",
		UserPrompt: "start",
		MaxSteps:   3,
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if runResult.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected run status: %s", runResult.State.Status)
	}
	if runResult.State.PendingRequirement == nil {
		t.Fatalf("expected pending requirement on suspended run")
	}

	prefix := agent.CloneMessages(runResult.State.Messages)
	continueResult, continueErr := runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    runResult.State.ID,
		MaxSteps: 3,
		Resolution: &agent.Resolution{
			RequirementID: "req-approval",
			Kind:          agent.RequirementKindApproval,
			Outcome:       agent.ResolutionOutcomeApproved,
		},
	})
	if continueErr != nil {
		t.Fatalf("continue returned error: %v", continueErr)
	}
	if continueResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected continue status: %s", continueResult.State.Status)
	}
	if continueResult.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared after successful continue")
	}
	if continueResult.State.Version != runResult.State.Version+1 {
		t.Fatalf("unexpected version after continue: got=%d want=%d", continueResult.State.Version, runResult.State.Version+1)
	}
	if len(continueResult.State.Messages) <= len(prefix) {
		t.Fatalf("expected transcript growth after continue")
	}
	if !reflect.DeepEqual(continueResult.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("continue mutated transcript prefix")
	}

	loaded, err := store.Load(context.Background(), runResult.State.ID)
	if err != nil {
		t.Fatalf("load final state: %v", err)
	}
	if !reflect.DeepEqual(loaded, continueResult.State) {
		t.Fatalf("saved state mismatch: got=%+v want=%+v", loaded, continueResult.State)
	}

	gotEvents := events.Events()
	assertEventTypes(t, gotEvents, []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeRunSuspended,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	if gotEvents[3].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected start command kind: %s", gotEvents[3].CommandKind)
	}
	if gotEvents[5].CommandKind != agent.CommandKindContinue {
		t.Fatalf("unexpected continue command kind: %s", gotEvents[5].CommandKind)
	}
}

func newLifecycleRunner(t *testing.T, store *runstoreinmem.Store, events *eventinginmem.Sink) *agent.Runner {
	t.Helper()

	return newDispatchRunner(
		t,
		store,
		events,
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "ok",
			},
		},
	)
}
