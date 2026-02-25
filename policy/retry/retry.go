package retry

import (
	"context"
	"errors"

	"agentruntime/agent"
	"agentruntime/agentreact"
)

// Config controls retry behavior for wrapped model and tool execution calls.
type Config struct {
	MaxAttempts int
	ShouldRetry func(error) bool
}

// WrapModel wraps a model with deterministic, error-only retries.
func WrapModel(model agentreact.Model, cfg Config) agentreact.Model {
	if model == nil {
		return nil
	}
	return &modelWrapper{
		next: model,
		cfg:  cfg,
	}
}

type modelWrapper struct {
	next agentreact.Model
	cfg  Config
}

func (w *modelWrapper) Generate(ctx context.Context, request agentreact.ModelRequest) (agent.Message, error) {
	if ctx == nil {
		return agent.Message{}, agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.Message{}, ctxErr
	}

	attempts := normalizedAttempts(w.cfg.MaxAttempts)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		msg, err := w.next.Generate(ctx, request)
		if err == nil {
			return msg, nil
		}
		lastErr = err
		if attempt == attempts || !shouldRetry(ctx, w.cfg, err) {
			break
		}
	}
	return agent.Message{}, lastErr
}

// WrapToolExecutor wraps a tool executor with deterministic, error-only retries.
func WrapToolExecutor(executor agentreact.ToolExecutor, cfg Config) agentreact.ToolExecutor {
	if executor == nil {
		return nil
	}
	return &toolExecutorWrapper{
		next: executor,
		cfg:  cfg,
	}
}

type toolExecutorWrapper struct {
	next agentreact.ToolExecutor
	cfg  Config
}

func (w *toolExecutorWrapper) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	if ctx == nil {
		return agent.ToolResult{}, agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.ToolResult{}, ctxErr
	}

	attempts := normalizedAttempts(w.cfg.MaxAttempts)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := w.next.Execute(ctx, call)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt == attempts || !shouldRetry(ctx, w.cfg, err) {
			break
		}
	}
	return agent.ToolResult{}, lastErr
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
