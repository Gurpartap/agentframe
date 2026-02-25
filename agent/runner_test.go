package agent_test

import (
	"context"
	"errors"
	"testing"

	"agentruntime/adapters/inmem"
	"agentruntime/adapters/modeltest"
	"agentruntime/adapters/tools"
	"agentruntime/agent"
)

func TestRunnerRun_CompletesAfterToolObservation(t *testing.T) {
	t.Parallel()

	model := modeltest.NewScriptedModel(
		modeltest.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "I should call a tool first.",
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
		modeltest.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Final answer after tool observation.",
			},
		},
	)
	registry := tools.NewRegistry(map[string]tools.Handler{
		"lookup": func(_ context.Context, args map[string]any) (string, error) {
			return "tool_result_for=" + args["q"].(string), nil
		},
	})
	events := inmem.NewEventSink()
	loop, err := agent.NewReactLoop(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: inmem.NewCounterIDGenerator("test"),
		RunStore:    inmem.NewRunStore(),
		ReactLoop:   loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Run(context.Background(), agent.RunInput{
		SystemPrompt: "Be concise.",
		UserPrompt:   "Find info about Go.",
		MaxSteps:     4,
		Tools: []agent.ToolDefinition{
			{Name: "lookup", Description: "Look up information"},
		},
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Output != "Final answer after tool observation." {
		t.Fatalf("unexpected output: %q", result.State.Output)
	}
	if len(result.State.Messages) != 5 {
		t.Fatalf("unexpected message count: %d", len(result.State.Messages))
	}
}

func TestRunnerRun_MaxStepsExceeded(t *testing.T) {
	t.Parallel()

	model := modeltest.NewScriptedModel(
		modeltest.Response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Need tool.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
					},
				},
			},
		},
	)
	registry := tools.NewRegistry(map[string]tools.Handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "value", nil
		},
	})
	loop, err := agent.NewReactLoop(model, registry, inmem.NewEventSink())
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: inmem.NewCounterIDGenerator("test"),
		RunStore:    inmem.NewRunStore(),
		ReactLoop:   loop,
		EventSink:   inmem.NewEventSink(),
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Do a tool thing.",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(err, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got: %v", err)
	}
	if result.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
}
