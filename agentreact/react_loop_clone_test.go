package agentreact

import (
	"testing"

	"agentruntime/agent"
)

func TestCloneToolDefinitions_DeepCopiesNestedInputSchema(t *testing.T) {
	t.Parallel()

	original := []agent.ToolDefinition{
		{
			Name: "lookup",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filters": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"tags": map[string]any{
									"type":  "array",
									"items": []any{"alpha", map[string]any{"kind": "string"}},
								},
							},
						},
					},
				},
				"required": []any{"query"},
			},
		},
	}

	cloned := cloneToolDefinitions(original)

	clonedSchema := mustSchemaMap(t, cloned[0].InputSchema)
	clonedSchema["type"] = "array"
	clonedProperties := mustSchemaMap(t, clonedSchema["properties"])
	clonedFilters := mustSchemaMap(t, clonedProperties["filters"])
	clonedItems := mustSchemaMap(t, clonedFilters["items"])
	clonedItemsProperties := mustSchemaMap(t, clonedItems["properties"])
	clonedTags := mustSchemaMap(t, clonedItemsProperties["tags"])
	clonedTagItems := mustSchemaSlice(t, clonedTags["items"])
	clonedTagItems[0] = "cloned-alpha"
	mustSchemaMap(t, clonedTagItems[1])["kind"] = "number"
	clonedRequired := mustSchemaSlice(t, clonedSchema["required"])
	clonedRequired[0] = "cloned-required"

	originalSchema := mustSchemaMap(t, original[0].InputSchema)
	if originalSchema["type"] != "object" {
		t.Fatalf("clone mutation leaked into original schema type: %v", originalSchema["type"])
	}
	originalProperties := mustSchemaMap(t, originalSchema["properties"])
	originalFilters := mustSchemaMap(t, originalProperties["filters"])
	originalItems := mustSchemaMap(t, originalFilters["items"])
	originalItemsProperties := mustSchemaMap(t, originalItems["properties"])
	originalTags := mustSchemaMap(t, originalItemsProperties["tags"])
	originalTagItems := mustSchemaSlice(t, originalTags["items"])
	if originalTagItems[0] != "alpha" {
		t.Fatalf("clone mutation leaked into original nested slice: %v", originalTagItems[0])
	}
	if mustSchemaMap(t, originalTagItems[1])["kind"] != "string" {
		t.Fatalf("clone mutation leaked into original nested map: %v", mustSchemaMap(t, originalTagItems[1])["kind"])
	}
	originalRequired := mustSchemaSlice(t, originalSchema["required"])
	if originalRequired[0] != "query" {
		t.Fatalf("clone mutation leaked into original required: %v", originalRequired[0])
	}

	originalSchema["type"] = "original-type"
	originalTagItems[0] = "original-alpha"
	mustSchemaMap(t, originalTagItems[1])["kind"] = "boolean"
	originalRequired[0] = "original-required"

	if clonedSchema["type"] != "array" {
		t.Fatalf("original mutation leaked into cloned schema type: %v", clonedSchema["type"])
	}
	if clonedTagItems[0] != "cloned-alpha" {
		t.Fatalf("original mutation leaked into cloned nested slice: %v", clonedTagItems[0])
	}
	if mustSchemaMap(t, clonedTagItems[1])["kind"] != "number" {
		t.Fatalf("original mutation leaked into cloned nested map: %v", mustSchemaMap(t, clonedTagItems[1])["kind"])
	}
	if clonedRequired[0] != "cloned-required" {
		t.Fatalf("original mutation leaked into cloned required: %v", clonedRequired[0])
	}
}

func mustSchemaMap(t *testing.T, value any) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return m
}

func mustSchemaSlice(t *testing.T, value any) []any {
	t.Helper()

	s, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", value)
	}
	return s
}
