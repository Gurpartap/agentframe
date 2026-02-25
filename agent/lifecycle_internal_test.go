package agent

import (
	"errors"
	"testing"
)

type runStatusTransitionCase struct {
	name string
	from RunStatus
	to   RunStatus
}

func validRunStatusTransitions() []runStatusTransitionCase {
	return []runStatusTransitionCase{
		{name: "initialize_to_pending", from: "", to: RunStatusPending},
		{name: "pending_to_running", from: RunStatusPending, to: RunStatusRunning},
		{name: "pending_to_cancelled", from: RunStatusPending, to: RunStatusCancelled},
		{name: "running_to_suspended", from: RunStatusRunning, to: RunStatusSuspended},
		{name: "running_to_max_steps", from: RunStatusRunning, to: RunStatusMaxStepsExceeded},
		{name: "running_to_completed", from: RunStatusRunning, to: RunStatusCompleted},
		{name: "running_to_failed", from: RunStatusRunning, to: RunStatusFailed},
		{name: "running_to_cancelled", from: RunStatusRunning, to: RunStatusCancelled},
		{name: "suspended_to_running", from: RunStatusSuspended, to: RunStatusRunning},
		{name: "suspended_to_cancelled", from: RunStatusSuspended, to: RunStatusCancelled},
		{name: "max_steps_to_running", from: RunStatusMaxStepsExceeded, to: RunStatusRunning},
		{name: "max_steps_to_cancelled", from: RunStatusMaxStepsExceeded, to: RunStatusCancelled},
	}
}

func invalidRunStatusTransitions() []runStatusTransitionCase {
	return []runStatusTransitionCase{
		{name: "pending_to_completed", from: RunStatusPending, to: RunStatusCompleted},
		{name: "completed_to_running", from: RunStatusCompleted, to: RunStatusRunning},
		{name: "failed_to_running", from: RunStatusFailed, to: RunStatusRunning},
		{name: "cancelled_to_running", from: RunStatusCancelled, to: RunStatusRunning},
		{name: "unknown_source", from: RunStatus("unknown"), to: RunStatusRunning},
	}
}

func TestValidateRunStatusTransition_Valid(t *testing.T) {
	t.Parallel()

	for _, tc := range validRunStatusTransitions() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := validateRunStatusTransition(tc.from, tc.to); err != nil {
				t.Fatalf("expected transition %s -> %s to be valid, got %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateRunStatusTransition_Invalid(t *testing.T) {
	t.Parallel()

	for _, tc := range invalidRunStatusTransitions() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateRunStatusTransition(tc.from, tc.to)
			if !errors.Is(err, ErrInvalidRunStateTransition) {
				t.Fatalf("expected ErrInvalidRunStateTransition for %s -> %s, got %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestTransitionRunStatus_Valid(t *testing.T) {
	t.Parallel()

	for _, tc := range validRunStatusTransitions() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := RunState{Status: tc.from}
			if err := TransitionRunStatus(&state, tc.to); err != nil {
				t.Fatalf("expected transition %s -> %s to be valid, got %v", tc.from, tc.to, err)
			}
			if state.Status != tc.to {
				t.Fatalf("unexpected status after transition: got=%s want=%s", state.Status, tc.to)
			}
		})
	}
}

func TestTransitionRunStatus_Invalid(t *testing.T) {
	t.Parallel()

	for _, tc := range invalidRunStatusTransitions() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := RunState{Status: tc.from}
			err := TransitionRunStatus(&state, tc.to)
			if !errors.Is(err, ErrInvalidRunStateTransition) {
				t.Fatalf("expected ErrInvalidRunStateTransition for %s -> %s, got %v", tc.from, tc.to, err)
			}
			if state.Status != tc.from {
				t.Fatalf("invalid transition changed status: got=%s want=%s", state.Status, tc.from)
			}
		})
	}
}
