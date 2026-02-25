package agentreact_test

import (
	"context"
	"errors"
	"testing"

	"agentruntime/agent"
	"agentruntime/agentreact"
)

func TestNew_ValidatesRequiredDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		model   agentreact.Model
		tools   agentreact.ToolExecutor
		wantErr error
	}{
		{
			name:    "nil model",
			model:   nil,
			tools:   newRegistry(nil),
			wantErr: agentreact.ErrMissingModel,
		},
		{
			name:    "nil tool executor",
			model:   newScriptedModel(),
			tools:   nil,
			wantErr: agentreact.ErrMissingToolExecutor,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			loop, err := agentreact.New(tc.model, tc.tools, newEventSink())
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if loop != nil {
				t.Fatalf("expected nil loop on constructor error")
			}
		})
	}
}

func TestNew_NilEventSinkDefaultsToNoop(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "done",
		},
	})
	loop, err := agentreact.New(model, newRegistry(nil), nil)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	if loop == nil {
		t.Fatalf("expected loop")
	}

	state, execErr := loop.Execute(context.Background(), agent.RunState{
		ID:     "constructor-event-sink",
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{
				Role:    agent.RoleUser,
				Content: "hello",
			},
		},
	}, agent.EngineInput{MaxSteps: 1})
	if execErr != nil {
		t.Fatalf("execute with default event sink: %v", execErr)
	}
	if state.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", state.Status)
	}
	if state.Output != "done" {
		t.Fatalf("unexpected output: %q", state.Output)
	}
}
