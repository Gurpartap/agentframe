package agent_test

import (
	"context"
	"reflect"
	"testing"

	"agentruntime/agent"
	eventinginmem "agentruntime/eventing/inmem"
	runstoreinmem "agentruntime/runstore/inmem"
)

func TestRunnerEngineInputToolDefinitionsRemainImmutable(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		runID agent.RunID
		seed  func(*testing.T, *runstoreinmem.Store)
		call  func(*agent.Runner, []agent.ToolDefinition) (agent.RunResult, error)
	}

	cases := []testCase{
		{
			name:  "start",
			runID: "immut-tools-start",
			call: func(runner *agent.Runner, tools []agent.ToolDefinition) (agent.RunResult, error) {
				return runner.Run(context.Background(), agent.RunInput{
					RunID:      "immut-tools-start",
					UserPrompt: "start",
					MaxSteps:   3,
					Tools:      tools,
				})
			},
		},
		{
			name:  "continue",
			runID: "immut-tools-continue",
			seed: func(t *testing.T, store *runstoreinmem.Store) {
				t.Helper()
				if err := store.Save(context.Background(), agent.RunState{
					ID:     "immut-tools-continue",
					Status: agent.RunStatusPending,
					Step:   2,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "continue"},
					},
				}); err != nil {
					t.Fatalf("seed continue state: %v", err)
				}
			},
			call: func(runner *agent.Runner, tools []agent.ToolDefinition) (agent.RunResult, error) {
				return runner.Continue(context.Background(), "immut-tools-continue", 3, tools, nil)
			},
		},
		{
			name:  "follow_up",
			runID: "immut-tools-follow-up",
			seed: func(t *testing.T, store *runstoreinmem.Store) {
				t.Helper()
				if err := store.Save(context.Background(), agent.RunState{
					ID:     "immut-tools-follow-up",
					Status: agent.RunStatusPending,
					Step:   2,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "seed"},
					},
				}); err != nil {
					t.Fatalf("seed follow-up state: %v", err)
				}
			},
			call: func(runner *agent.Runner, tools []agent.ToolDefinition) (agent.RunResult, error) {
				return runner.FollowUp(context.Background(), "immut-tools-follow-up", "follow up", 3, tools)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tools := nestedToolDefinitionsForMutationIsolation()
			wantTools := agent.CloneToolDefinitions(tools)

			store := runstoreinmem.New()
			if tc.seed != nil {
				tc.seed(t, store)
			}
			events := eventinginmem.New()
			engine := &engineSpy{
				executeFn: func(_ context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
					mutateToolDefinitionsForIsolationTest(t, input.Tools)

					next := state
					next.Step++
					next.Status = agent.RunStatusCompleted
					next.Output = "done"
					return next, nil
				},
			}
			runner := newDispatchRunnerWithEngine(t, store, events, engine)

			result, err := tc.call(runner, tools)
			if err != nil {
				t.Fatalf("call returned error: %v", err)
			}
			if result.State.Status != agent.RunStatusCompleted {
				t.Fatalf("unexpected status: %s", result.State.Status)
			}
			if engine.calls != 1 {
				t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
			}
			if !reflect.DeepEqual(tools, wantTools) {
				t.Fatalf("caller tool definitions mutated: got=%+v want=%+v", tools, wantTools)
			}
		})
	}
}

func nestedToolDefinitionsForMutationIsolation() []agent.ToolDefinition {
	return []agent.ToolDefinition{
		{
			Name:        "lookup",
			Description: "search",
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
}

func mutateToolDefinitionsForIsolationTest(t *testing.T, tools []agent.ToolDefinition) {
	t.Helper()

	if len(tools) == 0 {
		t.Fatalf("expected tools for mutation test")
	}
	tools[0].Name = "mutated-name"
	tools[0].InputSchema["type"] = "array"
	tools[0].InputSchema["added"] = map[string]any{"flag": true}

	properties := mustToolSchemaMap(t, tools[0].InputSchema["properties"])
	query := mustToolSchemaMap(t, properties["query"])
	enum := mustToolSchemaSlice(t, query["enum"])
	enum[0] = "mutated-alpha"
	query["extra"] = "x"

	required := mustToolSchemaSlice(t, tools[0].InputSchema["required"])
	required[0] = "changed-required"
}

func mustToolSchemaMap(t *testing.T, value any) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return m
}

func mustToolSchemaSlice(t *testing.T, value any) []any {
	t.Helper()

	s, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", value)
	}
	return s
}
