package runtimewire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire"
)

func TestCancellationBehaviorDeterministic(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t)

	result, runErr := rt.Runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "[loop] cancellation target",
		MaxSteps:   1,
		Tools:      rt.ToolDefinitions,
	})
	if !errors.Is(runErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("initial status mismatch: got=%s want=%s", result.State.Status, agent.RunStatusMaxStepsExceeded)
	}

	cancelled, cancelErr := rt.Runner.Cancel(context.Background(), result.State.ID)
	if cancelErr != nil {
		t.Fatalf("cancel run: %v", cancelErr)
	}
	if cancelled.State.Status != agent.RunStatusCancelled {
		t.Fatalf("cancelled status mismatch: got=%s want=%s", cancelled.State.Status, agent.RunStatusCancelled)
	}

	repeated, repeatedErr := rt.Runner.Cancel(context.Background(), result.State.ID)
	if !errors.Is(repeatedErr, agent.ErrRunNotCancellable) {
		t.Fatalf("expected ErrRunNotCancellable, got %v", repeatedErr)
	}
	if repeated.State.Status != agent.RunStatusCancelled {
		t.Fatalf("repeated cancel state mismatch: got=%s want=%s", repeated.State.Status, agent.RunStatusCancelled)
	}
}

func TestConflictBehaviorDeterministic(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t)

	state := agent.RunState{ID: "conflict-run", Step: 0}
	if err := agent.TransitionRunStatus(&state, agent.RunStatusPending); err != nil {
		t.Fatalf("transition to pending: %v", err)
	}

	if err := rt.RunStore.Save(context.Background(), state); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	staleA, err := rt.RunStore.Load(context.Background(), state.ID)
	if err != nil {
		t.Fatalf("load staleA: %v", err)
	}
	staleB, err := rt.RunStore.Load(context.Background(), state.ID)
	if err != nil {
		t.Fatalf("load staleB: %v", err)
	}

	if err := agent.TransitionRunStatus(&staleA, agent.RunStatusRunning); err != nil {
		t.Fatalf("transition staleA to running: %v", err)
	}
	if err := rt.RunStore.Save(context.Background(), staleA); err != nil {
		t.Fatalf("save staleA: %v", err)
	}

	if err := agent.TransitionRunStatus(&staleB, agent.RunStatusRunning); err != nil {
		t.Fatalf("transition staleB to running: %v", err)
	}
	if err := rt.RunStore.Save(context.Background(), staleB); !errors.Is(err, agent.ErrRunVersionConflict) {
		t.Fatalf("expected ErrRunVersionConflict, got %v", err)
	}
}

func TestRepeatedContinueDeterminism(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t)

	result, runErr := rt.Runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "[loop] repeat continue",
		MaxSteps:   1,
		Tools:      rt.ToolDefinitions,
	})
	if !errors.Is(runErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", runErr)
	}

	expectedStep := result.State.Step
	for i := 0; i < 3; i++ {
		continued, continueErr := rt.Runner.Continue(context.Background(), result.State.ID, 1, rt.ToolDefinitions, nil)
		if !errors.Is(continueErr, agent.ErrMaxStepsExceeded) {
			t.Fatalf("continue %d expected ErrMaxStepsExceeded, got %v", i+1, continueErr)
		}
		if continued.State.Status != agent.RunStatusMaxStepsExceeded {
			t.Fatalf(
				"continue %d status mismatch: got=%s want=%s",
				i+1,
				continued.State.Status,
				agent.RunStatusMaxStepsExceeded,
			)
		}
		if continued.State.Step != expectedStep {
			t.Fatalf(
				"continue %d step mismatch: got=%d want=%d",
				i+1,
				continued.State.Step,
				expectedStep,
			)
		}
		if continued.State.Error != agent.ErrMaxStepsExceeded.Error() {
			t.Fatalf(
				"continue %d error mismatch: got=%q want=%q",
				i+1,
				continued.State.Error,
				agent.ErrMaxStepsExceeded.Error(),
			)
		}
	}
}

func newRuntime(t *testing.T) *runtimewire.Runtime {
	t.Helper()

	rt, err := runtimewire.New(config.Default())
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return rt
}
