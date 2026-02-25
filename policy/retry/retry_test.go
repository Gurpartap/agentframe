package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"agentruntime/agent"
	"agentruntime/agentreact"
)

type modelFunc func(context.Context, agentreact.ModelRequest) (agent.Message, error)

func (f modelFunc) Generate(ctx context.Context, request agentreact.ModelRequest) (agent.Message, error) {
	return f(ctx, request)
}

type toolExecutorFunc func(context.Context, agent.ToolCall) (agent.ToolResult, error)

func (f toolExecutorFunc) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	return f(ctx, call)
}

func TestWrapModel_FailTwiceThenSucceed(t *testing.T) {
	t.Parallel()

	attempts := 0
	model := modelFunc(func(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
		attempts++
		if attempts < 3 {
			return agent.Message{}, fmt.Errorf("attempt %d failed", attempts)
		}
		return agent.Message{Role: agent.RoleAssistant, Content: "ok"}, nil
	})

	wrapped := WrapModel(model, Config{MaxAttempts: 3})
	msg, err := wrapped.Generate(context.Background(), agentreact.ModelRequest{})
	if err != nil {
		t.Fatalf("generate returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
	if msg.Content != "ok" {
		t.Fatalf("unexpected message content: %q", msg.Content)
	}
}

func TestWrapModel_AlwaysFailReturnsLastError(t *testing.T) {
	t.Parallel()

	attempts := 0
	var lastErr error
	model := modelFunc(func(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
		attempts++
		lastErr = fmt.Errorf("attempt %d failed", attempts)
		return agent.Message{}, lastErr
	})

	wrapped := WrapModel(model, Config{MaxAttempts: 3})
	_, err := wrapped.Generate(context.Background(), agentreact.ModelRequest{})
	if !errors.Is(err, lastErr) {
		t.Fatalf("expected last error %v, got %v", lastErr, err)
	}
	if attempts != 3 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestWrapToolExecutor_FailTwiceThenSucceed(t *testing.T) {
	t.Parallel()

	attempts := 0
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		attempts++
		if attempts < 3 {
			return agent.ToolResult{}, fmt.Errorf("attempt %d failed", attempts)
		}
		return agent.ToolResult{
			CallID:  "call-1",
			Name:    "lookup",
			Content: "ok",
		}, nil
	})

	wrapped := WrapToolExecutor(executor, Config{MaxAttempts: 3})
	result, err := wrapped.Execute(context.Background(), agent.ToolCall{ID: "call-1", Name: "lookup"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
	if result.Content != "ok" {
		t.Fatalf("unexpected result content: %q", result.Content)
	}
}

func TestWrapToolExecutor_AlwaysFailReturnsLastError(t *testing.T) {
	t.Parallel()

	attempts := 0
	var lastErr error
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		attempts++
		lastErr = fmt.Errorf("attempt %d failed", attempts)
		return agent.ToolResult{}, lastErr
	})

	wrapped := WrapToolExecutor(executor, Config{MaxAttempts: 4})
	_, err := wrapped.Execute(context.Background(), agent.ToolCall{ID: "call-1", Name: "lookup"})
	if !errors.Is(err, lastErr) {
		t.Fatalf("expected last error %v, got %v", lastErr, err)
	}
	if attempts != 4 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestWrapModel_ShouldRetryFalseStopsAfterFirstError(t *testing.T) {
	t.Parallel()

	attempts := 0
	model := modelFunc(func(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
		attempts++
		return agent.Message{}, errors.New("retryable")
	})

	wrapped := WrapModel(model, Config{
		MaxAttempts: 5,
		ShouldRetry: func(error) bool {
			return false
		},
	})
	_, err := wrapped.Generate(context.Background(), agentreact.ModelRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestWrapToolExecutor_ShouldRetryFalseStopsAfterFirstError(t *testing.T) {
	t.Parallel()

	attempts := 0
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		attempts++
		return agent.ToolResult{}, errors.New("retryable")
	})

	wrapped := WrapToolExecutor(executor, Config{
		MaxAttempts: 5,
		ShouldRetry: func(error) bool {
			return false
		},
	})
	_, err := wrapped.Execute(context.Background(), agent.ToolCall{ID: "call-1", Name: "lookup"})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestWrapModel_ContextErrorsDoNotRetryByDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "deadline_exceeded", err: context.DeadlineExceeded},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			attempts := 0
			model := modelFunc(func(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
				attempts++
				return agent.Message{}, tc.err
			})
			wrapped := WrapModel(model, Config{MaxAttempts: 5})

			_, err := wrapped.Generate(context.Background(), agentreact.ModelRequest{})
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
			if attempts != 1 {
				t.Fatalf("unexpected attempts: %d", attempts)
			}
		})
	}
}

func TestWrapToolExecutor_ContextErrorsDoNotRetryByDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "deadline_exceeded", err: context.DeadlineExceeded},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			attempts := 0
			executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
				attempts++
				return agent.ToolResult{}, tc.err
			})
			wrapped := WrapToolExecutor(executor, Config{MaxAttempts: 5})

			_, err := wrapped.Execute(context.Background(), agent.ToolCall{ID: "call-1", Name: "lookup"})
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
			if attempts != 1 {
				t.Fatalf("unexpected attempts: %d", attempts)
			}
		})
	}
}

func TestWrapModel_ContextDoneStopsWithoutAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	model := modelFunc(func(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
		attempts++
		return agent.Message{}, errors.New("unexpected call")
	})
	wrapped := WrapModel(model, Config{MaxAttempts: 5})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := wrapped.Generate(ctx, agentreact.ModelRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if attempts != 0 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestWrapToolExecutor_ContextDoneStopsWithoutAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		attempts++
		return agent.ToolResult{}, errors.New("unexpected call")
	})
	wrapped := WrapToolExecutor(executor, Config{MaxAttempts: 5})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := wrapped.Execute(ctx, agent.ToolCall{ID: "call-1", Name: "lookup"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if attempts != 0 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestWrapModel_NilContextStopsWithoutAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	model := modelFunc(func(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
		attempts++
		return agent.Message{}, errors.New("unexpected call")
	})
	wrapped := WrapModel(model, Config{MaxAttempts: 5})

	_, err := wrapped.Generate(nil, agentreact.ModelRequest{})
	if !errors.Is(err, agent.ErrContextNil) {
		t.Fatalf("expected ErrContextNil, got %v", err)
	}
	if attempts != 0 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestWrapToolExecutor_NilContextStopsWithoutAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		attempts++
		return agent.ToolResult{}, errors.New("unexpected call")
	})
	wrapped := WrapToolExecutor(executor, Config{MaxAttempts: 5})

	_, err := wrapped.Execute(nil, agent.ToolCall{ID: "call-1", Name: "lookup"})
	if !errors.Is(err, agent.ErrContextNil) {
		t.Fatalf("expected ErrContextNil, got %v", err)
	}
	if attempts != 0 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}
