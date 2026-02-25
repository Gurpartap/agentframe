package agent

import "errors"

var (
	// ErrMaxStepsExceeded is returned when the loop reaches its step budget.
	ErrMaxStepsExceeded = errors.New("react loop exceeded max steps")
	// ErrRunNotFound is returned by run stores when a run ID is unknown.
	ErrRunNotFound = errors.New("run not found")
)
