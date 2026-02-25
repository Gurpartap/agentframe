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

func TestRunnerDispatch_EngineOutputContractViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "start_run_id_mismatch",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-start-id-mismatch")
				events := eventinginmem.New()
				store := runstoreinmem.New()
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.ID = "unexpected-run-id"
						next.Step++
						next.Status = agent.RunStatusCompleted
						next.Output = "corrupt"
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, err := runner.Run(context.Background(), agent.RunInput{
					RunID:        runID,
					SystemPrompt: "system",
					UserPrompt:   "start",
					MaxSteps:     2,
				})
				if !errors.Is(err, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", err)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
				}

				persisted, loadErr := store.Load(context.Background(), runID)
				if loadErr != nil {
					t.Fatalf("load persisted state: %v", loadErr)
				}
				want := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Messages: []agent.Message{
						{Role: agent.RoleSystem, Content: "system"},
						{Role: agent.RoleUser, Content: "start"},
					},
					Version: 1,
				}
				if !reflect.DeepEqual(persisted, want) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persisted, want)
				}

				gotEvents := events.Events()
				if countEventType(gotEvents, agent.EventTypeRunStarted) != 1 {
					t.Fatalf("expected exactly one run_started event, got=%d", countEventType(gotEvents, agent.EventTypeRunStarted))
				}
				assertNoCheckpointOrCommandAppliedEvents(t, gotEvents)
			},
		},
		{
			name: "continue_step_regression",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-continue-step-regression")
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Step:   4,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "continue"},
					},
				}

				events := eventinginmem.New()
				store := runstoreinmem.New()
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed store: %v", err)
				}
				persistedBefore, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load initial state: %v", err)
				}
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step = state.Step - 1
						next.Status = agent.RunStatusCompleted
						next.Output = "corrupt"
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, runErr := runner.Continue(context.Background(), runID, 3, nil)
				if !errors.Is(runErr, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", runErr)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
				}

				persistedAfter, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load persisted state: %v", err)
				}
				if !reflect.DeepEqual(persistedAfter, persistedBefore) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persistedAfter, persistedBefore)
				}
				assertNoCheckpointOrCommandAppliedEvents(t, events.Events())
			},
		},
		{
			name: "follow_up_transcript_prefix_changed",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-follow-up-prefix-changed")
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Step:   2,
					Messages: []agent.Message{
						{Role: agent.RoleSystem, Content: "system"},
						{Role: agent.RoleUser, Content: "seed"},
					},
				}

				events := eventinginmem.New()
				store := runstoreinmem.New()
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed store: %v", err)
				}
				persistedBefore, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load initial state: %v", err)
				}
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step = state.Step + 1
						next.Status = agent.RunStatusCompleted
						next.Output = "corrupt"
						next.Messages = append([]agent.Message(nil), state.Messages...)
						next.Messages[0].Content = "rewritten"
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, runErr := runner.FollowUp(context.Background(), runID, "follow up", 3, nil)
				if !errors.Is(runErr, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", runErr)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
				}

				persistedAfter, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load persisted state: %v", err)
				}
				if !reflect.DeepEqual(persistedAfter, persistedBefore) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persistedAfter, persistedBefore)
				}
				assertNoCheckpointOrCommandAppliedEvents(t, events.Events())
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, tc.run)
	}
}

func assertNoCheckpointOrCommandAppliedEvents(t *testing.T, events []agent.Event) {
	t.Helper()

	for _, event := range events {
		if event.Type == agent.EventTypeRunCheckpoint || event.Type == agent.EventTypeCommandApplied {
			t.Fatalf("unexpected event type on rejected output: %s", event.Type)
		}
	}
}

func countEventType(events []agent.Event, want agent.EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == want {
			count++
		}
	}
	return count
}
