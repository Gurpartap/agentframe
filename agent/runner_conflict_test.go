package agent_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"agentruntime/agent"
	eventinginmem "agentruntime/eventing/inmem"
	runstoreinmem "agentruntime/runstore/inmem"
)

func TestRunnerMutatingCommands_SaveConflictReturnsNormalizedCommandConflict(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "continue",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("conflict-continue")
				store := newConflictInjectingStore(2)
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Step:   3,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "continue"},
					},
				}
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed state: %v", err)
				}
				persistedBefore, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load seeded state: %v", err)
				}

				events := eventinginmem.New()
				engine := &conflictTestEngine{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := agent.CloneRunState(state)
						next.Step++
						next.Status = agent.RunStatusCompleted
						next.Output = "continued"
						return next, nil
					},
				}
				runner := newConflictTestRunner(t, store, engine, events)

				result, runErr := runner.Continue(context.Background(), runID, 3, nil, nil)
				if !errors.Is(runErr, agent.ErrCommandConflict) {
					t.Fatalf("expected ErrCommandConflict, got %v", runErr)
				}
				if !errors.Is(runErr, agent.ErrRunVersionConflict) {
					t.Fatalf("expected ErrRunVersionConflict compatibility, got %v", runErr)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute once, calls=%d", engine.calls)
				}

				persistedAfter, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("reload state: %v", err)
				}
				if !reflect.DeepEqual(persistedAfter, persistedBefore) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persistedAfter, persistedBefore)
				}
				assertNoCheckpointOrCommandEvents(t, events.Events())
			},
		},
		{
			name: "follow_up",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("conflict-follow-up")
				store := newConflictInjectingStore(2)
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Step:   2,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "seed"},
					},
				}
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed state: %v", err)
				}
				persistedBefore, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load seeded state: %v", err)
				}

				events := eventinginmem.New()
				engine := &conflictTestEngine{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := agent.CloneRunState(state)
						next.Step++
						next.Status = agent.RunStatusCompleted
						next.Output = "follow-up"
						return next, nil
					},
				}
				runner := newConflictTestRunner(t, store, engine, events)

				result, runErr := runner.FollowUp(context.Background(), runID, "follow-up prompt", 3, nil)
				if !errors.Is(runErr, agent.ErrCommandConflict) {
					t.Fatalf("expected ErrCommandConflict, got %v", runErr)
				}
				if !errors.Is(runErr, agent.ErrRunVersionConflict) {
					t.Fatalf("expected ErrRunVersionConflict compatibility, got %v", runErr)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute once, calls=%d", engine.calls)
				}

				persistedAfter, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("reload state: %v", err)
				}
				if !reflect.DeepEqual(persistedAfter, persistedBefore) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persistedAfter, persistedBefore)
				}
				assertNoCheckpointOrCommandEvents(t, events.Events())
			},
		},
		{
			name: "cancel",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("conflict-cancel")
				store := newConflictInjectingStore(2)
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusRunning,
					Step:   7,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "seed"},
					},
				}
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed state: %v", err)
				}
				persistedBefore, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load seeded state: %v", err)
				}

				events := eventinginmem.New()
				engine := &conflictTestEngine{}
				runner := newConflictTestRunner(t, store, engine, events)

				result, runErr := runner.Cancel(context.Background(), runID)
				if !errors.Is(runErr, agent.ErrCommandConflict) {
					t.Fatalf("expected ErrCommandConflict, got %v", runErr)
				}
				if !errors.Is(runErr, agent.ErrRunVersionConflict) {
					t.Fatalf("expected ErrRunVersionConflict compatibility, got %v", runErr)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 0 {
					t.Fatalf("cancel must not invoke engine, calls=%d", engine.calls)
				}

				persistedAfter, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("reload state: %v", err)
				}
				if !reflect.DeepEqual(persistedAfter, persistedBefore) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persistedAfter, persistedBefore)
				}
				assertNoCheckpointOrCommandEvents(t, events.Events())
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, tc.run)
	}
}

func assertNoCheckpointOrCommandEvents(t *testing.T, events []agent.Event) {
	t.Helper()
	for _, event := range events {
		if event.Type == agent.EventTypeRunCheckpoint || event.Type == agent.EventTypeCommandApplied {
			t.Fatalf("unexpected event type on save conflict: %s", event.Type)
		}
	}
}

func newConflictTestRunner(
	t *testing.T,
	store agent.RunStore,
	engine agent.Engine,
	events agent.EventSink,
) *agent.Runner {
	t.Helper()

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("conflict"),
		RunStore:    store,
		Engine:      engine,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner
}

type conflictInjectingStore struct {
	base               *runstoreinmem.Store
	saveCalls          int
	conflictOnSaveCall int
}

func newConflictInjectingStore(conflictOnSaveCall int) *conflictInjectingStore {
	return &conflictInjectingStore{
		base:               runstoreinmem.New(),
		conflictOnSaveCall: conflictOnSaveCall,
	}
}

func (s *conflictInjectingStore) Save(ctx context.Context, state agent.RunState) error {
	s.saveCalls++
	if s.saveCalls == s.conflictOnSaveCall {
		return fmt.Errorf("%w: injected", agent.ErrRunVersionConflict)
	}
	return s.base.Save(ctx, state)
}

func (s *conflictInjectingStore) Load(ctx context.Context, runID agent.RunID) (agent.RunState, error) {
	return s.base.Load(ctx, runID)
}

type conflictTestEngine struct {
	calls     int
	executeFn func(context.Context, agent.RunState, agent.EngineInput) (agent.RunState, error)
}

func (e *conflictTestEngine) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	e.calls++
	if e.executeFn == nil {
		return state, nil
	}
	return e.executeFn(ctx, state, input)
}
