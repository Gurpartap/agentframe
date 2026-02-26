package agentreact

import (
	"context"
	"reflect"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
)

func TestReactLoopExecute_ClonesToolDefinitionsForModelRequests(t *testing.T) {
	t.Parallel()

	inputTools := []agent.ToolDefinition{
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
	wantTools := agent.CloneToolDefinitions(inputTools)

	model := &mutatingModel{
		t:         t,
		wantTools: wantTools,
	}
	loop, err := New(model, noopToolExecutor{}, nil)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}

	state := agent.RunState{
		ID:     "react-loop-clone-tools",
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hello"},
		},
	}
	result, execErr := loop.Execute(context.Background(), state, agent.EngineInput{
		MaxSteps: 2,
		Tools:    inputTools,
	})
	if execErr != nil {
		t.Fatalf("execute returned error: %v", execErr)
	}
	if model.calls != 1 {
		t.Fatalf("unexpected model calls: %d", model.calls)
	}
	if result.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Output != "done" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if !reflect.DeepEqual(inputTools, wantTools) {
		t.Fatalf("input tools mutated by model-boundary request: got=%+v want=%+v", inputTools, wantTools)
	}
}

type mutatingModel struct {
	t         *testing.T
	calls     int
	wantTools []agent.ToolDefinition
}

func (m *mutatingModel) Generate(_ context.Context, request ModelRequest) (agent.Message, error) {
	m.calls++
	if !reflect.DeepEqual(request.Tools, m.wantTools) {
		m.t.Fatalf("model received unexpected tool definitions: got=%+v want=%+v", request.Tools, m.wantTools)
	}

	request.Tools[0].Name = "mutated"
	request.Tools[0].InputSchema["type"] = "array"
	request.Tools[0].InputSchema["added"] = map[string]any{"flag": true}
	properties := mustReactToolMap(m.t, request.Tools[0].InputSchema["properties"])
	query := mustReactToolMap(m.t, properties["query"])
	enum := mustReactToolSlice(m.t, query["enum"])
	enum[0] = "mutated-alpha"
	query["extra"] = "x"
	required := mustReactToolSlice(m.t, request.Tools[0].InputSchema["required"])
	required[0] = "changed-required"

	return agent.Message{
		Role:    agent.RoleAssistant,
		Content: "done",
	}, nil
}

type noopToolExecutor struct{}

func (noopToolExecutor) Execute(context.Context, agent.ToolCall) (agent.ToolResult, error) {
	return agent.ToolResult{}, nil
}

func mustReactToolMap(t *testing.T, value any) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return m
}

func mustReactToolSlice(t *testing.T, value any) []any {
	t.Helper()

	s, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", value)
	}
	return s
}
