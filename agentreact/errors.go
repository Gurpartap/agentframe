package agentreact

import "errors"

var (
	// ErrMissingModel is returned when New is called without a model dependency.
	ErrMissingModel = errors.New("missing model")
	// ErrMissingToolExecutor is returned when New is called without a tool executor dependency.
	ErrMissingToolExecutor = errors.New("missing tool executor")
	// ErrToolCallInvalid is returned when the model produces an invalid tool call shape.
	ErrToolCallInvalid = errors.New("tool call is invalid")
)
