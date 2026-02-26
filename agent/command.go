package agent

// CommandKind identifies the command mutation route.
type CommandKind string

const (
	CommandKindStart    CommandKind = "start"
	CommandKindContinue CommandKind = "continue"
	CommandKindCancel   CommandKind = "cancel"
	CommandKindSteer    CommandKind = "steer"
	CommandKindFollowUp CommandKind = "follow_up"
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
	RunID      RunID
	CommandID  string
	MaxSteps   int
	Tools      []ToolDefinition
	Resolution *Resolution
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

// SteerCommand appends an instruction to a non-terminal run without executing the engine.
type SteerCommand struct {
	RunID       RunID
	Instruction string
}

func (SteerCommand) Kind() CommandKind {
	return CommandKindSteer
}

// FollowUpCommand appends a prompt to a non-terminal run and executes the engine.
type FollowUpCommand struct {
	RunID      RunID
	UserPrompt string
	MaxSteps   int
	Tools      []ToolDefinition
}

func (FollowUpCommand) Kind() CommandKind {
	return CommandKindFollowUp
}
