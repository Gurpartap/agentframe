package agent

import "fmt"

func isTerminalRunStatus(status RunStatus) bool {
	switch status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

func validateRunStatusTransition(from, to RunStatus) error {
	if from == to {
		return nil
	}

	allowed, ok := allowedRunStatusTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown source status %q", ErrInvalidRunStateTransition, from)
	}
	if _, ok := allowed[to]; !ok {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidRunStateTransition, from, to)
	}
	return nil
}

func transitionRunStatus(state *RunState, to RunStatus) error {
	if err := validateRunStatusTransition(state.Status, to); err != nil {
		return err
	}
	state.Status = to
	return nil
}

var allowedRunStatusTransitions = map[RunStatus]map[RunStatus]struct{}{
	"": {
		RunStatusPending: {},
	},
	RunStatusPending: {
		RunStatusRunning:   {},
		RunStatusCancelled: {},
	},
	RunStatusRunning: {
		RunStatusSuspended:        {},
		RunStatusCancelled:        {},
		RunStatusCompleted:        {},
		RunStatusFailed:           {},
		RunStatusMaxStepsExceeded: {},
	},
	RunStatusSuspended: {
		RunStatusRunning:   {},
		RunStatusCancelled: {},
	},
	RunStatusMaxStepsExceeded: {
		RunStatusRunning:   {},
		RunStatusCancelled: {},
	},
	RunStatusCompleted: {},
	RunStatusFailed:    {},
	RunStatusCancelled: {},
}
