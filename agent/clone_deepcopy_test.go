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

func TestCloneToolDefinitions_DeepCopiesNestedInputSchema(t *testing.T) {
	t.Parallel()

	original := []agent.ToolDefinition{
		{
			Name: "lookup",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type": "string",
						"enum": []any{"alpha", "beta"},
					},
				},
				"required": []any{"query"},
			},
		},
	}
	cloned := agent.CloneToolDefinitions(original)

	cloned[0].Name = "mutated"
	cloned[0].InputSchema["type"] = "array"
	cloned[0].InputSchema["added"] = map[string]any{"flag": true}
	clonedProperties := mustMap(t, cloned[0].InputSchema["properties"])
	clonedQuery := mustMap(t, clonedProperties["query"])
	clonedEnum := mustSlice(t, clonedQuery["enum"])
	clonedEnum[0] = "mutated-alpha"
	clonedQuery["extra"] = "x"
	clonedRequired := mustSlice(t, cloned[0].InputSchema["required"])
	clonedRequired[0] = "changed-required"

	if original[0].Name != "lookup" {
		t.Fatalf("clone mutation leaked into original name: %q", original[0].Name)
	}
	if original[0].InputSchema["type"] != "object" {
		t.Fatalf("clone mutation leaked into original schema type: %v", original[0].InputSchema["type"])
	}
	if _, ok := original[0].InputSchema["added"]; ok {
		t.Fatalf("clone mutation leaked into original schema map")
	}
	originalProperties := mustMap(t, original[0].InputSchema["properties"])
	originalQuery := mustMap(t, originalProperties["query"])
	originalEnum := mustSlice(t, originalQuery["enum"])
	if originalEnum[0] != "alpha" {
		t.Fatalf("clone mutation leaked into original nested slice: %v", originalEnum[0])
	}
	if _, ok := originalQuery["extra"]; ok {
		t.Fatalf("clone mutation leaked into original nested map")
	}
	originalRequired := mustSlice(t, original[0].InputSchema["required"])
	if originalRequired[0] != "query" {
		t.Fatalf("clone mutation leaked into original required: %v", originalRequired[0])
	}

	original[0].Name = "original-mutated"
	original[0].InputSchema["type"] = "object-mutated"
	originalEnum[0] = "original-alpha"
	originalRequired[0] = "original-required"

	if cloned[0].Name != "mutated" {
		t.Fatalf("original mutation leaked into cloned name: %q", cloned[0].Name)
	}
	if cloned[0].InputSchema["type"] != "array" {
		t.Fatalf("original mutation leaked into cloned schema type: %v", cloned[0].InputSchema["type"])
	}
	if clonedEnum[0] != "mutated-alpha" {
		t.Fatalf("original mutation leaked into cloned nested slice: %v", clonedEnum[0])
	}
	if clonedRequired[0] != "changed-required" {
		t.Fatalf("original mutation leaked into cloned required: %v", clonedRequired[0])
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
