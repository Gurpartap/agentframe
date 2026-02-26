package agent_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
)

func TestRunnerDispatch_StartWrapperParity(t *testing.T) {
	t.Parallel()

	input := agent.RunInput{
		RunID:        "dispatch-start-run",
		SystemPrompt: "Be concise.",
		UserPrompt:   "hello",
		MaxSteps:     3,
	}
	wrapperRunner := newDispatchRunner(t, runstoreinmem.New(), eventinginmem.New(), response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
	})
	dispatchRunner := newDispatchRunner(t, runstoreinmem.New(), eventinginmem.New(), response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "done"},
	})

	wrapperResult, wrapperErr := wrapperRunner.Run(context.Background(), input)
	if wrapperErr != nil {
		t.Fatalf("run wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.StartCommand{Input: input})
	if dispatchErr != nil {
		t.Fatalf("dispatch start error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("start wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if dispatchResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected dispatch status: %s", dispatchResult.State.Status)
	}
	if dispatchResult.State.Version != 2 {
		t.Fatalf("unexpected dispatch version: %d", dispatchResult.State.Version)
	}
}

func TestRunnerDispatch_ContinueWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-continue-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "continue"},
		},
	}

	wrapperStore := runstoreinmem.New()
	if err := wrapperStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed wrapper store: %v", err)
	}
	dispatchStore := runstoreinmem.New()
	if err := dispatchStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed dispatch store: %v", err)
	}

	wrapperRunner := newDispatchRunner(t, wrapperStore, eventinginmem.New(), response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "continued"},
	})
	dispatchRunner := newDispatchRunner(t, dispatchStore, eventinginmem.New(), response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "continued"},
	})

	wrapperResult, wrapperErr := wrapperRunner.Continue(context.Background(), runID, 3, nil, nil)
	if wrapperErr != nil {
		t.Fatalf("continue wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    runID,
		MaxSteps: 3,
	})
	if dispatchErr != nil {
		t.Fatalf("dispatch continue error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("continue wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if dispatchResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected dispatch status: %s", dispatchResult.State.Status)
	}
	if dispatchResult.State.Step != 1 {
		t.Fatalf("unexpected dispatch step: %d", dispatchResult.State.Step)
	}
	if dispatchResult.State.Version != 2 {
		t.Fatalf("unexpected dispatch version: %d", dispatchResult.State.Version)
	}
}

func TestRunnerDispatch_CancelWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-cancel-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusRunning,
		Step:   7,
	}

	wrapperStore := runstoreinmem.New()
	if err := wrapperStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed wrapper store: %v", err)
	}
	dispatchStore := runstoreinmem.New()
	if err := dispatchStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed dispatch store: %v", err)
	}

	wrapperRunner := newDispatchRunner(t, wrapperStore, eventinginmem.New(), response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "unused"},
	})
	dispatchRunner := newDispatchRunner(t, dispatchStore, eventinginmem.New(), response{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "unused"},
	})

	wrapperResult, wrapperErr := wrapperRunner.Cancel(context.Background(), runID)
	if wrapperErr != nil {
		t.Fatalf("cancel wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.CancelCommand{RunID: runID})
	if dispatchErr != nil {
		t.Fatalf("dispatch cancel error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("cancel wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if dispatchResult.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected dispatch status: %s", dispatchResult.State.Status)
	}
	if dispatchResult.State.Step != initial.Step {
		t.Fatalf("unexpected dispatch step: got=%d want=%d", dispatchResult.State.Step, initial.Step)
	}
	if dispatchResult.State.Version != 2 {
		t.Fatalf("unexpected dispatch version: %d", dispatchResult.State.Version)
	}
}

func TestRunnerDispatch_SteerWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-steer-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   3,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}

	wrapperStore := runstoreinmem.New()
	if err := wrapperStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed wrapper store: %v", err)
	}
	dispatchStore := runstoreinmem.New()
	if err := dispatchStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed dispatch store: %v", err)
	}

	wrapperEvents := eventinginmem.New()
	wrapperEngine := &engineSpy{}
	wrapperRunner := newDispatchRunnerWithEngine(t, wrapperStore, wrapperEvents, wrapperEngine)
	dispatchEvents := eventinginmem.New()
	dispatchEngine := &engineSpy{}
	dispatchRunner := newDispatchRunnerWithEngine(t, dispatchStore, dispatchEvents, dispatchEngine)

	wrapperResult, wrapperErr := wrapperRunner.Steer(context.Background(), runID, "steer now")
	if wrapperErr != nil {
		t.Fatalf("steer wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.SteerCommand{
		RunID:       runID,
		Instruction: "steer now",
	})
	if dispatchErr != nil {
		t.Fatalf("dispatch steer error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("steer wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	if wrapperEngine.calls != 0 || dispatchEngine.calls != 0 {
		t.Fatalf("steer must not invoke engine: wrapper_calls=%d dispatch_calls=%d", wrapperEngine.calls, dispatchEngine.calls)
	}
	assertEventTypes(t, wrapperEvents.Events(), []agent.EventType{
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertEventTypes(t, dispatchEvents.Events(), []agent.EventType{
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertCommandKind(t, wrapperEvents.Events(), agent.CommandKindSteer)
	assertCommandKind(t, dispatchEvents.Events(), agent.CommandKindSteer)
}

func TestRunnerDispatch_FollowUpWrapperParity(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-followup-parity")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}

	wrapperStore := runstoreinmem.New()
	if err := wrapperStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed wrapper store: %v", err)
	}
	dispatchStore := runstoreinmem.New()
	if err := dispatchStore.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed dispatch store: %v", err)
	}

	wrapperEvents := eventinginmem.New()
	wrapperRunner := newDispatchRunner(
		t,
		wrapperStore,
		wrapperEvents,
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "follow-up done",
			},
		},
	)
	dispatchEvents := eventinginmem.New()
	dispatchRunner := newDispatchRunner(
		t,
		dispatchStore,
		dispatchEvents,
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "follow-up done",
			},
		},
	)

	wrapperResult, wrapperErr := wrapperRunner.FollowUp(context.Background(), runID, "next prompt", 4, nil)
	if wrapperErr != nil {
		t.Fatalf("follow up wrapper error: %v", wrapperErr)
	}
	dispatchResult, dispatchErr := dispatchRunner.Dispatch(context.Background(), agent.FollowUpCommand{
		RunID:      runID,
		UserPrompt: "next prompt",
		MaxSteps:   4,
	})
	if dispatchErr != nil {
		t.Fatalf("dispatch follow up error: %v", dispatchErr)
	}

	if !reflect.DeepEqual(dispatchResult, wrapperResult) {
		t.Fatalf("follow up wrapper/dispatch mismatch: wrapper=%+v dispatch=%+v", wrapperResult, dispatchResult)
	}
	assertEventTypes(t, wrapperEvents.Events(), []agent.EventType{
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertEventTypes(t, dispatchEvents.Events(), []agent.EventType{
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	})
	assertCommandKind(t, wrapperEvents.Events(), agent.CommandKindFollowUp)
	assertCommandKind(t, dispatchEvents.Events(), agent.CommandKindFollowUp)
}

func TestRunnerDispatch_InvalidInputMatrix(t *testing.T) {
	t.Parallel()

	const (
		existingRunID = agent.RunID("dispatch-invalid-input-existing-run")
		startRunID    = agent.RunID("dispatch-invalid-input-start-run")
	)
	seedState := agent.RunState{
		ID:     existingRunID,
		Status: agent.RunStatusPending,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}

	newFixture := func(t *testing.T) (*agent.Runner, *runstoreinmem.Store, *eventinginmem.Sink, *engineSpy, agent.RunState) {
		t.Helper()

		store := runstoreinmem.New()
		if err := store.Save(context.Background(), seedState); err != nil {
			t.Fatalf("seed store: %v", err)
		}
		persistedSeed, err := store.Load(context.Background(), existingRunID)
		if err != nil {
			t.Fatalf("load seeded state: %v", err)
		}

		events := eventinginmem.New()
		engine := &engineSpy{}
		runner := newDispatchRunnerWithEngine(t, store, events, engine)
		return runner, store, events, engine, persistedSeed
	}

	var nilCommand agent.Command
	var nilStart *agent.StartCommand
	var nilContinue *agent.ContinueCommand
	var nilCancel *agent.CancelCommand
	var nilSteer *agent.SteerCommand
	var nilFollowUp *agent.FollowUpCommand
	var nilUnknown *unknownCommand
	pointerStart := &agent.StartCommand{
		Input: agent.RunInput{
			RunID:      startRunID,
			UserPrompt: "pointer start",
			MaxSteps:   3,
		},
	}
	pointerContinue := &agent.ContinueCommand{
		RunID:    existingRunID,
		MaxSteps: 3,
	}
	pointerCancel := &agent.CancelCommand{
		RunID: existingRunID,
	}
	pointerSteer := &agent.SteerCommand{
		RunID:       existingRunID,
		Instruction: "pointer steer",
	}
	pointerFollowUp := &agent.FollowUpCommand{
		RunID:      existingRunID,
		UserPrompt: "pointer follow up",
		MaxSteps:   3,
	}
	pointerUnknown := &unknownCommand{}

	type matrixCase struct {
		name        string
		wantErr     error
		call        func(*agent.Runner) (agent.RunResult, error)
		checkAbsent agent.RunID
	}

	cases := []matrixCase{
		{
			name:    "nil_context_wrapper_start",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Run(nil, agent.RunInput{RunID: startRunID, UserPrompt: "hello", MaxSteps: 3})
			},
			checkAbsent: startRunID,
		},
		{
			name:    "nil_context_wrapper_continue",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Continue(nil, existingRunID, 3, nil, nil)
			},
		},
		{
			name:    "nil_context_wrapper_cancel",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Cancel(nil, existingRunID)
			},
		},
		{
			name:    "nil_context_wrapper_steer",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Steer(nil, existingRunID, "new direction")
			},
		},
		{
			name:    "nil_context_wrapper_follow_up",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.FollowUp(nil, existingRunID, "follow up", 3, nil)
			},
		},
		{
			name:    "nil_context_dispatch_start",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(nil, agent.StartCommand{
					Input: agent.RunInput{RunID: startRunID, UserPrompt: "hello", MaxSteps: 3},
				})
			},
			checkAbsent: startRunID,
		},
		{
			name:    "nil_context_dispatch_continue",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(nil, agent.ContinueCommand{RunID: existingRunID, MaxSteps: 3})
			},
		},
		{
			name:    "nil_context_dispatch_cancel",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(nil, agent.CancelCommand{RunID: existingRunID})
			},
		},
		{
			name:    "nil_context_dispatch_steer",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(nil, agent.SteerCommand{RunID: existingRunID, Instruction: "steer"})
			},
		},
		{
			name:    "nil_context_dispatch_follow_up",
			wantErr: agent.ErrContextNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(nil, agent.FollowUpCommand{RunID: existingRunID, UserPrompt: "follow up", MaxSteps: 3})
			},
		},
		{
			name:    "nil_command",
			wantErr: agent.ErrCommandNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), nilCommand)
			},
		},
		{
			name:    "nil_start_command",
			wantErr: agent.ErrCommandNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), nilStart)
			},
		},
		{
			name:    "nil_continue_command",
			wantErr: agent.ErrCommandNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), nilContinue)
			},
		},
		{
			name:    "nil_cancel_command",
			wantErr: agent.ErrCommandNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), nilCancel)
			},
		},
		{
			name:    "nil_steer_command",
			wantErr: agent.ErrCommandNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), nilSteer)
			},
		},
		{
			name:    "nil_follow_up_command",
			wantErr: agent.ErrCommandNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), nilFollowUp)
			},
		},
		{
			name:    "nil_unknown_command",
			wantErr: agent.ErrCommandNil,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), nilUnknown)
			},
		},
		{
			name:    "pointer_start_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), pointerStart)
			},
			checkAbsent: startRunID,
		},
		{
			name:    "pointer_continue_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), pointerContinue)
			},
		},
		{
			name:    "pointer_cancel_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), pointerCancel)
			},
		},
		{
			name:    "pointer_steer_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), pointerSteer)
			},
		},
		{
			name:    "pointer_follow_up_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), pointerFollowUp)
			},
		},
		{
			name:    "pointer_unknown_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), pointerUnknown)
			},
		},
		{
			name:    "unsupported_command",
			wantErr: agent.ErrCommandUnsupported,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), unknownCommand{})
			},
		},
		{
			name:    "invalid_start_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), invalidStartCommand{})
			},
		},
		{
			name:    "invalid_continue_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), invalidContinueCommand{})
			},
		},
		{
			name:    "invalid_cancel_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), invalidCancelCommand{})
			},
		},
		{
			name:    "invalid_steer_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), invalidSteerCommand{})
			},
		},
		{
			name:    "invalid_follow_up_command",
			wantErr: agent.ErrCommandInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), invalidFollowUpCommand{})
			},
		},
		{
			name:    "empty_run_id_continue",
			wantErr: agent.ErrInvalidRunID,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.ContinueCommand{RunID: "", MaxSteps: 3})
			},
		},
		{
			name:    "empty_run_id_cancel",
			wantErr: agent.ErrInvalidRunID,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.CancelCommand{RunID: ""})
			},
		},
		{
			name:    "empty_run_id_steer",
			wantErr: agent.ErrInvalidRunID,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.SteerCommand{RunID: "", Instruction: "steer"})
			},
		},
		{
			name:    "empty_run_id_follow_up",
			wantErr: agent.ErrInvalidRunID,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.FollowUpCommand{RunID: "", UserPrompt: "follow up", MaxSteps: 3})
			},
		},
		{
			name:    "empty_tool_name_start",
			wantErr: agent.ErrToolDefinitionsInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.StartCommand{
					Input: agent.RunInput{
						RunID:      startRunID,
						UserPrompt: "start",
						MaxSteps:   3,
						Tools: []agent.ToolDefinition{
							{Name: ""},
						},
					},
				})
			},
			checkAbsent: startRunID,
		},
		{
			name:    "duplicate_tool_name_start",
			wantErr: agent.ErrToolDefinitionsInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.StartCommand{
					Input: agent.RunInput{
						RunID:      startRunID,
						UserPrompt: "start",
						MaxSteps:   3,
						Tools: []agent.ToolDefinition{
							{Name: "lookup"},
							{Name: "lookup"},
						},
					},
				})
			},
			checkAbsent: startRunID,
		},
		{
			name:    "empty_tool_name_continue",
			wantErr: agent.ErrToolDefinitionsInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.ContinueCommand{
					RunID:    existingRunID,
					MaxSteps: 3,
					Tools: []agent.ToolDefinition{
						{Name: ""},
					},
				})
			},
		},
		{
			name:    "duplicate_tool_name_continue",
			wantErr: agent.ErrToolDefinitionsInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.ContinueCommand{
					RunID:    existingRunID,
					MaxSteps: 3,
					Tools: []agent.ToolDefinition{
						{Name: "lookup"},
						{Name: "lookup"},
					},
				})
			},
		},
		{
			name:    "empty_tool_name_follow_up",
			wantErr: agent.ErrToolDefinitionsInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.FollowUpCommand{
					RunID:      existingRunID,
					UserPrompt: "follow up",
					MaxSteps:   3,
					Tools: []agent.ToolDefinition{
						{Name: ""},
					},
				})
			},
		},
		{
			name:    "duplicate_tool_name_follow_up",
			wantErr: agent.ErrToolDefinitionsInvalid,
			call: func(runner *agent.Runner) (agent.RunResult, error) {
				return runner.Dispatch(context.Background(), agent.FollowUpCommand{
					RunID:      existingRunID,
					UserPrompt: "follow up",
					MaxSteps:   3,
					Tools: []agent.ToolDefinition{
						{Name: "lookup"},
						{Name: "lookup"},
					},
				})
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner, store, events, engine, persistedSeed := newFixture(t)
			result, err := tc.call(runner)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if !reflect.DeepEqual(result, agent.RunResult{}) {
				t.Fatalf("unexpected result: %+v", result)
			}
			if engine.calls != 0 {
				t.Fatalf("engine should not execute for rejected input, calls=%d", engine.calls)
			}
			if gotEvents := events.Events(); len(gotEvents) != 0 {
				t.Fatalf("unexpected events emitted: %d", len(gotEvents))
			}

			loaded, loadErr := store.Load(context.Background(), existingRunID)
			if loadErr != nil {
				t.Fatalf("load seeded state: %v", loadErr)
			}
			if !reflect.DeepEqual(loaded, persistedSeed) {
				t.Fatalf("persisted state mutated: got=%+v want=%+v", loaded, persistedSeed)
			}

			if tc.checkAbsent != "" {
				if _, loadErr := store.Load(context.Background(), tc.checkAbsent); !errors.Is(loadErr, agent.ErrRunNotFound) {
					t.Fatalf("expected ErrRunNotFound for %q, got %v", tc.checkAbsent, loadErr)
				}
			}
		})
	}
}

func TestRunnerSteer_AppendsTranscriptWithoutEngineExecution(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-steer-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   4,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "original"},
		},
	}

	store := runstoreinmem.New()
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load initial: %v", err)
	}

	events := eventinginmem.New()
	engine := &engineSpy{}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	result, err := runner.Steer(context.Background(), runID, "new direction")
	if err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	if engine.calls != 0 {
		t.Fatalf("engine should not execute for steer, calls=%d", engine.calls)
	}
	if result.State.Status != persistedInitial.Status {
		t.Fatalf("unexpected status: got=%s want=%s", result.State.Status, persistedInitial.Status)
	}
	if result.State.Step != persistedInitial.Step {
		t.Fatalf("unexpected step: got=%d want=%d", result.State.Step, persistedInitial.Step)
	}
	if result.State.Version != persistedInitial.Version+1 {
		t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version+1)
	}
	if len(result.State.Messages) != len(persistedInitial.Messages)+1 {
		t.Fatalf("unexpected message count: got=%d want=%d", len(result.State.Messages), len(persistedInitial.Messages)+1)
	}
	appended := result.State.Messages[len(result.State.Messages)-1]
	if appended.Role != agent.RoleUser || appended.Content != "new direction" {
		t.Fatalf("unexpected appended message: %+v", appended)
	}

	gotEvents := events.Events()
	if len(gotEvents) != 2 {
		t.Fatalf("unexpected event count: %d", len(gotEvents))
	}
	if gotEvents[0].Type != agent.EventTypeRunCheckpoint {
		t.Fatalf("unexpected first event type: %s", gotEvents[0].Type)
	}
	if gotEvents[1].Type != agent.EventTypeCommandApplied {
		t.Fatalf("unexpected second event type: %s", gotEvents[1].Type)
	}
	if gotEvents[1].CommandKind != agent.CommandKindSteer {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[1].CommandKind, agent.CommandKindSteer)
	}
}

func TestRunnerFollowUp_AppendsTranscriptAndInvokesEngine(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-follow-up-run")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "original"},
		},
	}

	store := runstoreinmem.New()
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	persistedInitial, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load initial: %v", err)
	}

	events := eventinginmem.New()
	engine := &engineSpy{
		executeFn: func(_ context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
			if len(state.Messages) != len(persistedInitial.Messages)+1 {
				t.Fatalf("unexpected message count received by engine: got=%d want=%d", len(state.Messages), len(persistedInitial.Messages)+1)
			}
			last := state.Messages[len(state.Messages)-1]
			if last.Role != agent.RoleUser || last.Content != "follow up prompt" {
				t.Fatalf("engine received unexpected appended message: %+v", last)
			}
			if input.MaxSteps != 5 {
				t.Fatalf("unexpected max steps: %d", input.MaxSteps)
			}
			if len(input.Tools) != 1 || input.Tools[0].Name != "lookup" {
				t.Fatalf("unexpected tools: %+v", input.Tools)
			}
			next := state
			next.Step++
			next.Status = agent.RunStatusCompleted
			next.Output = "done"
			return next, nil
		},
	}
	runner := newDispatchRunnerWithEngine(t, store, events, engine)

	result, err := runner.FollowUp(context.Background(), runID, "follow up prompt", 5, []agent.ToolDefinition{{Name: "lookup"}})
	if err != nil {
		t.Fatalf("follow up returned error: %v", err)
	}
	if engine.calls != 1 {
		t.Fatalf("engine should execute exactly once for follow up, calls=%d", engine.calls)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.Step != persistedInitial.Step+1 {
		t.Fatalf("unexpected step: got=%d want=%d", result.State.Step, persistedInitial.Step+1)
	}
	if result.State.Version != persistedInitial.Version+1 {
		t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedInitial.Version+1)
	}
	if len(result.State.Messages) != len(persistedInitial.Messages)+1 {
		t.Fatalf("unexpected message count: got=%d want=%d", len(result.State.Messages), len(persistedInitial.Messages)+1)
	}
	appended := result.State.Messages[len(result.State.Messages)-1]
	if appended.Role != agent.RoleUser || appended.Content != "follow up prompt" {
		t.Fatalf("unexpected appended message: %+v", appended)
	}

	gotEvents := events.Events()
	if len(gotEvents) != 2 {
		t.Fatalf("unexpected event count: %d", len(gotEvents))
	}
	if gotEvents[0].Type != agent.EventTypeRunCheckpoint {
		t.Fatalf("unexpected first event type: %s", gotEvents[0].Type)
	}
	if gotEvents[1].Type != agent.EventTypeCommandApplied {
		t.Fatalf("unexpected second event type: %s", gotEvents[1].Type)
	}
	if gotEvents[1].CommandKind != agent.CommandKindFollowUp {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[1].CommandKind, agent.CommandKindFollowUp)
	}
}

func TestRunnerSteerThenFollowUp_TranscriptAppendOnly(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-steer-follow-up-append")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleSystem, Content: "system"},
			{Role: agent.RoleUser, Content: "original"},
		},
	}
	store := runstoreinmem.New()
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	events := eventinginmem.New()
	runner := newDispatchRunner(
		t,
		store,
		events,
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "follow-up done",
			},
		},
	)

	steered, err := runner.Steer(context.Background(), runID, "steer instruction")
	if err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	prefix := agent.CloneMessages(steered.State.Messages)
	followed, err := runner.FollowUp(context.Background(), runID, "follow-up prompt", 3, nil)
	if err != nil {
		t.Fatalf("follow up returned error: %v", err)
	}
	if len(followed.State.Messages) <= len(prefix) {
		t.Fatalf("expected transcript growth after follow up")
	}
	if !reflect.DeepEqual(followed.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("follow up mutated transcript prefix")
	}
	if followed.State.Messages[len(prefix)].Role != agent.RoleUser || followed.State.Messages[len(prefix)].Content != "follow-up prompt" {
		t.Fatalf("unexpected follow-up appended message: %+v", followed.State.Messages[len(prefix)])
	}
}

func TestRunnerDispatch_TerminalStateImmutabilityForMutatingCommands(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-terminal-mutation-matrix")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusCompleted,
		Step:   2,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "frozen"},
		},
	}

	cases := []struct {
		name    string
		wantErr error
		call    func(context.Context, *agent.Runner) (agent.RunResult, error)
	}{
		{
			name:    "continue",
			wantErr: agent.ErrRunNotContinuable,
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.Continue(ctx, runID, 3, nil, nil)
			},
		},
		{
			name:    "cancel",
			wantErr: agent.ErrRunNotCancellable,
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.Cancel(ctx, runID)
			},
		},
		{
			name:    "steer",
			wantErr: agent.ErrRunNotContinuable,
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.Steer(ctx, runID, "instruction")
			},
		},
		{
			name:    "follow_up",
			wantErr: agent.ErrRunNotContinuable,
			call: func(ctx context.Context, runner *agent.Runner) (agent.RunResult, error) {
				return runner.FollowUp(ctx, runID, "prompt", 3, nil)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := runstoreinmem.New()
			if err := store.Save(context.Background(), initial); err != nil {
				t.Fatalf("seed store: %v", err)
			}
			persistedInitial, err := store.Load(context.Background(), runID)
			if err != nil {
				t.Fatalf("load initial: %v", err)
			}

			events := eventinginmem.New()
			engine := &engineSpy{}
			runner := newDispatchRunnerWithEngine(t, store, events, engine)

			result, err := tc.call(context.Background(), runner)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if !reflect.DeepEqual(result.State, persistedInitial) {
				t.Fatalf("result state mutated: got=%+v want=%+v", result.State, persistedInitial)
			}
			if engine.calls != 0 {
				t.Fatalf("engine should not execute on terminal rejection, calls=%d", engine.calls)
			}
			loaded, err := store.Load(context.Background(), runID)
			if err != nil {
				t.Fatalf("load state: %v", err)
			}
			if !reflect.DeepEqual(loaded, persistedInitial) {
				t.Fatalf("persisted state mutated: got=%+v want=%+v", loaded, persistedInitial)
			}
			if gotEvents := events.Events(); len(gotEvents) != 0 {
				t.Fatalf("unexpected events emitted: %d", len(gotEvents))
			}
		})
	}
}

func TestRunnerDispatch_SuspendedResolutionGating(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("dispatch-suspended-resolution-gating")
	seedState := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusSuspended,
		Step:   4,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
		PendingRequirement: &agent.PendingRequirement{
			ID:     "req-approval",
			Kind:   agent.RequirementKindApproval,
			Origin: agent.RequirementOriginModel,
		},
	}

	newFixture := func(t *testing.T) (*agent.Runner, *runstoreinmem.Store, *eventinginmem.Sink, *engineSpy, agent.RunState) {
		t.Helper()

		store := runstoreinmem.New()
		if err := store.Save(context.Background(), seedState); err != nil {
			t.Fatalf("seed store: %v", err)
		}
		persistedSeed, err := store.Load(context.Background(), runID)
		if err != nil {
			t.Fatalf("load seeded state: %v", err)
		}
		events := eventinginmem.New()
		engine := &engineSpy{}
		runner := newDispatchRunnerWithEngine(t, store, events, engine)
		return runner, store, events, engine, persistedSeed
	}

	t.Run("continue_missing_resolution", func(t *testing.T) {
		t.Parallel()

		runner, store, events, engine, persistedSeed := newFixture(t)
		result, err := runner.Dispatch(context.Background(), agent.ContinueCommand{
			RunID:    runID,
			MaxSteps: 3,
		})
		if !errors.Is(err, agent.ErrResolutionRequired) {
			t.Fatalf("expected ErrResolutionRequired, got %v", err)
		}
		if !reflect.DeepEqual(result.State, persistedSeed) {
			t.Fatalf("state mismatch: got=%+v want=%+v", result.State, persistedSeed)
		}
		if engine.calls != 0 {
			t.Fatalf("engine should not execute, calls=%d", engine.calls)
		}
		if gotEvents := events.Events(); len(gotEvents) != 0 {
			t.Fatalf("unexpected events emitted: %d", len(gotEvents))
		}
		loaded, err := store.Load(context.Background(), runID)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		if !reflect.DeepEqual(loaded, persistedSeed) {
			t.Fatalf("persisted state mutated: got=%+v want=%+v", loaded, persistedSeed)
		}
	})

	t.Run("continue_invalid_resolution", func(t *testing.T) {
		t.Parallel()

		runner, store, events, engine, persistedSeed := newFixture(t)
		result, err := runner.Dispatch(context.Background(), agent.ContinueCommand{
			RunID:    runID,
			MaxSteps: 3,
			Resolution: &agent.Resolution{
				RequirementID: "req-approval",
				Kind:          agent.RequirementKindApproval,
				Outcome:       agent.ResolutionOutcomeProvided,
			},
		})
		if !errors.Is(err, agent.ErrResolutionInvalid) {
			t.Fatalf("expected ErrResolutionInvalid, got %v", err)
		}
		if !reflect.DeepEqual(result.State, persistedSeed) {
			t.Fatalf("state mismatch: got=%+v want=%+v", result.State, persistedSeed)
		}
		if engine.calls != 0 {
			t.Fatalf("engine should not execute, calls=%d", engine.calls)
		}
		if gotEvents := events.Events(); len(gotEvents) != 0 {
			t.Fatalf("unexpected events emitted: %d", len(gotEvents))
		}
		loaded, err := store.Load(context.Background(), runID)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		if !reflect.DeepEqual(loaded, persistedSeed) {
			t.Fatalf("persisted state mutated: got=%+v want=%+v", loaded, persistedSeed)
		}
	})

	t.Run("continue_valid_resolution_executes_from_running", func(t *testing.T) {
		t.Parallel()

		runner, store, events, engine, persistedSeed := newFixture(t)
		engine.executeFn = func(_ context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
			if state.Status != agent.RunStatusRunning {
				t.Fatalf("expected running status at engine boundary, got %s", state.Status)
			}
			if state.PendingRequirement != nil {
				t.Fatalf("pending requirement should be cleared before engine execution")
			}
			if input.Resolution == nil {
				t.Fatalf("continue resolution must be forwarded to engine input")
			}
			if input.Resolution.RequirementID != "req-approval" {
				t.Fatalf("unexpected resolution requirement id: %q", input.Resolution.RequirementID)
			}
			if input.Resolution.Kind != agent.RequirementKindApproval {
				t.Fatalf("unexpected resolution kind: %s", input.Resolution.Kind)
			}
			if input.Resolution.Outcome != agent.ResolutionOutcomeApproved {
				t.Fatalf("unexpected resolution outcome: %s", input.Resolution.Outcome)
			}
			next := state
			next.Step++
			next.Status = agent.RunStatusCompleted
			next.Output = "done"
			return next, nil
		}

		result, err := runner.Dispatch(context.Background(), agent.ContinueCommand{
			RunID:    runID,
			MaxSteps: 3,
			Resolution: &agent.Resolution{
				RequirementID: "req-approval",
				Kind:          agent.RequirementKindApproval,
				Outcome:       agent.ResolutionOutcomeApproved,
			},
		})
		if err != nil {
			t.Fatalf("continue returned error: %v", err)
		}
		if engine.calls != 1 {
			t.Fatalf("engine should execute exactly once, calls=%d", engine.calls)
		}
		if result.State.Status != agent.RunStatusCompleted {
			t.Fatalf("unexpected status: %s", result.State.Status)
		}
		if result.State.PendingRequirement != nil {
			t.Fatalf("pending requirement must be cleared after continue")
		}
		if result.State.Version != persistedSeed.Version+1 {
			t.Fatalf("unexpected version: got=%d want=%d", result.State.Version, persistedSeed.Version+1)
		}
		if len(result.State.Messages) != len(persistedSeed.Messages) {
			t.Fatalf("unexpected transcript mutation")
		}
		assertEventTypes(t, events.Events(), []agent.EventType{
			agent.EventTypeRunCheckpoint,
			agent.EventTypeCommandApplied,
		})
		assertCommandKind(t, events.Events(), agent.CommandKindContinue)
		loaded, err := store.Load(context.Background(), runID)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		if !reflect.DeepEqual(loaded, result.State) {
			t.Fatalf("persisted state mismatch: got=%+v want=%+v", loaded, result.State)
		}
	})

	t.Run("steer_and_follow_up_rejected_while_suspended", func(t *testing.T) {
		t.Parallel()

		runner, store, events, engine, persistedSeed := newFixture(t)
		steerResult, steerErr := runner.Dispatch(context.Background(), agent.SteerCommand{
			RunID:       runID,
			Instruction: "new direction",
		})
		if !errors.Is(steerErr, agent.ErrResolutionRequired) {
			t.Fatalf("expected steer ErrResolutionRequired, got %v", steerErr)
		}
		if !reflect.DeepEqual(steerResult.State, persistedSeed) {
			t.Fatalf("steer state mismatch: got=%+v want=%+v", steerResult.State, persistedSeed)
		}
		followUpResult, followUpErr := runner.Dispatch(context.Background(), agent.FollowUpCommand{
			RunID:      runID,
			UserPrompt: "follow up",
			MaxSteps:   3,
		})
		if !errors.Is(followUpErr, agent.ErrResolutionRequired) {
			t.Fatalf("expected follow-up ErrResolutionRequired, got %v", followUpErr)
		}
		if !reflect.DeepEqual(followUpResult.State, persistedSeed) {
			t.Fatalf("follow-up state mismatch: got=%+v want=%+v", followUpResult.State, persistedSeed)
		}
		if engine.calls != 0 {
			t.Fatalf("engine should not execute, calls=%d", engine.calls)
		}
		if gotEvents := events.Events(); len(gotEvents) != 0 {
			t.Fatalf("unexpected events emitted: %d", len(gotEvents))
		}
		loaded, err := store.Load(context.Background(), runID)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		if !reflect.DeepEqual(loaded, persistedSeed) {
			t.Fatalf("persisted state mutated: got=%+v want=%+v", loaded, persistedSeed)
		}
	})
}

type unknownCommand struct{}

func (unknownCommand) Kind() agent.CommandKind {
	return agent.CommandKind("unknown")
}

type invalidStartCommand struct{}

func (invalidStartCommand) Kind() agent.CommandKind {
	return agent.CommandKindStart
}

type invalidContinueCommand struct{}

func (invalidContinueCommand) Kind() agent.CommandKind {
	return agent.CommandKindContinue
}

type invalidCancelCommand struct{}

func (invalidCancelCommand) Kind() agent.CommandKind {
	return agent.CommandKindCancel
}

type invalidSteerCommand struct{}

func (invalidSteerCommand) Kind() agent.CommandKind {
	return agent.CommandKindSteer
}

type invalidFollowUpCommand struct{}

func (invalidFollowUpCommand) Kind() agent.CommandKind {
	return agent.CommandKindFollowUp
}

func newDispatchRunner(
	t *testing.T,
	store *runstoreinmem.Store,
	events *eventinginmem.Sink,
	responses ...response,
) *agent.Runner {
	t.Helper()

	engine := &scriptedEngine{
		events:    events,
		responses: append([]response(nil), responses...),
	}
	return newDispatchRunnerWithEngine(t, store, events, engine)
}

type engineSpy struct {
	calls     int
	executeFn func(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error)
}

func (s *engineSpy) Execute(ctx context.Context, state agent.RunState, input agent.EngineInput) (agent.RunState, error) {
	s.calls++
	if s.executeFn == nil {
		return state, nil
	}
	return s.executeFn(ctx, state, input)
}

func newDispatchRunnerWithEngine(
	t *testing.T,
	store *runstoreinmem.Store,
	events *eventinginmem.Sink,
	engine agent.Engine,
) *agent.Runner {
	t.Helper()

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("dispatch"),
		RunStore:    store,
		Engine:      engine,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner
}

func assertEventTypes(t *testing.T, events []agent.Event, want []agent.EventType) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(events), len(want))
	}
	for i := range want {
		if events[i].Type != want[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, events[i].Type, want[i])
		}
	}
}

func assertCommandKind(t *testing.T, events []agent.Event, want agent.CommandKind) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("no events emitted")
	}
	last := events[len(events)-1]
	if last.Type != agent.EventTypeCommandApplied {
		t.Fatalf("last event must be command_applied, got=%s", last.Type)
	}
	if last.CommandKind != want {
		t.Fatalf("unexpected command kind: got=%s want=%s", last.CommandKind, want)
	}
}

type scriptedEngine struct {
	index     int
	events    agent.EventSink
	responses []response
}

func (s *scriptedEngine) Execute(ctx context.Context, state agent.RunState, _ agent.EngineInput) (agent.RunState, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		next := state
		if transitionErr := agent.TransitionRunStatus(&next, agent.RunStatusCancelled); transitionErr != nil {
			return state, errors.Join(ctxErr, transitionErr)
		}
		next.Error = ctxErr.Error()
		_ = s.events.Publish(ctx, agent.Event{
			RunID:       next.ID,
			Step:        next.Step,
			Type:        agent.EventTypeRunCancelled,
			Description: ctxErr.Error(),
		})
		return next, ctxErr
	}

	if s.index >= len(s.responses) {
		return state, fmt.Errorf("script exhausted at step %d", s.index+1)
	}
	current := s.responses[s.index]
	s.index++
	if current.Err != nil {
		return state, current.Err
	}

	next := state
	if err := agent.TransitionRunStatus(&next, agent.RunStatusRunning); err != nil {
		return next, err
	}
	next.Step++

	message := agent.CloneMessage(current.Message)
	if message.Role == "" {
		message.Role = agent.RoleAssistant
	}
	next.Messages = append(next.Messages, message)
	_ = s.events.Publish(ctx, agent.Event{
		RunID:   next.ID,
		Step:    next.Step,
		Type:    agent.EventTypeAssistantMessage,
		Message: &message,
	})

	if len(message.ToolCalls) == 0 {
		if err := agent.TransitionRunStatus(&next, agent.RunStatusCompleted); err != nil {
			return next, err
		}
		next.Output = message.Content
		_ = s.events.Publish(ctx, agent.Event{
			RunID:       next.ID,
			Step:        next.Step,
			Type:        agent.EventTypeRunCompleted,
			Description: "assistant returned a final answer",
		})
	}

	return next, nil
}
