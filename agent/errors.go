package agent

import "errors"

var (
	// ErrMaxStepsExceeded is returned when the loop reaches its step budget.
	ErrMaxStepsExceeded = errors.New("react loop exceeded max steps")
	// ErrRunNotFound is returned by run stores when a run ID is unknown.
	ErrRunNotFound = errors.New("run not found")
	// ErrInvalidRunStateTransition is returned when a run state transition violates lifecycle rules.
	ErrInvalidRunStateTransition = errors.New("invalid run state transition")
	// ErrRunNotContinuable is returned when continue is requested for a terminal run.
	ErrRunNotContinuable = errors.New("run is not continuable")
	// ErrRunNotCancellable is returned when cancel is requested for a terminal run.
	ErrRunNotCancellable = errors.New("run is not cancellable")
)
