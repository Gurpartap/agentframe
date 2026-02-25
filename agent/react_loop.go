package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
)

const DefaultMaxSteps = 8

// ReactLoop executes a minimal ReAct sequence:
// model -> tool calls -> tool observations -> model -> ...
type ReactLoop struct {
	model  Model
	tools  ToolExecutor
	events EventSink
}

func NewReactLoop(model Model, tools ToolExecutor, events EventSink) (*ReactLoop, error) {
	if model == nil {
		return nil, errors.New("model is required")
	}
	if tools == nil {
		return nil, errors.New("tool executor is required")
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

func (l *ReactLoop) Execute(ctx context.Context, state RunState, input EngineInput) (RunState, error) {
	maxSteps := input.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	toolDefinitions := indexToolDefinitions(input.Tools)

	if err := transitionRunStatus(&state, RunStatusRunning); err != nil {
		return state, err
	}
	for state.Step < maxSteps {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return l.cancelRun(ctx, state, ctxErr)
		}

		state.Step++

		assistant, err := l.model.Generate(ctx, ModelRequest{
			Messages: CloneMessages(state.Messages),
			Tools:    cloneToolDefinitions(input.Tools),
		})
		if err != nil {
			if cancellationErr := contextCancellationError(ctx, err); cancellationErr != nil {
				return l.cancelRun(ctx, state, cancellationErr)
			}
			if transitionErr := transitionRunStatus(&state, RunStatusFailed); transitionErr != nil {
				return state, errors.Join(err, transitionErr)
			}
			state.Error = err.Error()
			_ = l.events.Publish(ctx, Event{
				RunID:       state.ID,
				Step:        state.Step,
				Type:        EventTypeRunFailed,
				Description: fmt.Sprintf("model error: %v", err),
			})
			return state, err
		}
		if assistant.Role == "" {
			assistant.Role = RoleAssistant
		}
		state.Messages = append(state.Messages, CloneMessage(assistant))
		_ = l.events.Publish(ctx, Event{
			RunID:   state.ID,
			Step:    state.Step,
			Type:    EventTypeAssistantMessage,
			Message: &assistant,
		})

		if len(assistant.ToolCalls) == 0 {
			if err := transitionRunStatus(&state, RunStatusCompleted); err != nil {
				return state, err
			}
			state.Output = assistant.Content
			_ = l.events.Publish(ctx, Event{
				RunID:       state.ID,
				Step:        state.Step,
				Type:        EventTypeRunCompleted,
				Description: "assistant returned a final answer",
			})
			return state, nil
		}

		for _, toolCall := range assistant.ToolCalls {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return l.cancelRun(ctx, state, ctxErr)
			}

			definition, defined := toolDefinitions[toolCall.Name]
			var validationErr error
			if defined {
				validationErr = validateToolCallArguments(toolCall, definition)
			}
			result := ToolResult{}
			switch {
			case !defined:
				result = normalizedToolErrorResult(
					toolCall,
					ToolFailureReasonUnknownTool,
					fmt.Errorf("tool %q is not defined", toolCall.Name),
				)
			case validationErr != nil:
				result = normalizedToolErrorResult(
					toolCall,
					ToolFailureReasonInvalidArguments,
					validationErr,
				)
			default:
				executed, toolErr := l.tools.Execute(ctx, toolCall)
				if toolErr != nil {
					if cancellationErr := contextCancellationError(ctx, toolErr); cancellationErr != nil {
						return l.cancelRun(ctx, state, cancellationErr)
					}
					result = normalizedToolErrorResult(toolCall, ToolFailureReasonExecutorError, toolErr)
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

			state.Messages = append(state.Messages, ToolResultMessage(result))
			resultCopy := result
			_ = l.events.Publish(ctx, Event{
				RunID:      state.ID,
				Step:       state.Step,
				Type:       EventTypeToolResult,
				ToolResult: &resultCopy,
			})
		}
	}

	if err := transitionRunStatus(&state, RunStatusMaxStepsExceeded); err != nil {
		return state, errors.Join(ErrMaxStepsExceeded, err)
	}
	state.Error = ErrMaxStepsExceeded.Error()
	_ = l.events.Publish(ctx, Event{
		RunID:       state.ID,
		Step:        state.Step,
		Type:        EventTypeRunFailed,
		Description: ErrMaxStepsExceeded.Error(),
	})
	return state, ErrMaxStepsExceeded
}

func cloneToolDefinitions(in []ToolDefinition) []ToolDefinition {
	out := make([]ToolDefinition, len(in))
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

func (noopEventSink) Publish(context.Context, Event) error {
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

func (l *ReactLoop) cancelRun(ctx context.Context, state RunState, runErr error) (RunState, error) {
	if runErr == nil {
		runErr = context.Canceled
	}
	if transitionErr := transitionRunStatus(&state, RunStatusCancelled); transitionErr != nil {
		return state, errors.Join(runErr, transitionErr)
	}
	state.Error = runErr.Error()
	_ = l.events.Publish(ctx, Event{
		RunID:       state.ID,
		Step:        state.Step,
		Type:        EventTypeRunCancelled,
		Description: runErr.Error(),
	})
	return state, runErr
}
