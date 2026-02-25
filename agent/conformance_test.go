package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"agentruntime/agent"
	"agentruntime/agent/internal/testkit"
)

func TestConformance_EventOrdering(t *testing.T) {
	t.Parallel()

	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "I need to use a tool.",
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
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Final answer.",
			},
		},
	)
	registry := testkit.NewRegistry(map[string]testkit.Handler{
		"lookup": func(_ context.Context, args map[string]any) (string, error) {
			return "value_for=" + args["q"].(string), nil
		},
	})
	events := testkit.NewEventSink()
	loop, err := agent.NewReactLoop(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: testkit.NewCounterIDGenerator("evt"),
		RunStore:    testkit.NewRunStore(),
		ReactLoop:   loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Find info about Go.",
		MaxSteps:   4,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	got := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeAssistantMessage,
		agent.EventTypeToolResult,
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
	}
	wantSteps := []int{0, 1, 1, 2, 2, 2}
	if len(got) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(got), len(wantTypes))
	}
	for i := range wantTypes {
		if got[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, got[i].Type, wantTypes[i])
		}
		if got[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, got[i].Step, wantSteps[i])
		}
		if got[i].RunID != result.State.ID {
			t.Fatalf("event[%d] run_id mismatch: got=%s want=%s", i, got[i].RunID, result.State.ID)
		}
	}
	if got[1].Message == nil || len(got[1].Message.ToolCalls) != 1 || got[1].Message.ToolCalls[0].ID != "call-1" {
		t.Fatalf("assistant event does not contain expected tool call")
	}
	if got[2].ToolResult == nil || got[2].ToolResult.CallID != "call-1" || got[2].ToolResult.Name != "lookup" {
		t.Fatalf("tool result event does not link to expected tool call")
	}
}

func TestConformance_TranscriptToolCallResultLinkage(t *testing.T) {
	t.Parallel()

	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "I need two tools.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
						Arguments: map[string]any{
							"q": "Go",
						},
					},
					{
						ID:   "call-2",
						Name: "summarize",
						Arguments: map[string]any{
							"text": "Go is a programming language",
						},
					},
				},
			},
		},
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Final answer after both tools.",
			},
		},
	)
	registry := testkit.NewRegistry(map[string]testkit.Handler{
		"lookup": func(_ context.Context, args map[string]any) (string, error) {
			return "lookup=" + args["q"].(string), nil
		},
		"summarize": func(_ context.Context, args map[string]any) (string, error) {
			return "summary=" + args["text"].(string), nil
		},
	})
	loop, err := agent.NewReactLoop(model, registry, testkit.NewEventSink())
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: testkit.NewCounterIDGenerator("link"),
		RunStore:    testkit.NewRunStore(),
		ReactLoop:   loop,
		EventSink:   testkit.NewEventSink(),
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Use tools.",
		MaxSteps:   4,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
			{Name: "summarize"},
		},
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	callByID := map[string]agent.ToolCall{}
	callIndex := map[string]int{}
	resultCount := map[string]int{}
	for i, msg := range result.State.Messages {
		if msg.Role == agent.RoleAssistant {
			for _, call := range msg.ToolCalls {
				callByID[call.ID] = call
				callIndex[call.ID] = i
			}
			continue
		}
		if msg.Role != agent.RoleTool {
			continue
		}
		if msg.ToolCallID == "" {
			t.Fatalf("tool result at index %d has empty tool_call_id", i)
		}
		call, ok := callByID[msg.ToolCallID]
		if !ok {
			t.Fatalf("tool result at index %d references unknown tool_call_id %q", i, msg.ToolCallID)
		}
		if i <= callIndex[msg.ToolCallID] {
			t.Fatalf("tool result at index %d appears before its tool call", i)
		}
		if msg.Name != call.Name {
			t.Fatalf("tool result at index %d has name %q, want %q", i, msg.Name, call.Name)
		}
		resultCount[msg.ToolCallID]++
	}

	for id := range callByID {
		if resultCount[id] != 1 {
			t.Fatalf("tool call %q has result count %d, want 1", id, resultCount[id])
		}
	}
}

func TestConformance_ContinueDeterministicProgression(t *testing.T) {
	t.Parallel()

	model := testkit.NewScriptedModel(
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Need tool first.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
					},
				},
			},
		},
		testkit.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Final answer after continue.",
			},
		},
	)
	store := testkit.NewRunStore()
	registry := testkit.NewRegistry(map[string]testkit.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-value", nil
		},
	})
	loop, err := agent.NewReactLoop(model, registry, testkit.NewEventSink())
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: testkit.NewCounterIDGenerator("continue"),
		RunStore:    store,
		ReactLoop:   loop,
		EventSink:   testkit.NewEventSink(),
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	initialResult, initialErr := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Do the thing.",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(initialErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", initialErr)
	}
	if initialResult.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected initial status: %s", initialResult.State.Status)
	}
	if initialResult.State.Step != 1 {
		t.Fatalf("unexpected initial step: %d", initialResult.State.Step)
	}
	if initialResult.State.Version != 2 {
		t.Fatalf("unexpected initial version: %d", initialResult.State.Version)
	}

	loadedBeforeContinue, err := store.Load(context.Background(), initialResult.State.ID)
	if err != nil {
		t.Fatalf("load before continue: %v", err)
	}
	if !reflect.DeepEqual(loadedBeforeContinue, initialResult.State) {
		t.Fatalf("saved state mismatch before continue")
	}

	beforeMessages := agent.CloneMessages(initialResult.State.Messages)
	continuedResult, continueErr := runner.Continue(context.Background(), initialResult.State.ID, 3, []agent.ToolDefinition{
		{Name: "lookup"},
	})
	if continueErr != nil {
		t.Fatalf("continue returned error: %v", continueErr)
	}
	if continuedResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected continued status: %s", continuedResult.State.Status)
	}
	if continuedResult.State.Step != 2 {
		t.Fatalf("unexpected continued step: %d", continuedResult.State.Step)
	}
	if continuedResult.State.Version != initialResult.State.Version+1 {
		t.Fatalf("unexpected continued version: got=%d want=%d", continuedResult.State.Version, initialResult.State.Version+1)
	}
	if continuedResult.State.Output != "Final answer after continue." {
		t.Fatalf("unexpected continued output: %q", continuedResult.State.Output)
	}
	if len(continuedResult.State.Messages) <= len(beforeMessages) {
		t.Fatalf("expected transcript growth after continue")
	}
	if !reflect.DeepEqual(continuedResult.State.Messages[:len(beforeMessages)], beforeMessages) {
		t.Fatalf("continue mutated existing transcript prefix")
	}

	loadedAfterContinue, err := store.Load(context.Background(), initialResult.State.ID)
	if err != nil {
		t.Fatalf("load after continue: %v", err)
	}
	if !reflect.DeepEqual(loadedAfterContinue, continuedResult.State) {
		t.Fatalf("saved state mismatch after continue")
	}
}
