package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
)

func TestRunnerDispatch_EngineOutputContractViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "start_run_id_mismatch",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-start-id-mismatch")
				events := eventinginmem.New()
				store := runstoreinmem.New()
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.ID = "unexpected-run-id"
						next.Step++
						next.Status = agent.RunStatusCompleted
						next.Output = "corrupt"
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, err := runner.Run(context.Background(), agent.RunInput{
					RunID:        runID,
					SystemPrompt: "system",
					UserPrompt:   "start",
					MaxSteps:     2,
				})
				if !errors.Is(err, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", err)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
				}

				persisted, loadErr := store.Load(context.Background(), runID)
				if loadErr != nil {
					t.Fatalf("load persisted state: %v", loadErr)
				}
				want := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Messages: []agent.Message{
						{Role: agent.RoleSystem, Content: "system"},
						{Role: agent.RoleUser, Content: "start"},
					},
					Version: 1,
				}
				if !reflect.DeepEqual(persisted, want) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persisted, want)
				}

				gotEvents := events.Events()
				if countEventType(gotEvents, agent.EventTypeRunStarted) != 1 {
					t.Fatalf("expected exactly one run_started event, got=%d", countEventType(gotEvents, agent.EventTypeRunStarted))
				}
				assertNoCheckpointOrCommandAppliedEvents(t, gotEvents)
			},
		},
		{
			name: "continue_step_regression",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-continue-step-regression")
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Step:   4,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "continue"},
					},
				}

				events := eventinginmem.New()
				store := runstoreinmem.New()
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed store: %v", err)
				}
				persistedBefore, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load initial state: %v", err)
				}
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step = state.Step - 1
						next.Status = agent.RunStatusCompleted
						next.Output = "corrupt"
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, runErr := runner.Continue(context.Background(), runID, 3, nil, nil)
				if !errors.Is(runErr, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", runErr)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
				}

				persistedAfter, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load persisted state: %v", err)
				}
				if !reflect.DeepEqual(persistedAfter, persistedBefore) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persistedAfter, persistedBefore)
				}
				assertNoCheckpointOrCommandAppliedEvents(t, events.Events())
			},
		},
		{
			name: "follow_up_transcript_prefix_changed",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-follow-up-prefix-changed")
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Step:   2,
					Messages: []agent.Message{
						{Role: agent.RoleSystem, Content: "system"},
						{Role: agent.RoleUser, Content: "seed"},
					},
				}

				events := eventinginmem.New()
				store := runstoreinmem.New()
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed store: %v", err)
				}
				persistedBefore, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load initial state: %v", err)
				}
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step = state.Step + 1
						next.Status = agent.RunStatusCompleted
						next.Output = "corrupt"
						next.Messages = append([]agent.Message(nil), state.Messages...)
						next.Messages[0].Content = "rewritten"
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, runErr := runner.FollowUp(context.Background(), runID, "follow up", 3, nil)
				if !errors.Is(runErr, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", runErr)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}
				if engine.calls != 1 {
					t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
				}

				persistedAfter, err := store.Load(context.Background(), runID)
				if err != nil {
					t.Fatalf("load persisted state: %v", err)
				}
				if !reflect.DeepEqual(persistedAfter, persistedBefore) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persistedAfter, persistedBefore)
				}
				assertNoCheckpointOrCommandAppliedEvents(t, events.Events())
			},
		},
		{
			name: "start_suspended_model_origin_requires_assistant_requirement_evidence",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-start-model-origin-provenance")
				events := eventinginmem.New()
				store := runstoreinmem.New()
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step++
						next.Status = agent.RunStatusSuspended
						next.PendingRequirement = &agent.PendingRequirement{
							ID:     "req-model",
							Kind:   agent.RequirementKindApproval,
							Origin: agent.RequirementOriginModel,
						}
						next.Messages = append(next.Messages, agent.Message{
							Role:    agent.RoleAssistant,
							Content: "missing requirement payload",
						})
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, err := runner.Run(context.Background(), agent.RunInput{
					RunID:      runID,
					UserPrompt: "start",
					MaxSteps:   2,
				})
				if !errors.Is(err, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", err)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}

				persisted, loadErr := store.Load(context.Background(), runID)
				if loadErr != nil {
					t.Fatalf("load persisted state: %v", loadErr)
				}
				want := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "start"},
					},
					Version: 1,
				}
				if !reflect.DeepEqual(persisted, want) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persisted, want)
				}
				assertNoCheckpointOrCommandAppliedEvents(t, events.Events())
			},
		},
		{
			name: "start_suspended_tool_origin_requires_tool_observation_evidence",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-start-tool-origin-provenance")
				events := eventinginmem.New()
				store := runstoreinmem.New()
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step++
						next.Status = agent.RunStatusSuspended
						next.PendingRequirement = &agent.PendingRequirement{
							ID:         "req-tool",
							Kind:       agent.RequirementKindUserInput,
							Origin:     agent.RequirementOriginTool,
							ToolCallID: "call-required",
						}
						next.Messages = append(next.Messages, agent.Message{
							Role:    agent.RoleAssistant,
							Content: "still no tool observation",
							ToolCalls: []agent.ToolCall{
								{
									ID:   "call-required",
									Name: "lookup",
								},
							},
						})
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, err := runner.Run(context.Background(), agent.RunInput{
					RunID:      runID,
					UserPrompt: "start",
					MaxSteps:   2,
				})
				if !errors.Is(err, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", err)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}

				persisted, loadErr := store.Load(context.Background(), runID)
				if loadErr != nil {
					t.Fatalf("load persisted state: %v", loadErr)
				}
				want := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "start"},
					},
					Version: 1,
				}
				if !reflect.DeepEqual(persisted, want) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persisted, want)
				}
				assertNoCheckpointOrCommandAppliedEvents(t, events.Events())
			},
		},
		{
			name: "start_suspended_tool_origin_rejects_linked_but_unrelated_tool_evidence",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-start-tool-origin-unrelated-tool-evidence")
				events := eventinginmem.New()
				store := runstoreinmem.New()
				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step++
						next.Status = agent.RunStatusSuspended
						next.PendingRequirement = &agent.PendingRequirement{
							ID:         "req-tool-unrelated",
							Kind:       agent.RequirementKindUserInput,
							Origin:     agent.RequirementOriginTool,
							ToolCallID: "call-required",
						}
						next.Messages = append(next.Messages, agent.Message{
							Role:    agent.RoleAssistant,
							Content: "issued different tool call",
							ToolCalls: []agent.ToolCall{
								{
									ID:   "call-other",
									Name: "lookup",
								},
							},
						})
						next.Messages = append(next.Messages, agent.Message{
							Role:       agent.RoleTool,
							Name:       "lookup",
							ToolCallID: "call-other",
							Content:    "observation tied to unrelated call",
						})
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, err := runner.Run(context.Background(), agent.RunInput{
					RunID:      runID,
					UserPrompt: "start",
					MaxSteps:   2,
				})
				if !errors.Is(err, agent.ErrEngineOutputContractViolation) {
					t.Fatalf("expected ErrEngineOutputContractViolation, got %v", err)
				}
				if !reflect.DeepEqual(result, agent.RunResult{}) {
					t.Fatalf("unexpected result: %+v", result)
				}

				persisted, loadErr := store.Load(context.Background(), runID)
				if loadErr != nil {
					t.Fatalf("load persisted state: %v", loadErr)
				}
				want := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "start"},
					},
					Version: 1,
				}
				if !reflect.DeepEqual(persisted, want) {
					t.Fatalf("persisted state changed: got=%+v want=%+v", persisted, want)
				}
				assertNoCheckpointOrCommandAppliedEvents(t, events.Events())
			},
		},
		{
			name: "continue_suspended_tool_origin_accepts_prefix_assistant_tool_call_and_delta_observation",
			run: func(t *testing.T) {
				t.Parallel()

				const runID = agent.RunID("contract-continue-tool-origin-prefix-link")
				events := eventinginmem.New()
				store := runstoreinmem.New()
				initial := agent.RunState{
					ID:     runID,
					Status: agent.RunStatusPending,
					Step:   1,
					Messages: []agent.Message{
						{Role: agent.RoleUser, Content: "start"},
						{
							Role:    agent.RoleAssistant,
							Content: "calling tool",
							ToolCalls: []agent.ToolCall{
								{
									ID:   "call-prefix",
									Name: "lookup",
								},
							},
						},
					},
				}
				if err := store.Save(context.Background(), initial); err != nil {
					t.Fatalf("seed store: %v", err)
				}

				engine := &engineSpy{
					executeFn: func(_ context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
						next := state
						next.Step++
						next.Status = agent.RunStatusSuspended
						next.PendingRequirement = &agent.PendingRequirement{
							ID:         "req-tool-prefix",
							Kind:       agent.RequirementKindUserInput,
							Origin:     agent.RequirementOriginTool,
							ToolCallID: "call-prefix",
						}
						next.Messages = append(next.Messages, agent.Message{
							Role:       agent.RoleTool,
							Name:       "lookup",
							ToolCallID: "call-prefix",
							Content:    "need user input",
						})
						return next, nil
					},
				}
				runner := newDispatchRunnerWithEngine(t, store, events, engine)

				result, err := runner.Continue(context.Background(), runID, 3, nil, nil)
				if err != nil {
					t.Fatalf("continue returned error: %v", err)
				}
				if result.State.Status != agent.RunStatusSuspended {
					t.Fatalf("unexpected status: got=%s want=%s", result.State.Status, agent.RunStatusSuspended)
				}
				if result.State.PendingRequirement == nil {
					t.Fatalf("expected pending requirement")
				}
				if result.State.PendingRequirement.ToolCallID != "call-prefix" {
					t.Fatalf("unexpected pending requirement tool call id: %q", result.State.PendingRequirement.ToolCallID)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, tc.run)
	}
}

func assertNoCheckpointOrCommandAppliedEvents(t *testing.T, events []agent.Event) {
	t.Helper()

	for _, event := range events {
		if event.Type == agent.EventTypeRunCheckpoint || event.Type == agent.EventTypeCommandApplied {
			t.Fatalf("unexpected event type on rejected output: %s", event.Type)
		}
	}
}

func countEventType(events []agent.Event, want agent.EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == want {
			count++
		}
	}
	return count
}
