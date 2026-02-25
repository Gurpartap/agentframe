package agent

// CommandKind identifies the command mutation route.
type CommandKind string

const (
	CommandKindStart    CommandKind = "start"
	CommandKindContinue CommandKind = "continue"
	CommandKindCancel   CommandKind = "cancel"
)

// Command is the typed runtime mutation contract.
type Command interface {
	Kind() CommandKind
}

// StartCommand starts a new run.
type StartCommand struct {
	Input RunInput
}

func (StartCommand) Kind() CommandKind {
	return CommandKindStart
}

// ContinueCommand continues an existing non-terminal run.
type ContinueCommand struct {
	RunID    RunID
	MaxSteps int
	Tools    []ToolDefinition
}

func (ContinueCommand) Kind() CommandKind {
	return CommandKindContinue
}

// CancelCommand cancels an existing non-terminal run.
type CancelCommand struct {
	RunID RunID
}

func (CancelCommand) Kind() CommandKind {
	return CommandKindCancel
}
