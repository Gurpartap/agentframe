package agent_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"agentruntime/agent"
	"agentruntime/agent/internal/testkit"
)

func TestToolFailure_UnknownTool(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Trying unknown tool.",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "ghost"},
				},
			},
		},
		testkit.Response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := testkit.NewRegistry(map[string]testkit.Handler{
		"ghost": func(_ context.Context, _ map[string]any) (string, error) {
			calls.Add(1)
			return "should not run", nil
		},
	})
	tools := []agent.ToolDefinition{{Name: "lookup"}}

	result, events := runToolTest(t, model, registry, tools)

	if calls.Load() != 0 {
		t.Fatalf("unexpected executor calls: %d", calls.Load())
	}
	toolResult := mustToolResultEvent(t, events.Events())
	if !toolResult.IsError {
		t.Fatalf("expected tool result error")
	}
	if toolResult.FailureReason != agent.ToolFailureReasonUnknownTool {
		t.Fatalf("unexpected failure reason: %s", toolResult.FailureReason)
	}
	if !strings.Contains(toolResult.Content, string(agent.ToolFailureReasonUnknownTool)) {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if result.State.Messages[2].Role != agent.RoleTool || !strings.Contains(result.State.Messages[2].Content, string(agent.ToolFailureReasonUnknownTool)) {
		t.Fatalf("unexpected transcript tool message: %+v", result.State.Messages[2])
	}
}

func TestToolFailure_InvalidArguments_NoExecutorCall(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Calling tool with invalid args.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
						Arguments: map[string]any{
							"q": 42,
						},
					},
				},
			},
		},
		testkit.Response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := testkit.NewRegistry(map[string]testkit.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			calls.Add(1)
			return "should not run", nil
		},
	})
	tools := []agent.ToolDefinition{
		{
			Name: "lookup",
			InputSchema: map[string]any{
				"required": []string{"q"},
				"properties": map[string]any{
					"q": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	result, events := runToolTest(t, model, registry, tools)

	if calls.Load() != 0 {
		t.Fatalf("unexpected executor calls: %d", calls.Load())
	}
	toolResult := mustToolResultEvent(t, events.Events())
	if !toolResult.IsError {
		t.Fatalf("expected tool result error")
	}
	if toolResult.FailureReason != agent.ToolFailureReasonInvalidArguments {
		t.Fatalf("unexpected failure reason: %s", toolResult.FailureReason)
	}
	if !strings.Contains(toolResult.Content, string(agent.ToolFailureReasonInvalidArguments)) {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if result.State.Messages[2].Role != agent.RoleTool || !strings.Contains(result.State.Messages[2].Content, string(agent.ToolFailureReasonInvalidArguments)) {
		t.Fatalf("unexpected transcript tool message: %+v", result.State.Messages[2])
	}
}

func TestToolFailure_ExecutorError(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Calling tool.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
						Arguments: map[string]any{
							"q": "Go",
						},
					},
				},
			},
		},
		testkit.Response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := testkit.NewRegistry(map[string]testkit.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			calls.Add(1)
			return "", errors.New("backend timeout")
		},
	})
	tools := []agent.ToolDefinition{
		{
			Name: "lookup",
			InputSchema: map[string]any{
				"required": []string{"q"},
				"properties": map[string]any{
					"q": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	result, events := runToolTest(t, model, registry, tools)

	if calls.Load() != 1 {
		t.Fatalf("unexpected executor calls: %d", calls.Load())
	}
	toolResult := mustToolResultEvent(t, events.Events())
	if !toolResult.IsError {
		t.Fatalf("expected tool result error")
	}
	if toolResult.FailureReason != agent.ToolFailureReasonExecutorError {
		t.Fatalf("unexpected failure reason: %s", toolResult.FailureReason)
	}
	if !strings.Contains(toolResult.Content, "backend timeout") {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if result.State.Messages[2].Role != agent.RoleTool || !strings.Contains(result.State.Messages[2].Content, string(agent.ToolFailureReasonExecutorError)) {
		t.Fatalf("unexpected transcript tool message: %+v", result.State.Messages[2])
	}
}

func TestToolFailure_ValidArgumentsUnchangedPath(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Calling tool.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
						Arguments: map[string]any{
							"q": "Go",
						},
					},
				},
			},
		},
		testkit.Response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := testkit.NewRegistry(map[string]testkit.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			calls.Add(1)
			return "tool-value", nil
		},
	})
	tools := []agent.ToolDefinition{
		{
			Name: "lookup",
			InputSchema: map[string]any{
				"required": []string{"q"},
				"properties": map[string]any{
					"q": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	result, events := runToolTest(t, model, registry, tools)

	if calls.Load() != 1 {
		t.Fatalf("unexpected executor calls: %d", calls.Load())
	}
	toolResult := mustToolResultEvent(t, events.Events())
	if toolResult.IsError {
		t.Fatalf("expected non-error tool result")
	}
	if toolResult.FailureReason != "" {
		t.Fatalf("unexpected failure reason: %q", toolResult.FailureReason)
	}
	if toolResult.Content != "tool-value" {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if result.State.Messages[2].Role != agent.RoleTool || result.State.Messages[2].Content != "tool-value" {
		t.Fatalf("unexpected transcript tool message: %+v", result.State.Messages[2])
	}
}

func runToolTest(
	t *testing.T,
	model *testkit.ScriptedModel,
	registry *testkit.Registry,
	tools []agent.ToolDefinition,
) (agent.RunResult, *testkit.EventSink) {
	t.Helper()

	events := testkit.NewEventSink()
	loop, err := agent.NewReactLoop(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: testkit.NewCounterIDGenerator("tool"),
		RunStore:    testkit.NewRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Use tools.",
		MaxSteps:   3,
		Tools:      tools,
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	return result, events
}

func mustToolResultEvent(t *testing.T, events []agent.Event) agent.ToolResult {
	t.Helper()
	for _, event := range events {
		if event.Type != agent.EventTypeToolResult || event.ToolResult == nil {
			continue
		}
		return *event.ToolResult
	}
	t.Fatal("no tool result event found")
	return agent.ToolResult{}
}
