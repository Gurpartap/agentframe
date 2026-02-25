package agent

import (
	"context"
	"errors"
	"fmt"
)

// Dependencies wires application services into the runtime orchestrator.
type Dependencies struct {
	IDGenerator IDGenerator
	RunStore    RunStore
	ReactLoop   *ReactLoop
	EventSink   EventSink
}

// Runner owns the run lifecycle and persistence.
type Runner struct {
	idGen  IDGenerator
	store  RunStore
	loop   *ReactLoop
	events EventSink
}

func NewRunner(deps Dependencies) (*Runner, error) {
	if deps.IDGenerator == nil {
		return nil, errors.New("id generator is required")
	}
	if deps.RunStore == nil {
		return nil, errors.New("run store is required")
	}
	if deps.ReactLoop == nil {
		return nil, errors.New("react loop is required")
	}
	if deps.EventSink == nil {
		deps.EventSink = noopEventSink{}
	}
	return &Runner{
		idGen:  deps.IDGenerator,
		store:  deps.RunStore,
		loop:   deps.ReactLoop,
		events: deps.EventSink,
	}, nil
}

// Run executes a new run from prompts and returns final state.
func (r *Runner) Run(ctx context.Context, input RunInput) (RunResult, error) {
	runID := input.RunID
	if runID == "" {
		generated, err := r.idGen.NewRunID(ctx)
		if err != nil {
			return RunResult{}, err
		}
		runID = generated
	}

	state := RunState{
		ID: runID,
	}
	if err := transitionRunStatus(&state, RunStatusPending); err != nil {
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

	if err := r.store.Save(ctx, state); err != nil {
		return RunResult{}, err
	}
	_ = r.events.Publish(ctx, Event{
		RunID:       runID,
		Step:        0,
		Type:        EventTypeRunStarted,
		Description: "run persisted and ready for execution",
	})

	finalState, runErr := r.loop.Execute(ctx, state, ReactConfig{
		MaxSteps: input.MaxSteps,
		Tools:    input.Tools,
	})

	if saveErr := r.store.Save(ctx, finalState); saveErr != nil {
		if runErr != nil {
			return RunResult{}, errors.Join(runErr, saveErr)
		}
		return RunResult{}, saveErr
	}
	_ = r.events.Publish(ctx, Event{
		RunID:       runID,
		Step:        finalState.Step,
		Type:        EventTypeRunCheckpoint,
		Description: "final state persisted",
	})
	return RunResult{State: finalState}, runErr
}

// Continue loads an existing run and executes additional ReAct steps.
func (r *Runner) Continue(ctx context.Context, runID RunID, maxSteps int, tools []ToolDefinition) (RunResult, error) {
	state, err := r.store.Load(ctx, runID)
	if err != nil {
		return RunResult{}, err
	}
	if isTerminalRunStatus(state.Status) {
		return RunResult{State: state}, fmt.Errorf("%w: %s", ErrRunNotContinuable, state.Status)
	}
	finalState, runErr := r.loop.Execute(ctx, state, ReactConfig{
		MaxSteps: maxSteps,
		Tools:    tools,
	})
	if saveErr := r.store.Save(ctx, finalState); saveErr != nil {
		if runErr != nil {
			return RunResult{}, errors.Join(runErr, saveErr)
		}
		return RunResult{}, saveErr
	}
	_ = r.events.Publish(ctx, Event{
		RunID:       runID,
		Step:        finalState.Step,
		Type:        EventTypeRunCheckpoint,
		Description: "continued run state persisted",
	})
	return RunResult{State: finalState}, runErr
}

// Cancel marks a non-terminal run as cancelled and persists the cancellation state.
func (r *Runner) Cancel(ctx context.Context, runID RunID) (RunResult, error) {
	state, err := r.store.Load(ctx, runID)
	if err != nil {
		return RunResult{}, err
	}
	if isTerminalRunStatus(state.Status) {
		return RunResult{State: state}, fmt.Errorf("%w: %s", ErrRunNotCancellable, state.Status)
	}
	if err := transitionRunStatus(&state, RunStatusCancelled); err != nil {
		return RunResult{State: state}, err
	}
	if err := r.store.Save(ctx, state); err != nil {
		return RunResult{}, err
	}
	_ = r.events.Publish(ctx, Event{
		RunID:       runID,
		Step:        state.Step,
		Type:        EventTypeRunCancelled,
		Description: "run cancelled",
	})
	return RunResult{State: state}, nil
}
