package registry_test

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	if !strings.Contains(err.Error(), `tool "missing" is not registered`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (agent.ToolResult{}) {
		t.Fatalf("unexpected result: %+v", result)
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
	if !strings.Contains(err.Error(), `tool "lookup" has nil handler`) {
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
	if !strings.Contains(err.Error(), `tool "lookup" has nil handler`) {
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
