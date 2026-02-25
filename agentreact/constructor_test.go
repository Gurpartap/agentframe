package agentreact_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
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

func TestExecute_NilContextFailsFastWithoutEventEmission(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "unexpected",
		},
	})
	events := newEventSink()
	loop, err := agentreact.New(model, newRegistry(nil), events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}

	initial := agent.RunState{
		ID:     "nil-context-execute",
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hello"},
		},
	}
	next, execErr := loop.Execute(nil, initial, agent.EngineInput{MaxSteps: 2})
	if !errors.Is(execErr, agent.ErrContextNil) {
		t.Fatalf("expected ErrContextNil, got %v", execErr)
	}
	if !reflect.DeepEqual(next, initial) {
		t.Fatalf("state changed on nil-context rejection: got=%+v want=%+v", next, initial)
	}
	if model.index != 0 {
		t.Fatalf("model should not be invoked on nil-context rejection, calls=%d", model.index)
	}
	if gotEvents := events.Events(); len(gotEvents) != 0 {
		t.Fatalf("expected no events on nil-context rejection, got %d", len(gotEvents))
	}
}

func TestExecute_InvalidToolDefinitionsFailFastWithoutSideEffects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		tools   []agent.ToolDefinition
		wantErr string
	}{
		{
			name: "empty tool name",
			tools: []agent.ToolDefinition{
				{Name: ""},
			},
			wantErr: "tool definitions are invalid: index=0 reason=empty_name",
		},
		{
			name: "duplicate tool names",
			tools: []agent.ToolDefinition{
				{Name: "lookup"},
				{Name: "lookup"},
			},
			wantErr: "tool definitions are invalid: index=1 name=\"lookup\" reason=duplicate_name first_index=0",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := newScriptedModel(response{
				Message: agent.Message{
					Role:    agent.RoleAssistant,
					Content: "unexpected",
				},
			})
			executor := &countingToolExecutor{}
			events := newEventSink()
			loop, err := agentreact.New(model, executor, events)
			if err != nil {
				t.Fatalf("new loop: %v", err)
			}

			initial := agent.RunState{
				ID:     "invalid-tools-execute",
				Status: agent.RunStatusPending,
				Messages: []agent.Message{
					{Role: agent.RoleUser, Content: "hello"},
				},
			}
			next, execErr := loop.Execute(context.Background(), initial, agent.EngineInput{
				MaxSteps: 2,
				Tools:    tc.tools,
			})
			if !errors.Is(execErr, agent.ErrToolDefinitionsInvalid) {
				t.Fatalf("expected ErrToolDefinitionsInvalid, got %v", execErr)
			}
			if execErr == nil || execErr.Error() != tc.wantErr {
				t.Fatalf("unexpected error text: got=%v want=%q", execErr, tc.wantErr)
			}
			if !reflect.DeepEqual(next, initial) {
				t.Fatalf("state changed on invalid tool definitions: got=%+v want=%+v", next, initial)
			}
			if model.index != 0 {
				t.Fatalf("model should not be invoked on invalid tool definitions, calls=%d", model.index)
			}
			if executor.calls.Load() != 0 {
				t.Fatalf("executor should not be invoked on invalid tool definitions, calls=%d", executor.calls.Load())
			}
			if gotEvents := events.Events(); len(gotEvents) != 0 {
				t.Fatalf("expected no events on invalid tool definitions rejection, got %d", len(gotEvents))
			}
		})
	}
}

type countingToolExecutor struct {
	calls atomic.Int32
}

func (e *countingToolExecutor) Execute(context.Context, agent.ToolCall) (agent.ToolResult, error) {
	e.calls.Add(1)
	return agent.ToolResult{Content: "unexpected"}, nil
}
