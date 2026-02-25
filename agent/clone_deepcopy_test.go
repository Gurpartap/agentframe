package agent_test

import (
	"testing"

	"agentruntime/agent"
)

func TestCloneToolCall_DeepCopiesNestedArguments(t *testing.T) {
	t.Parallel()

	original := agent.ToolCall{
		ID:   "call-1",
		Name: "lookup",
		Arguments: map[string]any{
			"query": "golang",
			"options": map[string]any{
				"filters": []any{
					map[string]any{
						"name":   "lang",
						"values": []any{"go", "rust"},
					},
				},
			},
		},
	}

	cloned := agent.CloneToolCall(original)

	cloned.Arguments["query"] = "swift"
	clonedOptions := mustMap(t, cloned.Arguments["options"])
	clonedFilters := mustSlice(t, clonedOptions["filters"])
	clonedFilter := mustMap(t, clonedFilters[0])
	clonedValues := mustSlice(t, clonedFilter["values"])
	clonedValues[0] = "javascript"
	clonedFilter["extra"] = "clone-only"
	clonedOptions["clone_only"] = []any{"value"}

	originalOptions := mustMap(t, original.Arguments["options"])
	if original.Arguments["query"] != "golang" {
		t.Fatalf("original query changed: %v", original.Arguments["query"])
	}
	if _, ok := originalOptions["clone_only"]; ok {
		t.Fatalf("clone mutation leaked into original options")
	}
	originalFilters := mustSlice(t, originalOptions["filters"])
	originalFilter := mustMap(t, originalFilters[0])
	if _, ok := originalFilter["extra"]; ok {
		t.Fatalf("clone mutation leaked into original filter")
	}
	originalValues := mustSlice(t, originalFilter["values"])
	if originalValues[0] != "go" {
		t.Fatalf("original nested value changed: %v", originalValues[0])
	}

	original.Arguments["query"] = "python"
	originalValues[1] = "java"
	originalOptions["source"] = "original-only"

	if cloned.Arguments["query"] != "swift" {
		t.Fatalf("original mutation leaked into cloned query: %v", cloned.Arguments["query"])
	}
	if _, ok := clonedOptions["source"]; ok {
		t.Fatalf("original mutation leaked into cloned options")
	}
	if clonedValues[1] != "rust" {
		t.Fatalf("original nested mutation leaked into clone: %v", clonedValues[1])
	}
}

func TestCloneMessageAndRunState_DeepCopyNestedToolArguments(t *testing.T) {
	t.Parallel()

	message := agent.Message{
		Role: agent.RoleAssistant,
		ToolCalls: []agent.ToolCall{
			{
				ID:   "call-2",
				Name: "search",
				Arguments: map[string]any{
					"body": map[string]any{
						"tags": []any{"runtime", map[string]any{"key": "value"}},
					},
				},
			},
		},
	}

	clonedMessage := agent.CloneMessage(message)
	clonedMessageBody := mustMap(t, clonedMessage.ToolCalls[0].Arguments["body"])
	clonedTags := mustSlice(t, clonedMessageBody["tags"])
	clonedTags[0] = "agent"
	mustMap(t, clonedTags[1])["key"] = "clone"

	originalMessageBody := mustMap(t, message.ToolCalls[0].Arguments["body"])
	originalTags := mustSlice(t, originalMessageBody["tags"])
	if originalTags[0] != "runtime" {
		t.Fatalf("message clone mutated original nested slice: %v", originalTags[0])
	}
	if mustMap(t, originalTags[1])["key"] != "value" {
		t.Fatalf("message clone mutated original nested map: %v", mustMap(t, originalTags[1])["key"])
	}

	originalTags[0] = "original"
	mustMap(t, originalTags[1])["key"] = "origin"
	if clonedTags[0] != "agent" {
		t.Fatalf("original message mutation leaked into clone: %v", clonedTags[0])
	}
	if mustMap(t, clonedTags[1])["key"] != "clone" {
		t.Fatalf("original message mutation leaked into cloned nested map: %v", mustMap(t, clonedTags[1])["key"])
	}

	state := agent.RunState{
		ID:       "run-1",
		Messages: []agent.Message{message},
	}
	clonedState := agent.CloneRunState(state)
	clonedStateBody := mustMap(t, clonedState.Messages[0].ToolCalls[0].Arguments["body"])
	clonedStateTags := mustSlice(t, clonedStateBody["tags"])
	clonedStateTags[0] = "state-clone"
	mustMap(t, clonedStateTags[1])["key"] = "state-clone-map"

	stateBody := mustMap(t, state.Messages[0].ToolCalls[0].Arguments["body"])
	stateTags := mustSlice(t, stateBody["tags"])
	if stateTags[0] != "original" {
		t.Fatalf("run state clone mutated original nested slice: %v", stateTags[0])
	}
	if mustMap(t, stateTags[1])["key"] != "origin" {
		t.Fatalf("run state clone mutated original nested map: %v", mustMap(t, stateTags[1])["key"])
	}

	stateTags[0] = "state-original"
	mustMap(t, stateTags[1])["key"] = "state-original-map"
	if clonedStateTags[0] != "state-clone" {
		t.Fatalf("original state mutation leaked into clone: %v", clonedStateTags[0])
	}
	if mustMap(t, clonedStateTags[1])["key"] != "state-clone-map" {
		t.Fatalf("original state mutation leaked into cloned nested map: %v", mustMap(t, clonedStateTags[1])["key"])
	}
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return m
}

func mustSlice(t *testing.T, value any) []any {
	t.Helper()

	s, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", value)
	}
	return s
}
