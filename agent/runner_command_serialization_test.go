package agent_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
)

func TestRunnerContinueConcurrentSameRunSerializedPreventsDoubleToolSideEffects(t *testing.T) {
	t.Parallel()

	const (
		runID         = agent.RunID("serialized-continue-tool-approval")
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
	engine := newBlockingContinueEngine(toolCallID, fingerprint)
	runner := newDispatchRunnerWithEngine(t, store, events, engine)
	resolution := &agent.Resolution{
		RequirementID: requirementID,
		Kind:          agent.RequirementKindApproval,
		Outcome:       agent.ResolutionOutcomeApproved,
	}

	type continueOutcome struct {
		result agent.RunResult
		err    error
	}
	firstDone := make(chan continueOutcome, 1)
	go func() {
		result, err := runner.Continue(context.Background(), runID, 3, nil, resolution)
		firstDone <- continueOutcome{result: result, err: err}
	}()

	select {
	case <-engine.firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first continue to reach engine")
	}

	secondDone := make(chan continueOutcome, 1)
	go func() {
		result, err := runner.Continue(context.Background(), runID, 3, nil, resolution)
		secondDone <- continueOutcome{result: result, err: err}
	}()

	select {
	case <-engine.secondEntered:
		t.Fatalf("second continue reached engine while first continue was still active")
	case <-secondDone:
		t.Fatalf("second continue completed before first continue released engine")
	default:
	}

	close(engine.releaseFirst)

	var first continueOutcome
	select {
	case first = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first continue result")
	}
	if first.err != nil {
		t.Fatalf("first continue returned error: %v", first.err)
	}
	if first.result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("first continue status mismatch: got=%s want=%s", first.result.State.Status, agent.RunStatusCompleted)
	}
	if first.result.State.PendingRequirement != nil {
		t.Fatalf("first continue should clear pending requirement")
	}

	var second continueOutcome
	select {
	case second = <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for second continue result")
	}
	if !errors.Is(second.err, agent.ErrRunNotContinuable) {
		t.Fatalf("expected ErrRunNotContinuable on second continue, got %v", second.err)
	}
	if second.result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("second continue should observe completed state, got %s", second.result.State.Status)
	}

	if calls := engine.calls.Load(); calls != 1 {
		t.Fatalf("engine call count mismatch: got=%d want=1", calls)
	}
	if sideEffects := engine.sideEffects.Load(); sideEffects != 1 {
		t.Fatalf("tool side effects must run once, got=%d", sideEffects)
	}

	loaded, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if loaded.Status != agent.RunStatusCompleted {
		t.Fatalf("persisted state status mismatch: got=%s want=%s", loaded.Status, agent.RunStatusCompleted)
	}
}

type blockingContinueEngine struct {
	expectedToolCallID  string
	expectedFingerprint string
	firstEntered        chan struct{}
	releaseFirst        chan struct{}
	secondEntered       chan struct{}
	calls               atomic.Int32
	sideEffects         atomic.Int32
}

func newBlockingContinueEngine(expectedToolCallID, expectedFingerprint string) *blockingContinueEngine {
	return &blockingContinueEngine{
		expectedToolCallID:  expectedToolCallID,
		expectedFingerprint: expectedFingerprint,
		firstEntered:        make(chan struct{}),
		releaseFirst:        make(chan struct{}),
		secondEntered:       make(chan struct{}, 1),
	}
}

func (e *blockingContinueEngine) Execute(
	ctx context.Context,
	state agent.RunState,
	input agent.EngineInput,
) (agent.RunState, error) {
	call := e.calls.Add(1)
	if call == 1 {
		if input.ResolvedRequirement == nil {
			return state, fmt.Errorf("missing resolved requirement on first continue")
		}
		if input.ResolvedRequirement.ToolCallID != e.expectedToolCallID {
			return state, fmt.Errorf("resolved requirement tool_call_id mismatch: got=%q", input.ResolvedRequirement.ToolCallID)
		}
		if input.ResolvedRequirement.Fingerprint != e.expectedFingerprint {
			return state, fmt.Errorf("resolved requirement fingerprint mismatch: got=%q", input.ResolvedRequirement.Fingerprint)
		}
		override, ok := agent.ApprovedToolCallReplayOverrideFromContext(ctx)
		if !ok {
			return state, fmt.Errorf("missing approved tool call replay override in context")
		}
		if override.ToolCallID != e.expectedToolCallID {
			return state, fmt.Errorf("override tool_call_id mismatch: got=%q", override.ToolCallID)
		}
		if override.Fingerprint != e.expectedFingerprint {
			return state, fmt.Errorf("override fingerprint mismatch: got=%q", override.Fingerprint)
		}
		close(e.firstEntered)
		<-e.releaseFirst
	} else {
		select {
		case e.secondEntered <- struct{}{}:
		default:
		}
	}

	e.sideEffects.Add(1)
	next := state
	next.Step++
	if err := agent.TransitionRunStatus(&next, agent.RunStatusCompleted); err != nil {
		return state, err
	}
	next.Output = "done"
	return next, nil
}
