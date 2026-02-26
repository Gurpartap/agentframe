package registry_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	toolingregistry "github.com/Gurpartap/agentframe/tooling/registry"
)

func TestRegistryNew_RejectsInvalidInitialEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial map[string]toolingregistry.Handler
		wantErr error
	}{
		{
			name: "empty_name",
			initial: map[string]toolingregistry.Handler{
				"": func(_ context.Context, _ map[string]any) (string, error) {
					return "unexpected", nil
				},
			},
			wantErr: toolingregistry.ErrToolNameEmpty,
		},
		{
			name: "nil_handler",
			initial: map[string]toolingregistry.Handler{
				"lookup": nil,
			},
			wantErr: toolingregistry.ErrNilHandler,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registry, err := toolingregistry.New(tc.initial)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if registry != nil {
				t.Fatalf("expected nil registry on constructor rejection")
			}
		})
	}
}

func TestRegistryExecute_UnknownToolReturnsError(t *testing.T) {
	t.Parallel()

	registry := mustNewRegistry(t, nil)
	result, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-1", Name: "missing"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, toolingregistry.ErrToolUnregistered) {
		t.Fatalf("expected ErrToolUnregistered, got %v", err)
	}
	if !strings.Contains(err.Error(), `"missing"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegistryExecute_EmptyToolNameReturnsError(t *testing.T) {
	t.Parallel()

	registry := mustNewRegistry(t, nil)
	result, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-empty"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, toolingregistry.ErrToolNameEmpty) {
		t.Fatalf("expected ErrToolNameEmpty, got %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegistryExecute_EmptyToolNameFailsBeforeHandlerLookup(t *testing.T) {
	t.Parallel()

	called := false
	registry := mustNewRegistry(t, map[string]toolingregistry.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			called = true
			return "should-not-run", nil
		},
	})

	_, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-empty", Name: ""})
	if !errors.Is(err, toolingregistry.ErrToolNameEmpty) {
		t.Fatalf("expected ErrToolNameEmpty, got %v", err)
	}
	if called {
		t.Fatalf("handler must not be invoked for empty tool name")
	}
}

func TestRegistryExecute_NormalizesResult(t *testing.T) {
	t.Parallel()

	registry := mustNewRegistry(t, map[string]toolingregistry.Handler{
		"lookup": func(_ context.Context, arguments map[string]any) (string, error) {
			if got, ok := arguments["query"].(string); !ok || got != "weather" {
				t.Fatalf("unexpected arguments: %+v", arguments)
			}
			return "sunny", nil
		},
	})

	result, err := registry.Execute(context.Background(), agent.ToolCall{
		ID:   "call-42",
		Name: "lookup",
		Arguments: map[string]any{
			"query": "weather",
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.CallID != "call-42" {
		t.Fatalf("unexpected call id: %s", result.CallID)
	}
	if result.Name != "lookup" {
		t.Fatalf("unexpected name: %s", result.Name)
	}
	if result.Content != "sunny" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
}

func TestRegistryRegister_AddsHandler(t *testing.T) {
	t.Parallel()

	registry := mustNewRegistry(t, nil)
	if err := registry.Register("ping", func(_ context.Context, _ map[string]any) (string, error) {
		return "pong", nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	result, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-7", Name: "ping"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Content != "pong" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
}

func TestRegistryRegister_RejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	registry := mustNewRegistry(t, map[string]toolingregistry.Handler{
		"ping": func(_ context.Context, _ map[string]any) (string, error) {
			return "pong", nil
		},
	})

	tests := []struct {
		name    string
		handler toolingregistry.Handler
		tool    string
		wantErr error
	}{
		{
			name: "empty_name",
			tool: "",
			handler: func(_ context.Context, _ map[string]any) (string, error) {
				return "unexpected", nil
			},
			wantErr: toolingregistry.ErrToolNameEmpty,
		},
		{
			name:    "nil_handler",
			tool:    "lookup",
			handler: nil,
			wantErr: toolingregistry.ErrNilHandler,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := registry.Register(tc.tool, tc.handler); !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}

			result, execErr := registry.Execute(context.Background(), agent.ToolCall{ID: "call-check", Name: "ping"})
			if execErr != nil {
				t.Fatalf("execute known handler after rejected registration: %v", execErr)
			}
			if result.Content != "pong" {
				t.Fatalf("unexpected content: %q", result.Content)
			}
		})
	}
}

func TestRegistryExecute_PropagatesHandlerError(t *testing.T) {
	t.Parallel()

	expected := errors.New("handler failed")
	registry := mustNewRegistry(t, map[string]toolingregistry.Handler{
		"fail": func(_ context.Context, _ map[string]any) (string, error) {
			return "", expected
		},
	})

	_, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-9", Name: "fail"})
	if !errors.Is(err, expected) {
		t.Fatalf("expected propagated error, got %v", err)
	}
}

func TestRegistryExecute_ContextCanceledFailsFastWithoutHandlerInvocation(t *testing.T) {
	t.Parallel()

	called := false
	registry := mustNewRegistry(t, map[string]toolingregistry.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			called = true
			return "unexpected", nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := registry.Execute(ctx, agent.ToolCall{ID: "call-canceled", Name: "lookup"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if called {
		t.Fatalf("handler must not be invoked when context is canceled")
	}
}

func TestRegistryExecute_ContextDeadlineExceededFailsFastWithoutHandlerInvocation(t *testing.T) {
	t.Parallel()

	called := false
	registry := mustNewRegistry(t, map[string]toolingregistry.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			called = true
			return "unexpected", nil
		},
	})

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	result, err := registry.Execute(ctx, agent.ToolCall{ID: "call-timeout", Name: "lookup"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if called {
		t.Fatalf("handler must not be invoked when context deadline is exceeded")
	}
}

func TestRegistryExecute_NilContextFailsFastWithoutHandlerInvocation(t *testing.T) {
	t.Parallel()

	called := false
	registry := mustNewRegistry(t, map[string]toolingregistry.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			called = true
			return "unexpected", nil
		},
	})

	result, err := registry.Execute(nil, agent.ToolCall{ID: "call-nil-context", Name: "lookup"})
	if !errors.Is(err, agent.ErrContextNil) {
		t.Fatalf("expected ErrContextNil, got %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if called {
		t.Fatalf("handler must not be invoked when context is nil")
	}
}

func mustNewRegistry(t *testing.T, initial map[string]toolingregistry.Handler) *toolingregistry.Registry {
	t.Helper()

	registry, err := toolingregistry.New(initial)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	return registry
}
