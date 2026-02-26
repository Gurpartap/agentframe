package agentreact_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

func TestRunnerRun_CompletesAfterToolObservation(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
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
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Final answer after tool observation.",
			},
		},
	)
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, args map[string]any) (string, error) {
			return "tool_result_for=" + args["q"].(string), nil
		},
	})
	events := newEventSink()
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("test"),
		RunStore:    newRunStore(),
		Engine:      loop,
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
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
	if len(result.State.Messages) != 5 {
		t.Fatalf("unexpected message count: %d", len(result.State.Messages))
	}
}

func TestRunnerRun_MaxStepsExceeded(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
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
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "value", nil
		},
	})
	events := newEventSink()
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("test"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
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
	if result.State.Version != 2 {
		t.Fatalf("unexpected version: %d", result.State.Version)
	}
	wantDescription := "run failed: " + agent.ErrMaxStepsExceeded.Error()
	runFailedEvents := 0
	for _, event := range events.Events() {
		if event.Type != agent.EventTypeRunFailed {
			continue
		}
		runFailedEvents++
		if event.Description != wantDescription {
			t.Fatalf("unexpected run_failed description: got=%q want=%q", event.Description, wantDescription)
		}
	}
	if runFailedEvents != 1 {
		t.Fatalf("unexpected run_failed event count: got=%d want=1", runFailedEvents)
	}
}
