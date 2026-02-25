package agentreact

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"agentruntime/agent"
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

func (l *ReactLoop) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	if ctx == nil {
		return state, agent.ErrContextNil
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
	for state.Step < maxSteps {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return l.cancelRun(ctx, state, ctxErr, eventErr)
		}

		state.Step++

		assistant, err := l.model.Generate(ctx, ModelRequest{
			Messages: agent.CloneMessages(state.Messages),
			Tools:    cloneToolDefinitions(input.Tools),
		})
		if err != nil {
			if cancellationErr := contextCancellationError(ctx, err); cancellationErr != nil {
				return l.cancelRun(ctx, state, cancellationErr, eventErr)
			}
			if transitionErr := agent.TransitionRunStatus(&state, agent.RunStatusFailed); transitionErr != nil {
				return state, errors.Join(err, transitionErr, eventErr)
			}
			state.Error = err.Error()
			eventErr = errors.Join(eventErr, publishEvent(ctx, l.events, agent.Event{
				RunID:       state.ID,
				Step:        state.Step,
				Type:        agent.EventTypeRunFailed,
				Description: fmt.Sprintf("model error: %v", err),
			}))
			return state, errors.Join(err, eventErr)
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
					result = normalizedToolErrorResult(toolCall, agent.ToolFailureReasonExecutorError, toolErr)
				} else {
					result = executed
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

func cloneToolDefinitions(in []agent.ToolDefinition) []agent.ToolDefinition {
	out := make([]agent.ToolDefinition, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].InputSchema != nil {
			out[i].InputSchema = make(map[string]any, len(in[i].InputSchema))
			maps.Copy(out[i].InputSchema, in[i].InputSchema)
		}
	}
	return out
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
