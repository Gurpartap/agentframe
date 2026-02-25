package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"agentruntime/agent"
	"agentruntime/agent/internal/testkit"
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

			store := testkit.NewRunStore()
			events := testkit.NewEventSink()
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

			store := testkit.NewRunStore()
			events := testkit.NewEventSink()
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

			store := testkit.NewRunStore()
			events := testkit.NewEventSink()
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

			result, err := runner.Continue(context.Background(), initial.ID, 3, nil)
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

	store := testkit.NewRunStore()
	events := testkit.NewEventSink()
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

	store := testkit.NewRunStore()
	events := testkit.NewEventSink()
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

	result, err := runner.Continue(ctx, initial.ID, 3, nil)
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

func newLifecycleRunner(t *testing.T, store *testkit.RunStore, events *testkit.EventSink) *agent.Runner {
	t.Helper()

	return newDispatchRunner(
		t,
		store,
		events,
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "ok",
			},
		},
	)
}
