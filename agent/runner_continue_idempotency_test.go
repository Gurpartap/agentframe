package agent_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
)

func TestRunnerContinueSameCommandIDDoesNotReexecuteTools(t *testing.T) {
	t.Parallel()

	fixture := newContinueIdempotencyFixture(t)

	_, err := fixture.runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:      fixture.runID,
		CommandID:  "continue-1",
		MaxSteps:   3,
		Resolution: fixture.resolution,
	})
	if err != nil {
		t.Fatalf("first continue: %v", err)
	}

	_, err = fixture.runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:      fixture.runID,
		CommandID:  "continue-1",
		MaxSteps:   3,
		Resolution: fixture.resolution,
	})
	if err != nil {
		t.Fatalf("duplicate continue: %v", err)
	}

	if calls := fixture.engine.calls.Load(); calls != 1 {
		t.Fatalf("engine execute count mismatch: got=%d want=1", calls)
	}
	if sideEffects := fixture.engine.sideEffects.Load(); sideEffects != 1 {
		t.Fatalf("tool side effect count mismatch: got=%d want=1", sideEffects)
	}
}

func TestRunnerContinueSameCommandIDReturnsStableSuccessOutcome(t *testing.T) {
	t.Parallel()

	fixture := newContinueIdempotencyFixture(t)

	first, firstErr := fixture.runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:      fixture.runID,
		CommandID:  "continue-stable-1",
		MaxSteps:   3,
		Resolution: fixture.resolution,
	})
	if firstErr != nil {
		t.Fatalf("first continue: %v", firstErr)
	}

	second, secondErr := fixture.runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:      fixture.runID,
		CommandID:  "continue-stable-1",
		MaxSteps:   3,
		Resolution: fixture.resolution,
	})
	if secondErr != nil {
		t.Fatalf("duplicate continue: %v", secondErr)
	}

	if !reflect.DeepEqual(second.State, first.State) {
		t.Fatalf("cached continue state mismatch:\nfirst=%+v\nsecond=%+v", first.State, second.State)
	}
	if calls := fixture.engine.calls.Load(); calls != 1 {
		t.Fatalf("engine execute count mismatch: got=%d want=1", calls)
	}
}

func TestRunnerContinueDifferentCommandIDFollowsNormalFlow(t *testing.T) {
	t.Parallel()

	fixture := newContinueIdempotencyFixture(t)

	_, err := fixture.runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:      fixture.runID,
		CommandID:  "continue-1",
		MaxSteps:   3,
		Resolution: fixture.resolution,
	})
	if err != nil {
		t.Fatalf("first continue: %v", err)
	}

	second, secondErr := fixture.runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:      fixture.runID,
		CommandID:  "continue-2",
		MaxSteps:   3,
		Resolution: fixture.resolution,
	})
	if !errors.Is(secondErr, agent.ErrRunNotContinuable) {
		t.Fatalf("expected ErrRunNotContinuable for different command id, got %v", secondErr)
	}
	if second.State.Status != agent.RunStatusCompleted {
		t.Fatalf("expected completed state on different command id replay, got=%s", second.State.Status)
	}

	if calls := fixture.engine.calls.Load(); calls != 1 {
		t.Fatalf("engine execute count mismatch: got=%d want=1", calls)
	}
	if sideEffects := fixture.engine.sideEffects.Load(); sideEffects != 1 {
		t.Fatalf("tool side effect count mismatch: got=%d want=1", sideEffects)
	}
}

type continueIdempotencyFixture struct {
	runner     *agent.Runner
	engine     *continueIdempotencyEngine
	runID      agent.RunID
	resolution *agent.Resolution
}

func newContinueIdempotencyFixture(t *testing.T) continueIdempotencyFixture {
	t.Helper()

	const (
		runID         = agent.RunID("continue-idempotency-run")
		requirementID = "req-tool-approval"
		toolCallID    = "call-1"
		fingerprint   = "fp-call-1"
	)

	store := runstoreinmem.New()
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusSuspended,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
		PendingRequirement: &agent.PendingRequirement{
			ID:          requirementID,
			Kind:        agent.RequirementKindApproval,
			Origin:      agent.RequirementOriginTool,
			ToolCallID:  toolCallID,
			Fingerprint: fingerprint,
		},
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	events := eventinginmem.New()
	engine := &continueIdempotencyEngine{}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	return continueIdempotencyFixture{
		runner: runner,
		engine: engine,
		runID:  runID,
		resolution: &agent.Resolution{
			RequirementID: requirementID,
			Kind:          agent.RequirementKindApproval,
			Outcome:       agent.ResolutionOutcomeApproved,
		},
	}
}

type continueIdempotencyEngine struct {
	calls       atomic.Int32
	sideEffects atomic.Int32
}

func (e *continueIdempotencyEngine) Execute(
	_ context.Context,
	state agent.RunState,
	_ agent.EngineInput,
) (agent.RunState, error) {
	e.calls.Add(1)
	e.sideEffects.Add(1)

	next := state
	next.Step++
	if err := agent.TransitionRunStatus(&next, agent.RunStatusCompleted); err != nil {
		return state, err
	}
	next.Output = "done"
	return next, nil
}
