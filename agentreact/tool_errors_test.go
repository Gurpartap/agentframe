package agentreact_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

func TestToolFailure_UnknownTool(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Trying unknown tool.",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "ghost"},
				},
			},
		},
		response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := newRegistry(map[string]handler{
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
	model := newScriptedModel(
		response{
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
		response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := newRegistry(map[string]handler{
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
	model := newScriptedModel(
		response{
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
		response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := newRegistry(map[string]handler{
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

func TestToolFailure_ExecutorResultCallIDMismatch(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
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
		response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		return agent.ToolResult{
			CallID:  "other-call",
			Name:    "lookup",
			Content: "tool-value",
		}, nil
	})
	tools := []agent.ToolDefinition{{Name: "lookup"}}

	result, events := runToolTest(t, model, executor, tools)

	toolResult := mustToolResultEvent(t, events.Events())
	if !toolResult.IsError {
		t.Fatalf("expected tool result error")
	}
	if toolResult.FailureReason != agent.ToolFailureReasonExecutorError {
		t.Fatalf("unexpected failure reason: %s", toolResult.FailureReason)
	}
	if toolResult.CallID != "call-1" {
		t.Fatalf("unexpected tool result call id: %q", toolResult.CallID)
	}
	if toolResult.Name != "lookup" {
		t.Fatalf("unexpected tool result name: %q", toolResult.Name)
	}
	if !strings.Contains(toolResult.Content, "call id mismatch") {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if result.State.Messages[2].Role != agent.RoleTool {
		t.Fatalf("unexpected transcript role: %+v", result.State.Messages[2])
	}
	if result.State.Messages[2].ToolCallID != "call-1" {
		t.Fatalf("unexpected transcript tool_call_id: %q", result.State.Messages[2].ToolCallID)
	}
	if result.State.Messages[2].Name != "lookup" {
		t.Fatalf("unexpected transcript tool name: %q", result.State.Messages[2].Name)
	}
}

func TestToolFailure_ExecutorResultNameMismatch(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
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
		response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		return agent.ToolResult{
			CallID:  "call-1",
			Name:    "other-tool",
			Content: "tool-value",
		}, nil
	})
	tools := []agent.ToolDefinition{{Name: "lookup"}}

	result, events := runToolTest(t, model, executor, tools)

	toolResult := mustToolResultEvent(t, events.Events())
	if !toolResult.IsError {
		t.Fatalf("expected tool result error")
	}
	if toolResult.FailureReason != agent.ToolFailureReasonExecutorError {
		t.Fatalf("unexpected failure reason: %s", toolResult.FailureReason)
	}
	if toolResult.CallID != "call-1" {
		t.Fatalf("unexpected tool result call id: %q", toolResult.CallID)
	}
	if toolResult.Name != "lookup" {
		t.Fatalf("unexpected tool result name: %q", toolResult.Name)
	}
	if !strings.Contains(toolResult.Content, "name mismatch") {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if result.State.Messages[2].Role != agent.RoleTool {
		t.Fatalf("unexpected transcript role: %+v", result.State.Messages[2])
	}
	if result.State.Messages[2].ToolCallID != "call-1" {
		t.Fatalf("unexpected transcript tool_call_id: %q", result.State.Messages[2].ToolCallID)
	}
	if result.State.Messages[2].Name != "lookup" {
		t.Fatalf("unexpected transcript tool name: %q", result.State.Messages[2].Name)
	}
}

func TestToolResultNormalization_EmptyIdentityFieldsRemainValid(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
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
		response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	executor := toolExecutorFunc(func(_ context.Context, _ agent.ToolCall) (agent.ToolResult, error) {
		return agent.ToolResult{
			CallID:  "",
			Name:    "",
			Content: "tool-value",
		}, nil
	})
	tools := []agent.ToolDefinition{{Name: "lookup"}}

	result, events := runToolTest(t, model, executor, tools)

	toolResult := mustToolResultEvent(t, events.Events())
	if toolResult.IsError {
		t.Fatalf("expected non-error tool result")
	}
	if toolResult.FailureReason != "" {
		t.Fatalf("unexpected failure reason: %q", toolResult.FailureReason)
	}
	if toolResult.CallID != "call-1" {
		t.Fatalf("unexpected tool result call id: %q", toolResult.CallID)
	}
	if toolResult.Name != "lookup" {
		t.Fatalf("unexpected tool result name: %q", toolResult.Name)
	}
	if toolResult.Content != "tool-value" {
		t.Fatalf("unexpected tool result content: %q", toolResult.Content)
	}
	if result.State.Messages[2].Role != agent.RoleTool {
		t.Fatalf("unexpected transcript role: %+v", result.State.Messages[2])
	}
	if result.State.Messages[2].ToolCallID != "call-1" {
		t.Fatalf("unexpected transcript tool_call_id: %q", result.State.Messages[2].ToolCallID)
	}
	if result.State.Messages[2].Name != "lookup" {
		t.Fatalf("unexpected transcript tool name: %q", result.State.Messages[2].Name)
	}
	if result.State.Messages[2].Content != "tool-value" {
		t.Fatalf("unexpected transcript content: %q", result.State.Messages[2].Content)
	}
}

func TestToolFailure_ValidArgumentsUnchangedPath(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	model := newScriptedModel(
		response{
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
		response{
			Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
		},
	)
	registry := newRegistry(map[string]handler{
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

func TestToolCallValidation_InvalidShapeFailsRunBeforeExecution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		toolCalls []agent.ToolCall
		wantError string
	}{
		{
			name: "empty call id",
			toolCalls: []agent.ToolCall{
				{ID: "", Name: "lookup"},
			},
			wantError: "tool call is invalid: index=0 reason=empty_id",
		},
		{
			name: "empty call name",
			toolCalls: []agent.ToolCall{
				{ID: "call-1", Name: ""},
			},
			wantError: "tool call is invalid: index=0 id=\"call-1\" reason=empty_name",
		},
		{
			name: "duplicate call ids",
			toolCalls: []agent.ToolCall{
				{ID: "dup-id", Name: "lookup"},
				{ID: "dup-id", Name: "lookup"},
			},
			wantError: "tool call is invalid: index=1 id=\"dup-id\" reason=duplicate_id first_index=0",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			model := newScriptedModel(response{
				Message: agent.Message{
					Role:      agent.RoleAssistant,
					Content:   "invalid tool call shape",
					ToolCalls: tc.toolCalls,
				},
			})
			registry := newRegistry(map[string]handler{
				"lookup": func(_ context.Context, _ map[string]any) (string, error) {
					calls.Add(1)
					return "unexpected", nil
				},
			})

			result, runErr, events := runToolTestExpectError(t, model, registry, []agent.ToolDefinition{
				{Name: "lookup"},
			})

			if !errors.Is(runErr, agentreact.ErrToolCallInvalid) {
				t.Fatalf("expected ErrToolCallInvalid, got %v", runErr)
			}
			if calls.Load() != 0 {
				t.Fatalf("unexpected executor calls: %d", calls.Load())
			}
			if result.State.Status != agent.RunStatusFailed {
				t.Fatalf("unexpected status: got=%s want=%s", result.State.Status, agent.RunStatusFailed)
			}
			if result.State.Error != tc.wantError {
				t.Fatalf("unexpected state error: got=%q want=%q", result.State.Error, tc.wantError)
			}
			if countEventsByType(events.Events(), agent.EventTypeToolResult) != 0 {
				t.Fatalf("unexpected tool_result events for invalid tool-call shape")
			}
			runFailedEvent := mustSingleRunFailedEvent(t, events.Events())
			wantDescription := "model error: " + tc.wantError
			if runFailedEvent.Description != wantDescription {
				t.Fatalf("unexpected run failed description: got=%q want=%q", runFailedEvent.Description, wantDescription)
			}
		})
	}
}

type toolExecutorFunc func(context.Context, agent.ToolCall) (agent.ToolResult, error)

func (f toolExecutorFunc) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	return f(ctx, call)
}

func runToolTest(
	t *testing.T,
	model *scriptedModel,
	executor agentreact.ToolExecutor,
	tools []agent.ToolDefinition,
) (agent.RunResult, *eventSink) {
	t.Helper()

	result, runErr, events := runToolTestExpectError(t, model, executor, tools)
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	return result, events
}

func runToolTestExpectError(
	t *testing.T,
	model *scriptedModel,
	executor agentreact.ToolExecutor,
	tools []agent.ToolDefinition,
) (agent.RunResult, error, *eventSink) {
	t.Helper()

	events := newEventSink()
	loop, err := agentreact.New(model, executor, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Use tools.",
		MaxSteps:   3,
		Tools:      tools,
	})
	return result, runErr, events
}

func countEventsByType(events []agent.Event, eventType agent.EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func mustSingleRunFailedEvent(t *testing.T, events []agent.Event) agent.Event {
	t.Helper()

	var runFailedEvent agent.Event
	count := 0
	for _, event := range events {
		if event.Type != agent.EventTypeRunFailed {
			continue
		}
		runFailedEvent = event
		count++
	}
	if count != 1 {
		t.Fatalf("unexpected run failed event count: got=%d want=1", count)
	}
	return runFailedEvent
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
