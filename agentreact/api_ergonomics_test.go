package agentreact_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"agentruntime/agent"
	"agentruntime/agentreact"
)

func TestAPIErgonomics_WrapperRunPath(t *testing.T) {
	t.Parallel()

	runner, _, _ := newExampleRuntime(t, "api-run", []response{
		{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "done",
			},
		},
	}, nil)

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "hello",
		MaxSteps:   2,
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Output != "done" {
		t.Fatalf("unexpected output: %q", result.State.Output)
	}
}

func TestAPIErgonomics_ExplicitDispatchStartPath(t *testing.T) {
	t.Parallel()

	runner, _, events := newExampleRuntime(t, "api-dispatch-start", []response{
		{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "started",
			},
		},
	}, nil)

	result, err := runner.Dispatch(context.Background(), agent.StartCommand{Input: agent.RunInput{
		RunID:      "dispatch-start-run",
		UserPrompt: "start",
		MaxSteps:   2,
	}})
	if err != nil {
		t.Fatalf("dispatch start returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Output != "started" {
		t.Fatalf("unexpected output: %q", result.State.Output)
	}

	gotEvents := events.Events()
	if len(gotEvents) == 0 {
		t.Fatalf("expected command events")
	}
	last := gotEvents[len(gotEvents)-1]
	if last.Type != agent.EventTypeCommandApplied || last.CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected last event: %+v", last)
	}
}

func TestAPIErgonomics_ContinueAfterMaxStepsPath(t *testing.T) {
	t.Parallel()

	runner, _, _ := newExampleRuntime(t, "api-continue", []response{
		{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "lookup"},
				},
			},
		},
		{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "done after continue",
			},
		},
	}, map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-value", nil
		},
	})

	initial, initialErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "continue-run",
		UserPrompt: "start",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(initialErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", initialErr)
	}
	if initial.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected initial status: %s", initial.State.Status)
	}

	continued, err := runner.Continue(context.Background(), initial.State.ID, 3, []agent.ToolDefinition{{Name: "lookup"}})
	if err != nil {
		t.Fatalf("continue returned error: %v", err)
	}
	if continued.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected continued status: %s", continued.State.Status)
	}
	if continued.State.Output != "done after continue" {
		t.Fatalf("unexpected continued output: %q", continued.State.Output)
	}
}

func TestAPIErgonomics_SteerThenFollowUpPath(t *testing.T) {
	t.Parallel()

	runner, _, _ := newExampleRuntime(t, "api-steer-followup", []response{
		{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "lookup"},
				},
			},
		},
		{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "final follow up answer",
			},
		},
	}, map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-value", nil
		},
	})

	initial, initialErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "steer-followup-run",
		UserPrompt: "start",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(initialErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", initialErr)
	}

	prefix := agent.CloneMessages(initial.State.Messages)
	steered, err := runner.Steer(context.Background(), initial.State.ID, "new direction")
	if err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	if len(steered.State.Messages) != len(prefix)+1 {
		t.Fatalf("unexpected steered transcript size: got=%d want=%d", len(steered.State.Messages), len(prefix)+1)
	}
	if !reflect.DeepEqual(steered.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("steer mutated transcript prefix")
	}

	steerPrefix := agent.CloneMessages(steered.State.Messages)
	followed, err := runner.FollowUp(context.Background(), initial.State.ID, "follow up prompt", 3, []agent.ToolDefinition{{Name: "lookup"}})
	if err != nil {
		t.Fatalf("follow up returned error: %v", err)
	}
	if followed.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected follow-up status: %s", followed.State.Status)
	}
	if followed.State.Output != "final follow up answer" {
		t.Fatalf("unexpected follow-up output: %q", followed.State.Output)
	}
	if len(followed.State.Messages) <= len(steerPrefix) {
		t.Fatalf("expected transcript growth after follow up")
	}
	if !reflect.DeepEqual(followed.State.Messages[:len(steerPrefix)], steerPrefix) {
		t.Fatalf("follow up mutated transcript prefix")
	}
}

func TestAPIErgonomics_CancelPath(t *testing.T) {
	t.Parallel()

	runner, _, events := newExampleRuntime(t, "api-cancel", []response{
		{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "lookup"},
				},
			},
		},
	}, map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-value", nil
		},
	})

	initial, initialErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "cancel-run",
		UserPrompt: "start",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(initialErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", initialErr)
	}

	cancelled, err := runner.Cancel(context.Background(), initial.State.ID)
	if err != nil {
		t.Fatalf("cancel returned error: %v", err)
	}
	if cancelled.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected cancelled status: %s", cancelled.State.Status)
	}

	gotEvents := events.Events()
	if len(gotEvents) < 2 {
		t.Fatalf("expected cancel command events")
	}
	last := gotEvents[len(gotEvents)-1]
	if last.Type != agent.EventTypeCommandApplied || last.CommandKind != agent.CommandKindCancel {
		t.Fatalf("unexpected last event: %+v", last)
	}
}

func newExampleRuntime(t *testing.T, idPrefix string, responses []response, toolHandlers map[string]handler) (*agent.Runner, *runStore, *eventSink) {
	t.Helper()

	model := newScriptedModel(responses...)
	registry := newRegistry(toolHandlers)
	events := newEventSink()
	store := newRunStore()
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator(idPrefix),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner, store, events
}
