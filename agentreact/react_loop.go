package agentreact

import (
	"context"
	"errors"
	"fmt"

	"github.com/Gurpartap/agentframe/agent"
)

const DefaultMaxSteps = 8

// ReactLoop executes a minimal ReAct sequence:
// model -> tool calls -> tool observations -> model -> ...
type ReactLoop struct {
	model  Model
	tools  ToolExecutor
	events agent.EventSink
}

func New(model Model, tools ToolExecutor, events agent.EventSink) (*ReactLoop, error) {
	if model == nil {
		return nil, fmt.Errorf("new react loop: %w", ErrMissingModel)
	}
	if tools == nil {
		return nil, fmt.Errorf("new react loop: %w", ErrMissingToolExecutor)
	}
	if events == nil {
		events = noopEventSink{}
	}
	return &ReactLoop{
		model:  model,
		tools:  tools,
		events: events,
	}, nil
}

func publishEvent(ctx context.Context, sink agent.EventSink, event agent.Event) error {
	if err := agent.ValidateEvent(event); err != nil {
		return err
	}
	if err := sink.Publish(ctx, event); err != nil {
		return errors.Join(
			agent.ErrEventPublish,
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

func cloneResolution(in *agent.Resolution) *agent.Resolution {
	if in == nil {
		return nil
	}
	resolutionCopy := *in
	return &resolutionCopy
}

func resolutionMessage(resolution *agent.Resolution) agent.Message {
	if resolution == nil {
		return agent.Message{}
	}
	content := fmt.Sprintf(
		"[resolution] requirement_id=%q kind=%s outcome=%s",
		resolution.RequirementID,
		resolution.Kind,
		resolution.Outcome,
	)
	if resolution.Value != "" {
		content = fmt.Sprintf("%s value=%q", content, resolution.Value)
	}
	return agent.Message{
		Role:    agent.RoleUser,
		Content: content,
	}
}

func (l *ReactLoop) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	if ctx == nil {
		return state, agent.ErrContextNil
	}
	if err := agent.ValidateRunState(state); err != nil {
		return state, err
	}
	if state.Status == agent.RunStatusSuspended {
		return state, fmt.Errorf(
			"%w: run_id=%q reason=continue_requires_resolution",
			agent.ErrResolutionRequired,
			state.ID,
		)
	}

	maxSteps := input.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	toolDefinitions := indexToolDefinitions(input.Tools)
	var eventErr error

	if err := agent.TransitionRunStatus(&state, agent.RunStatusRunning); err != nil {
		return state, errors.Join(err, eventErr)
	}
	if input.Resolution != nil {
		state.Messages = append(state.Messages, resolutionMessage(input.Resolution))
	}
	for state.Step < maxSteps {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return l.cancelRun(ctx, state, ctxErr, eventErr)
		}

		state.Step++

		assistant, err := l.model.Generate(ctx, ModelRequest{
			Messages:   agent.CloneMessages(state.Messages),
			Tools:      agent.CloneToolDefinitions(input.Tools),
			Resolution: cloneResolution(input.Resolution),
		})
		if err != nil {
			if cancellationErr := contextCancellationError(ctx, err); cancellationErr != nil {
				return l.cancelRun(ctx, state, cancellationErr, eventErr)
			}
			return l.failRun(ctx, state, err, eventErr)
		}
		if assistant.Role == "" {
			assistant.Role = agent.RoleAssistant
		}
		state.Messages = append(state.Messages, agent.CloneMessage(assistant))
		eventErr = errors.Join(eventErr, publishEvent(ctx, l.events, agent.Event{
			RunID:   state.ID,
			Step:    state.Step,
			Type:    agent.EventTypeAssistantMessage,
			Message: &assistant,
		}))
		if assistant.Requirement != nil {
			if len(assistant.ToolCalls) > 0 {
				return l.failRun(
					ctx,
					state,
					fmt.Errorf("assistant response cannot include both requirement and tool calls"),
					eventErr,
				)
			}
			requirementCopy := *assistant.Requirement
			if err := validateModelRequirement(&state, &requirementCopy); err != nil {
				return l.failRun(ctx, state, err, eventErr)
			}
			state.PendingRequirement = &requirementCopy
			if err := agent.TransitionRunStatus(&state, agent.RunStatusSuspended); err != nil {
				state.PendingRequirement = nil
				return l.failRun(ctx, state, err, eventErr)
			}
			return state, eventErr
		}

		if len(assistant.ToolCalls) == 0 {
			if err := agent.TransitionRunStatus(&state, agent.RunStatusCompleted); err != nil {
				return state, errors.Join(err, eventErr)
			}
			state.Output = assistant.Content
			eventErr = errors.Join(eventErr, publishEvent(ctx, l.events, agent.Event{
				RunID:       state.ID,
				Step:        state.Step,
				Type:        agent.EventTypeRunCompleted,
				Description: "assistant returned a final answer",
			}))
			return state, eventErr
		}
		if err := validateToolCallShape(assistant.ToolCalls); err != nil {
			return l.failRun(ctx, state, err, eventErr)
		}

		for _, toolCall := range assistant.ToolCalls {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return l.cancelRun(ctx, state, ctxErr, eventErr)
			}

			definition, defined := toolDefinitions[toolCall.Name]
			var validationErr error
			if defined {
				validationErr = validateToolCallArguments(toolCall, definition)
			}
			result := agent.ToolResult{}
			var suspendRequestErr *agent.SuspendRequestError
			var invalidSuspendErr error
			switch {
			case !defined:
				result = normalizedToolErrorResult(
					toolCall,
					agent.ToolFailureReasonUnknownTool,
					fmt.Errorf("tool %q is not defined", toolCall.Name),
				)
			case validationErr != nil:
				result = normalizedToolErrorResult(
					toolCall,
					agent.ToolFailureReasonInvalidArguments,
					validationErr,
				)
			default:
				executed, toolErr := l.tools.Execute(ctx, toolCall)
				if toolErr != nil {
					if cancellationErr := contextCancellationError(ctx, toolErr); cancellationErr != nil {
						return l.cancelRun(ctx, state, cancellationErr, eventErr)
					}
					if errors.As(toolErr, &suspendRequestErr) {
						invalidSuspendErr = validateToolSuspendRequest(&state, suspendRequestErr)
						if invalidSuspendErr != nil {
							result = normalizedToolErrorResult(toolCall, agent.ToolFailureReasonExecutorError, invalidSuspendErr)
						} else {
							result = normalizedToolErrorResult(toolCall, agent.ToolFailureReasonSuspended, toolErr)
						}
					} else {
						result = normalizedToolErrorResult(toolCall, agent.ToolFailureReasonExecutorError, toolErr)
					}
				} else {
					if identityErr := validateToolResultIdentity(toolCall, executed); identityErr != nil {
						result = normalizedToolErrorResult(toolCall, agent.ToolFailureReasonExecutorError, identityErr)
					} else {
						result = executed
					}
				}
			}
			if result.CallID == "" {
				result.CallID = toolCall.ID
			}
			if result.Name == "" {
				result.Name = toolCall.Name
			}

			state.Messages = append(state.Messages, agent.ToolResultMessage(result))
			resultCopy := result
			eventErr = errors.Join(eventErr, publishEvent(ctx, l.events, agent.Event{
				RunID:      state.ID,
				Step:       state.Step,
				Type:       agent.EventTypeToolResult,
				ToolResult: &resultCopy,
			}))
			if invalidSuspendErr != nil {
				return l.failRun(ctx, state, invalidSuspendErr, eventErr)
			}
			if suspendRequestErr != nil {
				requirementCopy := *suspendRequestErr.Requirement
				state.PendingRequirement = &requirementCopy
				if err := agent.TransitionRunStatus(&state, agent.RunStatusSuspended); err != nil {
					state.PendingRequirement = nil
					return l.failRun(ctx, state, err, eventErr)
				}
				return state, eventErr
			}
		}
	}

	if err := agent.TransitionRunStatus(&state, agent.RunStatusMaxStepsExceeded); err != nil {
		return state, errors.Join(agent.ErrMaxStepsExceeded, err, eventErr)
	}
	state.Error = agent.ErrMaxStepsExceeded.Error()
	eventErr = errors.Join(eventErr, publishEvent(ctx, l.events, agent.Event{
		RunID:       state.ID,
		Step:        state.Step,
		Type:        agent.EventTypeRunFailed,
		Description: agent.ErrMaxStepsExceeded.Error(),
	}))
	return state, errors.Join(agent.ErrMaxStepsExceeded, eventErr)
}

type noopEventSink struct{}

func (noopEventSink) Publish(context.Context, agent.Event) error {
	return nil
}

func contextCancellationError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func validateToolResultIdentity(call agent.ToolCall, result agent.ToolResult) error {
	if result.CallID != "" && result.CallID != call.ID {
		return fmt.Errorf("tool result call id mismatch: got=%q want=%q", result.CallID, call.ID)
	}
	if result.Name != "" && result.Name != call.Name {
		return fmt.Errorf("tool result name mismatch: got=%q want=%q", result.Name, call.Name)
	}
	return nil
}

func validateModelRequirement(state *agent.RunState, requirement *agent.PendingRequirement) error {
	if requirement.Origin != agent.RequirementOriginModel {
		return fmt.Errorf(
			"%w: field=pending_requirement.origin reason=invalid_for_source source=model value=%q want=%q",
			agent.ErrRunStateInvalid,
			requirement.Origin,
			agent.RequirementOriginModel,
		)
	}
	return validateRequirementContract(state, requirement)
}

func validateToolSuspendRequest(state *agent.RunState, request *agent.SuspendRequestError) error {
	if request == nil || request.Requirement == nil {
		return fmt.Errorf(
			"%w: field=pending_requirement reason=nil source=tool",
			agent.ErrRunStateInvalid,
		)
	}
	if request.Requirement.Origin != agent.RequirementOriginTool {
		return fmt.Errorf(
			"%w: field=pending_requirement.origin reason=invalid_for_source source=tool value=%q want=%q",
			agent.ErrRunStateInvalid,
			request.Requirement.Origin,
			agent.RequirementOriginTool,
		)
	}
	return validateRequirementContract(state, request.Requirement)
}

func validateRequirementContract(state *agent.RunState, requirement *agent.PendingRequirement) error {
	if state == nil {
		return errors.New("requirement validation state is nil")
	}
	return agent.ValidateRunState(agent.RunState{
		ID:                 state.ID,
		Version:            state.Version,
		Step:               state.Step,
		Status:             agent.RunStatusSuspended,
		PendingRequirement: requirement,
	})
}

func validateToolCallShape(calls []agent.ToolCall) error {
	seen := make(map[string]int, len(calls))
	for i, call := range calls {
		if call.ID == "" {
			return fmt.Errorf("%w: index=%d reason=empty_id", ErrToolCallInvalid, i)
		}
		if call.Name == "" {
			return fmt.Errorf("%w: index=%d id=%q reason=empty_name", ErrToolCallInvalid, i, call.ID)
		}
		if firstIndex, exists := seen[call.ID]; exists {
			return fmt.Errorf(
				"%w: index=%d id=%q reason=duplicate_id first_index=%d",
				ErrToolCallInvalid,
				i,
				call.ID,
				firstIndex,
			)
		}
		seen[call.ID] = i
	}
	return nil
}

func (l *ReactLoop) failRun(ctx context.Context, state agent.RunState, runErr error, eventErr error) (agent.RunState, error) {
	if runErr == nil {
		runErr = errors.New("run failed")
	}
	if transitionErr := agent.TransitionRunStatus(&state, agent.RunStatusFailed); transitionErr != nil {
		return state, errors.Join(runErr, transitionErr, eventErr)
	}
	state.Error = runErr.Error()
	eventErr = errors.Join(eventErr, publishEvent(ctx, l.events, agent.Event{
		RunID:       state.ID,
		Step:        state.Step,
		Type:        agent.EventTypeRunFailed,
		Description: fmt.Sprintf("model error: %v", runErr),
	}))
	return state, errors.Join(runErr, eventErr)
}

func (l *ReactLoop) cancelRun(ctx context.Context, state agent.RunState, runErr error, eventErr error) (agent.RunState, error) {
	if runErr == nil {
		runErr = context.Canceled
	}
	if transitionErr := agent.TransitionRunStatus(&state, agent.RunStatusCancelled); transitionErr != nil {
		return state, errors.Join(runErr, transitionErr, eventErr)
	}
	state.Error = runErr.Error()
	eventErr = errors.Join(eventErr, publishEvent(ctx, l.events, agent.Event{
		RunID:       state.ID,
		Step:        state.Step,
		Type:        agent.EventTypeRunCancelled,
		Description: runErr.Error(),
	}))
	return state, errors.Join(runErr, eventErr)
}
