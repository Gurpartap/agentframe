package agent

import "fmt"

// SuspendRequestError signals that execution should transition to suspended status.
type SuspendRequestError struct {
	Requirement *PendingRequirement
	Err         error
}

func (e *SuspendRequestError) Error() string {
	if e == nil {
		return "suspend request"
	}

	message := "suspend request"
	if e.Requirement != nil {
		message = fmt.Sprintf(
			"suspend request: requirement id=%q kind=%s origin=%s",
			e.Requirement.ID,
			e.Requirement.Kind,
			e.Requirement.Origin,
		)
	}
	if e.Err != nil {
		return message + ": " + e.Err.Error()
	}
	return message
}

func (e *SuspendRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
