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
	model  agent.Model
	tools  agent.ToolExecutor
	events agent.EventSink
}

func New(model agent.Model, tools agent.ToolExecutor, events agent.EventSink) (*ReactLoop, error) {
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

func (l *ReactLoop) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	maxSteps := input.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	toolDefinitions := indexToolDefinitions(input.Tools)

	if err := transitionRunStatus(&state, agent.RunStatusRunning); err != nil {
		return state, err
	}
	for state.Step < maxSteps {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return l.cancelRun(ctx, state, ctxErr)
		}

		state.Step++

		assistant, err := l.model.Generate(ctx, agent.ModelRequest{
			Messages: agent.CloneMessages(state.Messages),
			Tools:    cloneToolDefinitions(input.Tools),
		})
		if err != nil {
			if cancellationErr := contextCancellationError(ctx, err); cancellationErr != nil {
				return l.cancelRun(ctx, state, cancellationErr)
			}
			if transitionErr := transitionRunStatus(&state, agent.RunStatusFailed); transitionErr != nil {
				return state, errors.Join(err, transitionErr)
			}
			state.Error = err.Error()
			_ = l.events.Publish(ctx, agent.Event{
				RunID:       state.ID,
				Step:        state.Step,
				Type:        agent.EventTypeRunFailed,
				Description: fmt.Sprintf("model error: %v", err),
			})
			return state, err
		}
		if assistant.Role == "" {
			assistant.Role = agent.RoleAssistant
		}
		state.Messages = append(state.Messages, agent.CloneMessage(assistant))
		_ = l.events.Publish(ctx, agent.Event{
			RunID:   state.ID,
			Step:    state.Step,
			Type:    agent.EventTypeAssistantMessage,
			Message: &assistant,
		})

		if len(assistant.ToolCalls) == 0 {
			if err := transitionRunStatus(&state, agent.RunStatusCompleted); err != nil {
				return state, err
			}
			state.Output = assistant.Content
			_ = l.events.Publish(ctx, agent.Event{
				RunID:       state.ID,
				Step:        state.Step,
				Type:        agent.EventTypeRunCompleted,
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
						return l.cancelRun(ctx, state, cancellationErr)
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
			_ = l.events.Publish(ctx, agent.Event{
				RunID:      state.ID,
				Step:       state.Step,
				Type:       agent.EventTypeToolResult,
				ToolResult: &resultCopy,
			})
		}
	}

	if err := transitionRunStatus(&state, agent.RunStatusMaxStepsExceeded); err != nil {
		return state, errors.Join(agent.ErrMaxStepsExceeded, err)
	}
	state.Error = agent.ErrMaxStepsExceeded.Error()
	_ = l.events.Publish(ctx, agent.Event{
		RunID:       state.ID,
		Step:        state.Step,
		Type:        agent.EventTypeRunFailed,
		Description: agent.ErrMaxStepsExceeded.Error(),
	})
	return state, agent.ErrMaxStepsExceeded
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

func (l *ReactLoop) cancelRun(ctx context.Context, state agent.RunState, runErr error) (agent.RunState, error) {
	if runErr == nil {
		runErr = context.Canceled
	}
	if transitionErr := transitionRunStatus(&state, agent.RunStatusCancelled); transitionErr != nil {
		return state, errors.Join(runErr, transitionErr)
	}
	state.Error = runErr.Error()
	_ = l.events.Publish(ctx, agent.Event{
		RunID:       state.ID,
		Step:        state.Step,
		Type:        agent.EventTypeRunCancelled,
		Description: runErr.Error(),
	})
	return state, runErr
}
