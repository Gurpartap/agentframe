package agentreact

import (
	"fmt"

	"agentruntime/agent"
)

func validateRunStatusTransition(from, to agent.RunStatus) error {
	if from == to {
		return nil
	}

	allowed, ok := allowedRunStatusTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown source status %q", agent.ErrInvalidRunStateTransition, from)
	}
	if _, ok := allowed[to]; !ok {
		return fmt.Errorf("%w: %s -> %s", agent.ErrInvalidRunStateTransition, from, to)
	}
	return nil
}

func transitionRunStatus(state *agent.RunState, to agent.RunStatus) error {
	if err := validateRunStatusTransition(state.Status, to); err != nil {
		return err
	}
	state.Status = to
	return nil
}

var allowedRunStatusTransitions = map[agent.RunStatus]map[agent.RunStatus]struct{}{
	"": {
		agent.RunStatusPending: {},
	},
	agent.RunStatusPending: {
		agent.RunStatusRunning:   {},
		agent.RunStatusCancelled: {},
	},
	agent.RunStatusRunning: {
		agent.RunStatusSuspended:        {},
		agent.RunStatusCancelled:        {},
		agent.RunStatusCompleted:        {},
		agent.RunStatusFailed:           {},
		agent.RunStatusMaxStepsExceeded: {},
	},
	agent.RunStatusSuspended: {
		agent.RunStatusRunning:   {},
		agent.RunStatusCancelled: {},
	},
	agent.RunStatusMaxStepsExceeded: {
		agent.RunStatusRunning:   {},
		agent.RunStatusCancelled: {},
	},
	agent.RunStatusCompleted: {},
	agent.RunStatusFailed:    {},
	agent.RunStatusCancelled: {},
}
