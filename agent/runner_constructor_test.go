package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
)

func TestNewRunner_ValidatesRequiredDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*agent.Dependencies)
		wantErr error
	}{
		{
			name: "nil ID generator",
			mutate: func(deps *agent.Dependencies) {
				deps.IDGenerator = nil
			},
			wantErr: agent.ErrMissingIDGenerator,
		},
		{
			name: "nil run store",
			mutate: func(deps *agent.Dependencies) {
				deps.RunStore = nil
			},
			wantErr: agent.ErrMissingRunStore,
		},
		{
			name: "nil engine",
			mutate: func(deps *agent.Dependencies) {
				deps.Engine = nil
			},
			wantErr: agent.ErrMissingEngine,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deps := agent.Dependencies{
				IDGenerator: newCounterIDGenerator("constructor"),
				RunStore:    runstoreinmem.New(),
				Engine:      constructorEngine{},
				EventSink:   nil,
			}
			tc.mutate(&deps)

			runner, err := agent.NewRunner(deps)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if runner != nil {
				t.Fatalf("expected nil runner on constructor error")
			}
		})
	}
}

func TestNewRunner_NilEventSinkDefaultsToNoop(t *testing.T) {
	t.Parallel()

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("constructor"),
		RunStore:    runstoreinmem.New(),
		Engine:      constructorEngine{},
		EventSink:   nil,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if runner == nil {
		t.Fatalf("expected runner")
	}

	if _, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "hello",
		MaxSteps:   1,
	}); err != nil {
		t.Fatalf("run with default event sink: %v", err)
	}
}

type constructorEngine struct{}

func (constructorEngine) Execute(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
	return state, nil
}
