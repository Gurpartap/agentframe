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

func isKnownRequirementKind(kind RequirementKind) bool {
	switch kind {
	case RequirementKindApproval,
		RequirementKindUserInput,
		RequirementKindExternalExecution:
		return true
	default:
		return false
	}
}

func isKnownResolutionOutcome(outcome ResolutionOutcome) bool {
	switch outcome {
	case ResolutionOutcomeApproved,
		ResolutionOutcomeRejected,
		ResolutionOutcomeProvided,
		ResolutionOutcomeCompleted:
		return true
	default:
		return false
	}
}

func validatePendingRequirementContract(requirement *PendingRequirement) error {
	if requirement == nil {
		return fmt.Errorf("%w: field=pending_requirement reason=nil", ErrRunStateInvalid)
	}
	if requirement.ID == "" {
		return fmt.Errorf("%w: field=pending_requirement.id reason=empty", ErrRunStateInvalid)
	}
	if !isKnownRequirementKind(requirement.Kind) {
		return fmt.Errorf(
			"%w: field=pending_requirement.kind reason=unknown value=%q",
			ErrRunStateInvalid,
			requirement.Kind,
		)
	}
	return nil
}

func validateSuspensionInvariant(state RunState) error {
	if state.Status == RunStatusSuspended {
		return validatePendingRequirementContract(state.PendingRequirement)
	}
	if state.PendingRequirement != nil {
		return fmt.Errorf(
			"%w: field=pending_requirement reason=forbidden_for_status status=%s",
			ErrRunStateInvalid,
			state.Status,
		)
	}
	return nil
}

func validateResolutionContract(resolution *Resolution) error {
	if resolution == nil {
		return fmt.Errorf("%w: field=resolution reason=nil", ErrResolutionInvalid)
	}
	if resolution.RequirementID == "" {
		return fmt.Errorf("%w: field=resolution.requirement_id reason=empty", ErrResolutionInvalid)
	}
	if !isKnownRequirementKind(resolution.Kind) {
		return fmt.Errorf(
			"%w: field=resolution.kind reason=unknown value=%q",
			ErrResolutionInvalid,
			resolution.Kind,
		)
	}
	if !isKnownResolutionOutcome(resolution.Outcome) {
		return fmt.Errorf(
			"%w: field=resolution.outcome reason=unknown value=%q",
			ErrResolutionInvalid,
			resolution.Outcome,
		)
	}
	switch resolution.Kind {
	case RequirementKindApproval:
		if resolution.Outcome != ResolutionOutcomeApproved && resolution.Outcome != ResolutionOutcomeRejected {
			return fmt.Errorf(
				"%w: field=resolution.outcome reason=invalid_for_kind kind=%s outcome=%s",
				ErrResolutionInvalid,
				resolution.Kind,
				resolution.Outcome,
			)
		}
	case RequirementKindUserInput:
		if resolution.Outcome != ResolutionOutcomeProvided {
			return fmt.Errorf(
				"%w: field=resolution.outcome reason=invalid_for_kind kind=%s outcome=%s",
				ErrResolutionInvalid,
				resolution.Kind,
				resolution.Outcome,
			)
		}
	case RequirementKindExternalExecution:
		if resolution.Outcome != ResolutionOutcomeCompleted {
			return fmt.Errorf(
				"%w: field=resolution.outcome reason=invalid_for_kind kind=%s outcome=%s",
				ErrResolutionInvalid,
				resolution.Kind,
				resolution.Outcome,
			)
		}
	}
	return nil
}

func validateResolutionForRequirement(resolution *Resolution, requirement *PendingRequirement) error {
	if err := validateResolutionContract(resolution); err != nil {
		return err
	}
	if requirement == nil {
		return fmt.Errorf("%w: field=pending_requirement reason=nil", ErrResolutionInvalid)
	}
	if err := validatePendingRequirementContract(requirement); err != nil {
		return err
	}
	if resolution.RequirementID != requirement.ID {
		return fmt.Errorf(
			"%w: field=resolution.requirement_id reason=mismatch got=%q want=%q",
			ErrResolutionInvalid,
			resolution.RequirementID,
			requirement.ID,
		)
	}
	if resolution.Kind != requirement.Kind {
		return fmt.Errorf(
			"%w: field=resolution.kind reason=mismatch got=%q want=%q",
			ErrResolutionInvalid,
			resolution.Kind,
			requirement.Kind,
		)
	}
	return nil
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

// TransitionRunStatus validates and applies a run status transition.
func TransitionRunStatus(state *RunState, to RunStatus) error {
	from := state.Status
	if err := validateRunStatusTransition(state.Status, to); err != nil {
		return err
	}
	state.Status = to
	if err := validateSuspensionInvariant(*state); err != nil {
		state.Status = from
		return err
	}
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
