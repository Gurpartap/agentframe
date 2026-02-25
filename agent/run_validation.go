package agent

import (
	"errors"
	"fmt"
)

// ValidateRunState checks structural run-state invariants before persistence boundaries.
func ValidateRunState(state RunState) error {
	if state.ID == "" {
		return errors.Join(
			ErrRunStateInvalid,
			fmt.Errorf("%w: field=id reason=empty", ErrInvalidRunID),
		)
	}
	if state.Step < 0 {
		return fmt.Errorf(
			"%w: field=step reason=negative value=%d run_id=%q",
			ErrRunStateInvalid,
			state.Step,
			state.ID,
		)
	}
	if state.Version < 0 {
		return fmt.Errorf(
			"%w: field=version reason=negative value=%d run_id=%q",
			ErrRunStateInvalid,
			state.Version,
			state.ID,
		)
	}
	if !isKnownRunStatus(state.Status) {
		return fmt.Errorf(
			"%w: field=status reason=unknown value=%q run_id=%q",
			ErrRunStateInvalid,
			state.Status,
			state.ID,
		)
	}
	return nil
}

func isKnownRunStatus(status RunStatus) bool {
	switch status {
	case RunStatusPending,
		RunStatusRunning,
		RunStatusSuspended,
		RunStatusCancelled,
		RunStatusCompleted,
		RunStatusFailed,
		RunStatusMaxStepsExceeded:
		return true
	default:
		return false
	}
}
