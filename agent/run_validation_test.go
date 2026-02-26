package agent_test

import (
	"errors"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
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
			name: "valid suspended with pending requirement",
			state: agent.RunState{
				ID:      "run-valid-suspended",
				Version: 2,
				Step:    3,
				Status:  agent.RunStatusSuspended,
				PendingRequirement: &agent.PendingRequirement{
					ID:     "req-1",
					Kind:   agent.RequirementKindApproval,
					Origin: agent.RequirementOriginModel,
				},
			},
		},
		{
			name: "valid suspended with tool-origin requirement linkage",
			state: agent.RunState{
				ID:      "run-valid-suspended-tool-origin",
				Version: 2,
				Step:    3,
				Status:  agent.RunStatusSuspended,
				PendingRequirement: &agent.PendingRequirement{
					ID:          "req-tool",
					Kind:        agent.RequirementKindUserInput,
					Origin:      agent.RequirementOriginTool,
					ToolCallID:  "call-1",
					Fingerprint: "fp-call-1",
				},
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
		{
			name: "suspended missing pending requirement",
			state: agent.RunState{
				ID:      "run-suspended-missing-requirement",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatusSuspended,
			},
			wantErr: true,
		},
		{
			name: "suspended tool-origin requirement missing linkage",
			state: agent.RunState{
				ID:      "run-suspended-tool-missing-linkage",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatusSuspended,
				PendingRequirement: &agent.PendingRequirement{
					ID:     "req-tool",
					Kind:   agent.RequirementKindUserInput,
					Origin: agent.RequirementOriginTool,
				},
			},
			wantErr: true,
		},
		{
			name: "suspended tool-origin requirement missing fingerprint",
			state: agent.RunState{
				ID:      "run-suspended-tool-missing-fingerprint",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatusSuspended,
				PendingRequirement: &agent.PendingRequirement{
					ID:         "req-tool",
					Kind:       agent.RequirementKindUserInput,
					Origin:     agent.RequirementOriginTool,
					ToolCallID: "call-1",
				},
			},
			wantErr: true,
		},
		{
			name: "running with pending requirement",
			state: agent.RunState{
				ID:      "run-running-with-requirement",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatusRunning,
				PendingRequirement: &agent.PendingRequirement{
					ID:     "req-1",
					Kind:   agent.RequirementKindApproval,
					Origin: agent.RequirementOriginModel,
				},
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

func TestValidateRunState_SuspendedRequiresRequirementOrigin(t *testing.T) {
	t.Parallel()

	state := agent.RunState{
		ID:      "run-suspended-missing-origin",
		Version: 1,
		Step:    2,
		Status:  agent.RunStatusSuspended,
		PendingRequirement: &agent.PendingRequirement{
			ID:   "req-approval",
			Kind: agent.RequirementKindApproval,
		},
	}

	err := agent.ValidateRunState(state)
	if !errors.Is(err, agent.ErrRunStateInvalid) {
		t.Fatalf("expected ErrRunStateInvalid, got %v", err)
	}
}

func TestValidateRunState_RejectsUnknownRequirementOrigin(t *testing.T) {
	t.Parallel()

	state := agent.RunState{
		ID:      "run-suspended-unknown-origin",
		Version: 1,
		Step:    2,
		Status:  agent.RunStatusSuspended,
		PendingRequirement: &agent.PendingRequirement{
			ID:     "req-approval",
			Kind:   agent.RequirementKindApproval,
			Origin: agent.RequirementOrigin("mystery"),
		},
	}

	err := agent.ValidateRunState(state)
	if !errors.Is(err, agent.ErrRunStateInvalid) {
		t.Fatalf("expected ErrRunStateInvalid, got %v", err)
	}
}
