package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// Dependencies wires application services into the runtime orchestrator.
type Dependencies struct {
	IDGenerator IDGenerator
	RunStore    RunStore
	Engine      Engine
	EventSink   EventSink
}

// Runner owns the run lifecycle and persistence.
type Runner struct {
	idGen  IDGenerator
	store  RunStore
	engine Engine
	events EventSink
}

func NewRunner(deps Dependencies) (*Runner, error) {
	if deps.IDGenerator == nil {
		return nil, fmt.Errorf("new runner: %w", ErrMissingIDGenerator)
	}
	if deps.RunStore == nil {
		return nil, fmt.Errorf("new runner: %w", ErrMissingRunStore)
	}
	if deps.Engine == nil {
		return nil, fmt.Errorf("new runner: %w", ErrMissingEngine)
	}
	if deps.EventSink == nil {
		deps.EventSink = noopEventSink{}
	}
	return &Runner{
		idGen:  deps.IDGenerator,
		store:  deps.RunStore,
		engine: deps.Engine,
		events: deps.EventSink,
	}, nil
}

func publishEvent(ctx context.Context, sink EventSink, event Event) error {
	if err := ValidateEvent(event); err != nil {
		return err
	}
	if err := sink.Publish(ctx, event); err != nil {
		return errors.Join(
			ErrEventPublish,
			fmt.Errorf(
				"type=%s run_id=%s step=%d: %w",
				event.Type,
				event.RunID,
				event.Step,
				err,
			),
		)
	}
	return nil
}

func cancellationEventDescription(runErr error) string {
	if runErr == nil {
		return "run cancelled"
	}
	return runErr.Error()
}

func suspensionEventDescription(state RunState) string {
	if state.PendingRequirement == nil {
		return "run suspended"
	}
	return fmt.Sprintf(
		"run suspended awaiting requirement origin=%s kind=%s id=%q",
		state.PendingRequirement.Origin,
		state.PendingRequirement.Kind,
		state.PendingRequirement.ID,
	)
}

func validateContinueResolution(state RunState, command ContinueCommand) error {
	if state.Status != RunStatusSuspended {
		if command.Resolution != nil {
			return fmt.Errorf(
				"%w: command=%s status=%s run_id=%q",
				ErrResolutionUnexpected,
				CommandKindContinue,
				state.Status,
				state.ID,
			)
		}
		return nil
	}
	if command.Resolution == nil {
		return fmt.Errorf(
			"%w: command=%s status=%s run_id=%q",
			ErrResolutionRequired,
			CommandKindContinue,
			state.Status,
			state.ID,
		)
	}
	if err := validateResolutionForRequirement(command.Resolution, state.PendingRequirement); err != nil {
		return err
	}
	return nil
}

func validateEngineOutput(prev RunState, next RunState) error {
	if next.ID != prev.ID {
		return fmt.Errorf(
			"%w: invariant=run_id input=%q output=%q",
			ErrEngineOutputContractViolation,
			prev.ID,
			next.ID,
		)
	}
	if next.Step < prev.Step {
		return fmt.Errorf(
			"%w: invariant=step input=%d output=%d run_id=%q",
			ErrEngineOutputContractViolation,
			prev.Step,
			next.Step,
			prev.ID,
		)
	}
	if len(next.Messages) < len(prev.Messages) {
		return fmt.Errorf(
			"%w: invariant=messages_length input=%d output=%d run_id=%q",
			ErrEngineOutputContractViolation,
			len(prev.Messages),
			len(next.Messages),
			prev.ID,
		)
	}
	if !reflect.DeepEqual(next.Messages[:len(prev.Messages)], prev.Messages) {
		return fmt.Errorf(
			"%w: invariant=messages_prefix run_id=%q",
			ErrEngineOutputContractViolation,
			prev.ID,
		)
	}
	if err := validateSuspendedRequirementProvenance(prev, next); err != nil {
		return err
	}
	return nil
}

func validateSuspendedRequirementProvenance(prev RunState, next RunState) error {
	if next.Status != RunStatusSuspended {
		return nil
	}
	if next.PendingRequirement == nil {
		return nil
	}

	additions := next.Messages[len(prev.Messages):]
	if len(additions) == 0 {
		return fmt.Errorf(
			"%w: invariant=suspension_origin_provenance reason=no_message_evidence origin=%s run_id=%q",
			ErrEngineOutputContractViolation,
			next.PendingRequirement.Origin,
			next.ID,
		)
	}

	switch next.PendingRequirement.Origin {
	case RequirementOriginModel:
		if !hasMatchingAssistantRequirement(additions, next.PendingRequirement) {
			return fmt.Errorf(
				"%w: invariant=suspension_origin_provenance reason=missing_assistant_requirement origin=%s requirement_id=%q run_id=%q",
				ErrEngineOutputContractViolation,
				next.PendingRequirement.Origin,
				next.PendingRequirement.ID,
				next.ID,
			)
		}
	case RequirementOriginTool:
		if !hasLinkedToolObservation(additions) {
			return fmt.Errorf(
				"%w: invariant=suspension_origin_provenance reason=missing_linked_tool_observation origin=%s requirement_id=%q run_id=%q",
				ErrEngineOutputContractViolation,
				next.PendingRequirement.Origin,
				next.PendingRequirement.ID,
				next.ID,
			)
		}
	default:
		return fmt.Errorf(
			"%w: invariant=suspension_origin_provenance reason=unknown_origin origin=%q run_id=%q",
			ErrEngineOutputContractViolation,
			next.PendingRequirement.Origin,
			next.ID,
		)
	}

	return nil
}

func hasMatchingAssistantRequirement(messages []Message, requirement *PendingRequirement) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != RoleAssistant || message.Requirement == nil {
			continue
		}
		current := message.Requirement
		if current.ID == requirement.ID &&
			current.Kind == requirement.Kind &&
			current.Origin == requirement.Origin {
			return true
		}
	}
	return false
}

func hasLinkedToolObservation(messages []Message) bool {
	assistantCallsByID := make(map[string]string)
	for _, message := range messages {
		switch message.Role {
		case RoleAssistant:
			for _, toolCall := range message.ToolCalls {
				if toolCall.ID == "" {
					continue
				}
				assistantCallsByID[toolCall.ID] = toolCall.Name
			}
		case RoleTool:
			if message.ToolCallID == "" {
				continue
			}
			toolName, exists := assistantCallsByID[message.ToolCallID]
			if !exists {
				continue
			}
			if message.Name != "" && toolName != "" && message.Name != toolName {
				continue
			}
			return true
		}
	}
	return false
}

func validateToolDefinitions(command CommandKind, tools []ToolDefinition) error {
	seen := make(map[string]struct{}, len(tools))
	for i := range tools {
		name := tools[i].Name
		if name == "" {
			return fmt.Errorf(
				"%w: command=%s index=%d reason=empty_name",
				ErrToolDefinitionsInvalid,
				command,
				i,
			)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf(
				"%w: command=%s index=%d name=%q reason=duplicate_name",
				ErrToolDefinitionsInvalid,
				command,
				i,
				name,
			)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func normalizeCommandSaveError(command CommandKind, err error) error {
	if !errors.Is(err, ErrRunVersionConflict) {
		return err
	}
	return errors.Join(
		ErrCommandConflict,
		fmt.Errorf("command=%s: %w", command, err),
	)
}

func sideEffectContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if ctx.Err() != nil {
		return context.WithoutCancel(ctx)
	}
	return ctx
}

// Dispatch executes a typed command against the run store.
func (r *Runner) Dispatch(ctx context.Context, cmd Command) (RunResult, error) {
	if ctx == nil {
		return RunResult{}, ErrContextNil
	}
	if isNilCommand(cmd) {
		return RunResult{}, ErrCommandNil
	}
	if reflect.ValueOf(cmd).Kind() == reflect.Pointer {
		return RunResult{}, fmt.Errorf("%w: kind=%s payload=%T", ErrCommandInvalid, cmd.Kind(), cmd)
	}

	switch command := cmd.(type) {
	case StartCommand:
		return r.dispatchStart(ctx, command)
	case ContinueCommand:
		return r.dispatchContinue(ctx, command)
	case CancelCommand:
		return r.dispatchCancel(ctx, command)
	case SteerCommand:
		return r.dispatchSteer(ctx, command)
	case FollowUpCommand:
		return r.dispatchFollowUp(ctx, command)
	default:
		switch kind := cmd.Kind(); kind {
		case CommandKindStart, CommandKindContinue, CommandKindCancel, CommandKindSteer, CommandKindFollowUp:
			return RunResult{}, fmt.Errorf("%w: kind=%s payload=%T", ErrCommandInvalid, kind, cmd)
		default:
			return RunResult{}, fmt.Errorf("%w: %s", ErrCommandUnsupported, kind)
		}
	}
}

func isNilCommand(cmd Command) bool {
	if cmd == nil {
		return true
	}

	value := reflect.ValueOf(cmd)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

// Run executes a new run from prompts and returns final state.
func (r *Runner) Run(ctx context.Context, input RunInput) (RunResult, error) {
	return r.Dispatch(ctx, StartCommand{Input: input})
}

func (r *Runner) dispatchStart(ctx context.Context, cmd StartCommand) (RunResult, error) {
	input := cmd.Input
	if err := validateToolDefinitions(CommandKindStart, input.Tools); err != nil {
		return RunResult{}, err
	}
	runID := input.RunID
	if runID == "" {
		generated, err := r.idGen.NewRunID(ctx)
		if err != nil {
			return RunResult{}, err
		}
		runID = generated
		if runID == "" {
			return RunResult{}, fmt.Errorf("%w: command=%s", ErrInvalidRunID, CommandKindStart)
		}
	}

	state := RunState{
		ID: runID,
	}
	if err := TransitionRunStatus(&state, RunStatusPending); err != nil {
		return RunResult{}, err
	}
	if input.SystemPrompt != "" {
		state.Messages = append(state.Messages, Message{
			Role:    RoleSystem,
			Content: input.SystemPrompt,
		})
	}
	if input.UserPrompt != "" {
		state.Messages = append(state.Messages, Message{
			Role:    RoleUser,
			Content: input.UserPrompt,
		})
	}

	sideEffectCtx := func() context.Context { return sideEffectContext(ctx) }

	if err := r.store.Save(sideEffectCtx(), state); err != nil {
		return RunResult{}, normalizeCommandSaveError(CommandKindStart, err)
	}
	state.Version++
	var eventErr error
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       runID,
		Step:        0,
		Type:        EventTypeRunStarted,
		Description: "run persisted and ready for execution",
	}))

	finalState, runErr := r.engine.Execute(ctx, state, EngineInput{
		MaxSteps:   input.MaxSteps,
		Tools:      CloneToolDefinitions(input.Tools),
		Resolution: nil,
	})
	if contractErr := validateEngineOutput(state, finalState); contractErr != nil {
		return RunResult{}, errors.Join(contractErr, eventErr)
	}

	if saveErr := r.store.Save(sideEffectCtx(), finalState); saveErr != nil {
		saveErr = normalizeCommandSaveError(CommandKindStart, saveErr)
		return RunResult{}, errors.Join(runErr, saveErr, eventErr)
	}
	if finalState.Status == RunStatusCancelled {
		eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
			RunID:       runID,
			Step:        finalState.Step,
			Type:        EventTypeRunCancelled,
			Description: cancellationEventDescription(runErr),
		}))
	}
	if finalState.Status == RunStatusSuspended {
		eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
			RunID:       runID,
			Step:        finalState.Step,
			Type:        EventTypeRunSuspended,
			Description: suspensionEventDescription(finalState),
		}))
	}
	finalState.Version++
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       runID,
		Step:        finalState.Step,
		Type:        EventTypeRunCheckpoint,
		Description: "final state persisted",
	}))
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       runID,
		Step:        finalState.Step,
		Type:        EventTypeCommandApplied,
		CommandKind: CommandKindStart,
		Description: "start command applied",
	}))
	return RunResult{State: finalState}, errors.Join(runErr, eventErr)
}

// Continue loads an existing run and executes additional engine steps.
func (r *Runner) Continue(
	ctx context.Context,
	runID RunID,
	maxSteps int,
	tools []ToolDefinition,
	resolution *Resolution,
) (RunResult, error) {
	return r.Dispatch(ctx, ContinueCommand{
		RunID:      runID,
		MaxSteps:   maxSteps,
		Tools:      tools,
		Resolution: resolution,
	})
}

func (r *Runner) dispatchContinue(ctx context.Context, cmd ContinueCommand) (RunResult, error) {
	runID := cmd.RunID
	if runID == "" {
		return RunResult{}, fmt.Errorf("%w: command=%s", ErrInvalidRunID, CommandKindContinue)
	}
	if err := validateToolDefinitions(CommandKindContinue, cmd.Tools); err != nil {
		return RunResult{}, err
	}
	sideEffectCtx := func() context.Context { return sideEffectContext(ctx) }
	state, err := r.store.Load(sideEffectCtx(), runID)
	if err != nil {
		return RunResult{}, err
	}
	if isTerminalRunStatus(state.Status) {
		return RunResult{State: state}, fmt.Errorf("%w: %s", ErrRunNotContinuable, state.Status)
	}
	if err := validateContinueResolution(state, cmd); err != nil {
		return RunResult{State: state}, err
	}
	if state.Status == RunStatusSuspended {
		state.PendingRequirement = nil
		if err := TransitionRunStatus(&state, RunStatusRunning); err != nil {
			return RunResult{State: state}, err
		}
	}
	finalState, runErr := r.engine.Execute(ctx, state, EngineInput{
		MaxSteps:   cmd.MaxSteps,
		Tools:      CloneToolDefinitions(cmd.Tools),
		Resolution: cmd.Resolution,
	})
	var eventErr error
	if contractErr := validateEngineOutput(state, finalState); contractErr != nil {
		return RunResult{}, errors.Join(contractErr, eventErr)
	}
	if saveErr := r.store.Save(sideEffectCtx(), finalState); saveErr != nil {
		saveErr = normalizeCommandSaveError(CommandKindContinue, saveErr)
		return RunResult{}, errors.Join(runErr, saveErr, eventErr)
	}
	if finalState.Status == RunStatusCancelled {
		eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
			RunID:       runID,
			Step:        finalState.Step,
			Type:        EventTypeRunCancelled,
			Description: cancellationEventDescription(runErr),
		}))
	}
	if finalState.Status == RunStatusSuspended {
		eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
			RunID:       runID,
			Step:        finalState.Step,
			Type:        EventTypeRunSuspended,
			Description: suspensionEventDescription(finalState),
		}))
	}
	finalState.Version++
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       runID,
		Step:        finalState.Step,
		Type:        EventTypeRunCheckpoint,
		Description: "continued run state persisted",
	}))
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       runID,
		Step:        finalState.Step,
		Type:        EventTypeCommandApplied,
		CommandKind: CommandKindContinue,
		Description: "continue command applied",
	}))
	return RunResult{State: finalState}, errors.Join(runErr, eventErr)
}

// Cancel marks a non-terminal run as cancelled and persists the cancellation state.
func (r *Runner) Cancel(ctx context.Context, runID RunID) (RunResult, error) {
	return r.Dispatch(ctx, CancelCommand{RunID: runID})
}

func (r *Runner) dispatchCancel(ctx context.Context, cmd CancelCommand) (RunResult, error) {
	runID := cmd.RunID
	if runID == "" {
		return RunResult{}, fmt.Errorf("%w: command=%s", ErrInvalidRunID, CommandKindCancel)
	}
	sideEffectCtx := func() context.Context { return sideEffectContext(ctx) }
	state, err := r.store.Load(sideEffectCtx(), runID)
	if err != nil {
		return RunResult{}, err
	}
	if isTerminalRunStatus(state.Status) {
		return RunResult{State: state}, fmt.Errorf("%w: %s", ErrRunNotCancellable, state.Status)
	}
	if state.Status == RunStatusSuspended {
		state.PendingRequirement = nil
	}
	if err := TransitionRunStatus(&state, RunStatusCancelled); err != nil {
		return RunResult{State: state}, err
	}
	if err := r.store.Save(sideEffectCtx(), state); err != nil {
		return RunResult{}, normalizeCommandSaveError(CommandKindCancel, err)
	}
	state.Version++
	var eventErr error
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       runID,
		Step:        state.Step,
		Type:        EventTypeRunCancelled,
		Description: "run cancelled",
	}))
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       runID,
		Step:        state.Step,
		Type:        EventTypeCommandApplied,
		CommandKind: CommandKindCancel,
		Description: "cancel command applied",
	}))
	return RunResult{State: state}, eventErr
}

// Steer appends a user instruction to a non-terminal run without engine execution.
func (r *Runner) Steer(ctx context.Context, runID RunID, instruction string) (RunResult, error) {
	return r.Dispatch(ctx, SteerCommand{
		RunID:       runID,
		Instruction: instruction,
	})
}

func (r *Runner) dispatchSteer(ctx context.Context, cmd SteerCommand) (RunResult, error) {
	if cmd.RunID == "" {
		return RunResult{}, fmt.Errorf("%w: command=%s", ErrInvalidRunID, CommandKindSteer)
	}
	sideEffectCtx := func() context.Context { return sideEffectContext(ctx) }
	state, err := r.store.Load(sideEffectCtx(), cmd.RunID)
	if err != nil {
		return RunResult{}, err
	}
	if state.Status == RunStatusSuspended {
		return RunResult{State: state}, fmt.Errorf(
			"%w: command=%s status=%s run_id=%q",
			ErrResolutionRequired,
			CommandKindSteer,
			state.Status,
			state.ID,
		)
	}
	if isTerminalRunStatus(state.Status) {
		return RunResult{State: state}, fmt.Errorf("%w: %s", ErrRunNotContinuable, state.Status)
	}
	state.Messages = append(state.Messages, Message{
		Role:    RoleUser,
		Content: cmd.Instruction,
	})
	if err := r.store.Save(sideEffectCtx(), state); err != nil {
		return RunResult{}, err
	}
	state.Version++
	var eventErr error
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       cmd.RunID,
		Step:        state.Step,
		Type:        EventTypeRunCheckpoint,
		Description: "steered run state persisted",
	}))
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       cmd.RunID,
		Step:        state.Step,
		Type:        EventTypeCommandApplied,
		CommandKind: CommandKindSteer,
		Description: "steer command applied",
	}))
	return RunResult{State: state}, eventErr
}

// FollowUp appends a user prompt to a non-terminal run and executes the engine.
func (r *Runner) FollowUp(ctx context.Context, runID RunID, prompt string, maxSteps int, tools []ToolDefinition) (RunResult, error) {
	return r.Dispatch(ctx, FollowUpCommand{
		RunID:      runID,
		UserPrompt: prompt,
		MaxSteps:   maxSteps,
		Tools:      tools,
	})
}

func (r *Runner) dispatchFollowUp(ctx context.Context, cmd FollowUpCommand) (RunResult, error) {
	if cmd.RunID == "" {
		return RunResult{}, fmt.Errorf("%w: command=%s", ErrInvalidRunID, CommandKindFollowUp)
	}
	if err := validateToolDefinitions(CommandKindFollowUp, cmd.Tools); err != nil {
		return RunResult{}, err
	}
	sideEffectCtx := func() context.Context { return sideEffectContext(ctx) }
	state, err := r.store.Load(sideEffectCtx(), cmd.RunID)
	if err != nil {
		return RunResult{}, err
	}
	if state.Status == RunStatusSuspended {
		return RunResult{State: state}, fmt.Errorf(
			"%w: command=%s status=%s run_id=%q",
			ErrResolutionRequired,
			CommandKindFollowUp,
			state.Status,
			state.ID,
		)
	}
	if isTerminalRunStatus(state.Status) {
		return RunResult{State: state}, fmt.Errorf("%w: %s", ErrRunNotContinuable, state.Status)
	}
	state.Messages = append(state.Messages, Message{
		Role:    RoleUser,
		Content: cmd.UserPrompt,
	})
	finalState, runErr := r.engine.Execute(ctx, state, EngineInput{
		MaxSteps:   cmd.MaxSteps,
		Tools:      CloneToolDefinitions(cmd.Tools),
		Resolution: nil,
	})
	var eventErr error
	if contractErr := validateEngineOutput(state, finalState); contractErr != nil {
		return RunResult{}, errors.Join(contractErr, eventErr)
	}
	if saveErr := r.store.Save(sideEffectCtx(), finalState); saveErr != nil {
		saveErr = normalizeCommandSaveError(CommandKindFollowUp, saveErr)
		return RunResult{}, errors.Join(runErr, saveErr, eventErr)
	}
	if finalState.Status == RunStatusCancelled {
		eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
			RunID:       cmd.RunID,
			Step:        finalState.Step,
			Type:        EventTypeRunCancelled,
			Description: cancellationEventDescription(runErr),
		}))
	}
	if finalState.Status == RunStatusSuspended {
		eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
			RunID:       cmd.RunID,
			Step:        finalState.Step,
			Type:        EventTypeRunSuspended,
			Description: suspensionEventDescription(finalState),
		}))
	}
	finalState.Version++
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       cmd.RunID,
		Step:        finalState.Step,
		Type:        EventTypeRunCheckpoint,
		Description: "follow-up run state persisted",
	}))
	eventErr = errors.Join(eventErr, publishEvent(sideEffectCtx(), r.events, Event{
		RunID:       cmd.RunID,
		Step:        finalState.Step,
		Type:        EventTypeCommandApplied,
		CommandKind: CommandKindFollowUp,
		Description: "follow-up command applied",
	}))
	return RunResult{State: finalState}, errors.Join(runErr, eventErr)
}
