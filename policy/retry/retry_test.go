package retry

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"agentruntime/agent"
)

type engineFunc func(context.Context, agent.RunState, agent.EngineInput) (agent.RunState, error)

func (f engineFunc) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	return f(ctx, state, input)
}

func TestWrapEngine_FailTwiceThenSucceed(t *testing.T) {
	t.Parallel()

	attempts := 0
	initial := agent.RunState{
		ID:     "run-1",
		Step:   4,
		Status: agent.RunStatusRunning,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}
	engine := engineFunc(func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
		attempts++
		if state.Messages[0].Content != "seed" {
			t.Fatalf("attempt %d received mutated state content: %q", attempts, state.Messages[0].Content)
		}
		state.Messages[0].Content = fmt.Sprintf("attempt-%d", attempts)
		if attempts < 3 {
			return state, fmt.Errorf("attempt %d failed", attempts)
		}
		next := state
		next.Step++
		next.Status = agent.RunStatusCompleted
		next.Output = "ok"
		return next, nil
	})

	wrapped := WrapEngine(engine, Config{MaxAttempts: 3})
	gotState, err := wrapped.Execute(context.Background(), initial, agent.EngineInput{})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
	if gotState.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", gotState.Status)
	}
	if gotState.Step != initial.Step+1 {
		t.Fatalf("unexpected step: got=%d want=%d", gotState.Step, initial.Step+1)
	}
	if gotState.Output != "ok" {
		t.Fatalf("unexpected output: %q", gotState.Output)
	}
	if initial.Messages[0].Content != "seed" {
		t.Fatalf("wrapper should preserve input state snapshot, got %q", initial.Messages[0].Content)
	}
}

func TestWrapEngine_AlwaysFailReturnsLastError(t *testing.T) {
	t.Parallel()

	attempts := 0
	var lastErr error
	engine := engineFunc(func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
		attempts++
		state.Step = attempts
		lastErr = fmt.Errorf("attempt %d failed", attempts)
		return state, lastErr
	})

	initial := agent.RunState{
		ID:     "run-always-fail",
		Status: agent.RunStatusRunning,
	}
	wrapped := WrapEngine(engine, Config{MaxAttempts: 4})
	gotState, err := wrapped.Execute(context.Background(), initial, agent.EngineInput{})
	if !errors.Is(err, lastErr) {
		t.Fatalf("expected last error %v, got %v", lastErr, err)
	}
	if attempts != 4 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
	if gotState.Step != 4 {
		t.Fatalf("unexpected state from last attempt: %d", gotState.Step)
	}
}

func TestWrapEngine_ShouldRetryFalseStopsAfterFirstError(t *testing.T) {
	t.Parallel()

	attempts := 0
	engine := engineFunc(func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
		attempts++
		state.Step = 9
		return state, errors.New("retryable")
	})

	wrapped := WrapEngine(engine, Config{
		MaxAttempts: 5,
		ShouldRetry: func(error) bool {
			return false
		},
	})
	gotState, err := wrapped.Execute(context.Background(), agent.RunState{ID: "run"}, agent.EngineInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
	if gotState.Step != 9 {
		t.Fatalf("unexpected state from first attempt: %d", gotState.Step)
	}
}

func TestWrapEngine_ContextErrorsDoNotRetryByDefault(t *testing.T) {
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
			engine := engineFunc(func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
				attempts++
				state.Step = 3
				return state, tc.err
			})
			wrapped := WrapEngine(engine, Config{MaxAttempts: 5})

			gotState, err := wrapped.Execute(context.Background(), agent.RunState{ID: "run"}, agent.EngineInput{})
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
			if attempts != 1 {
				t.Fatalf("unexpected attempts: %d", attempts)
			}
			if gotState.Step != 3 {
				t.Fatalf("unexpected state returned: %d", gotState.Step)
			}
		})
	}
}

func TestWrapEngine_ContextDoneStopsWithoutAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	engine := engineFunc(func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
		attempts++
		return state, errors.New("unexpected call")
	})
	wrapped := WrapEngine(engine, Config{MaxAttempts: 5})

	initial := agent.RunState{
		ID:     "run-pre-canceled",
		Status: agent.RunStatusRunning,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	gotState, err := wrapped.Execute(ctx, initial, agent.EngineInput{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if attempts != 0 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
	if !reflect.DeepEqual(gotState, initial) {
		t.Fatalf("state changed for pre-done context: got=%+v want=%+v", gotState, initial)
	}
}

func TestWrapEngine_NilContextStopsWithoutAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	engine := engineFunc(func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
		attempts++
		return state, errors.New("unexpected call")
	})
	wrapped := WrapEngine(engine, Config{MaxAttempts: 5})

	initial := agent.RunState{
		ID:     "run-nil-context",
		Status: agent.RunStatusRunning,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}
	gotState, err := wrapped.Execute(nil, initial, agent.EngineInput{})
	if !errors.Is(err, agent.ErrContextNil) {
		t.Fatalf("expected ErrContextNil, got %v", err)
	}
	if attempts != 0 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
	if !reflect.DeepEqual(gotState, initial) {
		t.Fatalf("state changed for nil context: got=%+v want=%+v", gotState, initial)
	}
}
