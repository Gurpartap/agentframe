package agent

import "errors"

var (
	// ErrMaxStepsExceeded is returned when the loop reaches its step budget.
	ErrMaxStepsExceeded = errors.New("run exceeded max steps")
	// ErrRunNotFound is returned by run stores when a run ID is unknown.
	ErrRunNotFound = errors.New("run not found")
	// ErrRunVersionConflict is returned when a save is attempted with a stale run version.
	ErrRunVersionConflict = errors.New("run version conflict")
	// ErrInvalidRunStateTransition is returned when a run state transition violates lifecycle rules.
	ErrInvalidRunStateTransition = errors.New("invalid run state transition")
	// ErrRunNotContinuable is returned when continue is requested for a terminal run.
	ErrRunNotContinuable = errors.New("run is not continuable")
	// ErrRunNotCancellable is returned when cancel is requested for a terminal run.
	ErrRunNotCancellable = errors.New("run is not cancellable")
	// ErrCommandNil is returned when dispatch is called with a nil command.
	ErrCommandNil = errors.New("command is nil")
	// ErrContextNil is returned when runtime commands are invoked with a nil context.
	ErrContextNil = errors.New("context is nil")
	// ErrCommandInvalid is returned when command kind and payload type are inconsistent.
	ErrCommandInvalid = errors.New("command is invalid")
	// ErrCommandUnsupported is returned when command kind has no runtime handler.
	ErrCommandUnsupported = errors.New("command is unsupported")
	// ErrInvalidRunID is returned when a runtime command is invoked with an empty or invalid run ID.
	ErrInvalidRunID = errors.New("invalid run id")
	// ErrEventPublish is returned when runtime event emission fails.
	ErrEventPublish = errors.New("event publish failed")
	// ErrEventInvalid is returned when an event payload violates required runtime contracts.
	ErrEventInvalid = errors.New("event is invalid")
	// ErrEngineOutputContractViolation is returned when engine output violates runtime state invariants.
	ErrEngineOutputContractViolation = errors.New("engine output contract violation")
	// ErrToolDefinitionsInvalid is returned when command tool definitions violate runtime input constraints.
	ErrToolDefinitionsInvalid = errors.New("tool definitions are invalid")
	// ErrMissingIDGenerator is returned when NewRunner is called without an ID generator dependency.
	ErrMissingIDGenerator = errors.New("missing id generator")
	// ErrMissingRunStore is returned when NewRunner is called without a run store dependency.
	ErrMissingRunStore = errors.New("missing run store")
	// ErrMissingEngine is returned when NewRunner is called without an engine dependency.
	ErrMissingEngine = errors.New("missing engine")
)
