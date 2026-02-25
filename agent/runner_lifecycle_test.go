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
			if len(gotEvents) != 1 {
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

func newLifecycleRunner(t *testing.T, store *testkit.RunStore, events *testkit.EventSink) *agent.Runner {
	t.Helper()

	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "ok",
			},
		},
	)
	registry := testkit.NewRegistry(map[string]testkit.Handler{})
	loop, err := agent.NewReactLoop(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: testkit.NewCounterIDGenerator("life"),
		RunStore:    store,
		ReactLoop:   loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner
}
