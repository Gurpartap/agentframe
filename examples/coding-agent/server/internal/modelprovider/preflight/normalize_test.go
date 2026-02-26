package preflight

import (
	"reflect"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
)

func TestNormalizeMessagesForProvider_ValidPassThrough(t *testing.T) {
	t.Parallel()

	input := []agent.Message{
		{Role: agent.RoleSystem, Content: "system"},
		{Role: agent.RoleUser, Content: "user"},
		{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "bash", Arguments: map[string]any{"command": "pwd"}},
			},
		},
		{Role: agent.RoleTool, ToolCallID: "call-1", Name: "bash", Content: "ok"},
		{Role: agent.RoleAssistant, Content: "done"},
	}

	got, err := NormalizeMessagesForProvider(input)
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("normalized transcript mismatch:\n got=%+v\nwant=%+v", got, input)
	}
}

func TestNormalizeMessagesForProvider_DuplicateToolObservationDedupe(t *testing.T) {
	t.Parallel()

	got, err := NormalizeMessagesForProvider([]agent.Message{
		{Role: agent.RoleUser, Content: "start"},
		{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "bash", Arguments: map[string]any{"command": "git status"}},
			},
		},
		{Role: agent.RoleTool, ToolCallID: "call-1", Name: "bash", Content: "blocked"},
		{Role: agent.RoleUser, Content: `[resolution] requirement_id="req-1"`},
		{Role: agent.RoleTool, ToolCallID: "call-1", Name: "bash", Content: "ok"},
	})
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("normalized message count mismatch: got=%d want=%d", len(got), 4)
	}
	if got[2].Role != agent.RoleTool || got[2].ToolCallID != "call-1" || got[2].Content != "ok" {
		t.Fatalf("deduped tool message mismatch: got=%+v", got[2])
	}
	if got[3].Role != agent.RoleUser {
		t.Fatalf("expected non-tool order preserved, got role=%s at index 3", got[3].Role)
	}
}

func TestNormalizeMessagesForProvider_MissingToolCallIDError(t *testing.T) {
	t.Parallel()

	_, err := NormalizeMessagesForProvider([]agent.Message{
		{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "bash"},
			},
		},
		{Role: agent.RoleTool, Name: "bash", Content: "result"},
	})
	if err == nil {
		t.Fatalf("expected error for missing tool_call_id")
	}
}

func TestNormalizeMessagesForProvider_UnknownToolCallIDError(t *testing.T) {
	t.Parallel()

	_, err := NormalizeMessagesForProvider([]agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
		{Role: agent.RoleTool, ToolCallID: "call-unknown", Name: "bash", Content: "result"},
	})
	if err == nil {
		t.Fatalf("expected error for unknown tool_call_id")
	}
}

func TestNormalizeMessagesForProvider_MultiCallStability(t *testing.T) {
	t.Parallel()

	got, err := NormalizeMessagesForProvider([]agent.Message{
		{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "bash"},
				{ID: "call-2", Name: "bash"},
			},
		},
		{Role: agent.RoleTool, ToolCallID: "call-2", Name: "bash", Content: "call2-first"},
		{Role: agent.RoleTool, ToolCallID: "call-1", Name: "bash", Content: "call1-only"},
		{Role: agent.RoleTool, ToolCallID: "call-2", Name: "bash", Content: "call2-latest"},
	})
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("normalized message count mismatch: got=%d want=%d", len(got), 3)
	}
	if got[1].ToolCallID != "call-2" || got[1].Content != "call2-latest" {
		t.Fatalf("call-2 stability mismatch: got=%+v", got[1])
	}
	if got[2].ToolCallID != "call-1" || got[2].Content != "call1-only" {
		t.Fatalf("call-1 stability mismatch: got=%+v", got[2])
	}
}

func TestNormalizeMessagesForProvider_MessageOrderStability(t *testing.T) {
	t.Parallel()

	got, err := NormalizeMessagesForProvider([]agent.Message{
		{Role: agent.RoleSystem, Content: "s"},
		{Role: agent.RoleUser, Content: "u1"},
		{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "bash"},
			},
		},
		{Role: agent.RoleTool, ToolCallID: "call-1", Name: "bash", Content: "first"},
		{Role: agent.RoleUser, Content: "u2"},
		{Role: agent.RoleTool, ToolCallID: "call-1", Name: "bash", Content: "latest"},
		{Role: agent.RoleAssistant, Content: "done"},
	})
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	if len(got) != 6 {
		t.Fatalf("normalized message count mismatch: got=%d want=%d", len(got), 6)
	}
	if got[0].Role != agent.RoleSystem || got[0].Content != "s" {
		t.Fatalf("message order mismatch at index 0: got=%+v", got[0])
	}
	if got[1].Role != agent.RoleUser || got[1].Content != "u1" {
		t.Fatalf("message order mismatch at index 1: got=%+v", got[1])
	}
	if got[4].Role != agent.RoleUser || got[4].Content != "u2" {
		t.Fatalf("message order mismatch at index 4: got=%+v", got[4])
	}
	if got[5].Role != agent.RoleAssistant || got[5].Content != "done" {
		t.Fatalf("message order mismatch at index 5: got=%+v", got[5])
	}
	if got[3].Role != agent.RoleTool || got[3].Content != "latest" {
		t.Fatalf("deduped tool order mismatch at index 3: got=%+v", got[3])
	}
}
