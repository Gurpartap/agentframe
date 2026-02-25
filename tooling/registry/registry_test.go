package registry_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agentruntime/agent"
	toolingregistry "agentruntime/tooling/registry"
)

func TestRegistryExecute_UnknownToolReturnsError(t *testing.T) {
	t.Parallel()

	registry := toolingregistry.New(nil)
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

	registry := toolingregistry.New(nil)
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
	registry := toolingregistry.New(map[string]toolingregistry.Handler{
		"": func(_ context.Context, _ map[string]any) (string, error) {
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

	registry := toolingregistry.New(map[string]toolingregistry.Handler{
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

func TestRegistryExecute_NilHandlerFromInitialMapReturnsError(t *testing.T) {
	t.Parallel()

	registry := toolingregistry.New(map[string]toolingregistry.Handler{
		"lookup": nil,
	})

	result, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-2", Name: "lookup"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, toolingregistry.ErrNilHandler) {
		t.Fatalf("expected ErrNilHandler, got %v", err)
	}
	if !strings.Contains(err.Error(), `"lookup"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegistryRegister_AddsHandler(t *testing.T) {
	t.Parallel()

	registry := toolingregistry.New(nil)
	registry.Register("ping", func(_ context.Context, _ map[string]any) (string, error) {
		return "pong", nil
	})

	result, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-7", Name: "ping"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Content != "pong" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
}

func TestRegistryExecute_NilHandlerFromRegisterReturnsError(t *testing.T) {
	t.Parallel()

	registry := toolingregistry.New(nil)
	registry.Register("lookup", nil)

	result, err := registry.Execute(context.Background(), agent.ToolCall{ID: "call-8", Name: "lookup"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, toolingregistry.ErrNilHandler) {
		t.Fatalf("expected ErrNilHandler, got %v", err)
	}
	if !strings.Contains(err.Error(), `"lookup"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegistryExecute_PropagatesHandlerError(t *testing.T) {
	t.Parallel()

	expected := errors.New("handler failed")
	registry := toolingregistry.New(map[string]toolingregistry.Handler{
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
	registry := toolingregistry.New(map[string]toolingregistry.Handler{
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
	registry := toolingregistry.New(map[string]toolingregistry.Handler{
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
