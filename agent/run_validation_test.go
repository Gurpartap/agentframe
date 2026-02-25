package agent_test

import (
	"errors"
	"testing"

	"agentruntime/agent"
)

func TestValidateRunStateMatrix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		state            agent.RunState
		wantErr          bool
		wantInvalidRunID bool
	}{
		{
			name: "valid pending",
			state: agent.RunState{
				ID:      "run-valid-1",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatusPending,
			},
		},
		{
			name: "valid completed",
			state: agent.RunState{
				ID:      "run-valid-2",
				Version: 3,
				Step:    5,
				Status:  agent.RunStatusCompleted,
			},
		},
		{
			name: "empty id",
			state: agent.RunState{
				ID:      "",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatusPending,
			},
			wantErr:          true,
			wantInvalidRunID: true,
		},
		{
			name: "negative step",
			state: agent.RunState{
				ID:      "run-negative-step",
				Version: 0,
				Step:    -1,
				Status:  agent.RunStatusPending,
			},
			wantErr: true,
		},
		{
			name: "negative version",
			state: agent.RunState{
				ID:      "run-negative-version",
				Version: -1,
				Step:    0,
				Status:  agent.RunStatusPending,
			},
			wantErr: true,
		},
		{
			name: "empty status",
			state: agent.RunState{
				ID:      "run-empty-status",
				Version: 0,
				Step:    0,
				Status:  "",
			},
			wantErr: true,
		},
		{
			name: "unknown status",
			state: agent.RunState{
				ID:      "run-unknown-status",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatus("mystery"),
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := agent.ValidateRunState(tc.state)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if !errors.Is(err, agent.ErrRunStateInvalid) {
				t.Fatalf("expected ErrRunStateInvalid, got %v", err)
			}
			if tc.wantInvalidRunID && !errors.Is(err, agent.ErrInvalidRunID) {
				t.Fatalf("expected ErrInvalidRunID compatibility, got %v", err)
			}
		})
	}
}
