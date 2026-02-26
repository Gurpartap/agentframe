package retry

import (
	"context"
	"errors"

	"github.com/Gurpartap/agentframe/agent"
)

// Config controls retry behavior for wrapped engine execution calls.
type Config struct {
	MaxAttempts int
	ShouldRetry func(error) bool
}

// WrapEngine wraps an engine with deterministic, error-only retries.
func WrapEngine(engine agent.Engine, cfg Config) agent.Engine {
	if engine == nil {
		return nil
	}
	return &engineWrapper{
		next: engine,
		cfg:  cfg,
	}
}

type engineWrapper struct {
	next agent.Engine
	cfg  Config
}

func (w *engineWrapper) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	if ctx == nil {
		return state, agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return state, ctxErr
	}

	attempts := normalizedAttempts(w.cfg.MaxAttempts)
	baseState := agent.CloneRunState(state)
	baseInput := agent.EngineInput{
		MaxSteps: input.MaxSteps,
		Tools:    agent.CloneToolDefinitions(input.Tools),
	}
	lastState := baseState
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptState := agent.CloneRunState(baseState)
		attemptInput := agent.EngineInput{
			MaxSteps: baseInput.MaxSteps,
			Tools:    agent.CloneToolDefinitions(baseInput.Tools),
		}
		nextState, err := w.next.Execute(ctx, attemptState, attemptInput)
		if err == nil {
			return nextState, nil
		}
		lastState = nextState
		lastErr = err
		if attempt == attempts || !shouldRetry(ctx, w.cfg, err) {
			break
		}
	}
	return lastState, lastErr
}

func normalizedAttempts(maxAttempts int) int {
	if maxAttempts < 1 {
		return 1
	}
	return maxAttempts
}

func shouldRetry(ctx context.Context, cfg Config, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if cfg.ShouldRetry == nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return true
	}
	return cfg.ShouldRetry(err)
}
