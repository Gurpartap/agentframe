package agentreact_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

func TestConformance_EventOrdering(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "I need to use a tool.",
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
				Content: "Final answer.",
			},
		},
	)
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, args map[string]any) (string, error) {
			return "value_for=" + args["q"].(string), nil
		},
	})
	events := newEventSink()
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("evt"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Find info about Go.",
		MaxSteps:   4,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	got := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeAssistantMessage,
		agent.EventTypeToolResult,
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{0, 1, 1, 2, 2, 2, 2}
	if len(got) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(got), len(wantTypes))
	}
	for i := range wantTypes {
		if got[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, got[i].Type, wantTypes[i])
		}
		if got[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, got[i].Step, wantSteps[i])
		}
		if got[i].RunID != result.State.ID {
			t.Fatalf("event[%d] run_id mismatch: got=%s want=%s", i, got[i].RunID, result.State.ID)
		}
	}
	if got[1].Message == nil || len(got[1].Message.ToolCalls) != 1 || got[1].Message.ToolCalls[0].ID != "call-1" {
		t.Fatalf("assistant event does not contain expected tool call")
	}
	if got[2].ToolResult == nil || got[2].ToolResult.CallID != "call-1" || got[2].ToolResult.Name != "lookup" {
		t.Fatalf("tool result event does not link to expected tool call")
	}
	if got[6].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected command kind: got=%s want=%s", got[6].CommandKind, agent.CommandKindStart)
	}
}

func TestConformance_TranscriptToolCallResultLinkage(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "I need two tools.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
						Arguments: map[string]any{
							"q": "Go",
						},
					},
					{
						ID:   "call-2",
						Name: "summarize",
						Arguments: map[string]any{
							"text": "Go is a programming language",
						},
					},
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Final answer after both tools.",
			},
		},
	)
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, args map[string]any) (string, error) {
			return "lookup=" + args["q"].(string), nil
		},
		"summarize": func(_ context.Context, args map[string]any) (string, error) {
			return "summary=" + args["text"].(string), nil
		},
	})
	loop, err := agentreact.New(model, registry, newEventSink())
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("link"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   newEventSink(),
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Use tools.",
		MaxSteps:   4,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
			{Name: "summarize"},
		},
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	callByID := map[string]agent.ToolCall{}
	callIndex := map[string]int{}
	resultCount := map[string]int{}
	for i, msg := range result.State.Messages {
		if msg.Role == agent.RoleAssistant {
			for _, call := range msg.ToolCalls {
				callByID[call.ID] = call
				callIndex[call.ID] = i
			}
			continue
		}
		if msg.Role != agent.RoleTool {
			continue
		}
		if msg.ToolCallID == "" {
			t.Fatalf("tool result at index %d has empty tool_call_id", i)
		}
		call, ok := callByID[msg.ToolCallID]
		if !ok {
			t.Fatalf("tool result at index %d references unknown tool_call_id %q", i, msg.ToolCallID)
		}
		if i <= callIndex[msg.ToolCallID] {
			t.Fatalf("tool result at index %d appears before its tool call", i)
		}
		if msg.Name != call.Name {
			t.Fatalf("tool result at index %d has name %q, want %q", i, msg.Name, call.Name)
		}
		resultCount[msg.ToolCallID]++
	}

	for id := range callByID {
		if resultCount[id] != 1 {
			t.Fatalf("tool call %q has result count %d, want 1", id, resultCount[id])
		}
	}
}

func TestConformance_ContinueDeterministicProgression(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Need tool first.",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
					},
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "Final answer after continue.",
			},
		},
	)
	store := newRunStore()
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-value", nil
		},
	})
	loop, err := agentreact.New(model, registry, newEventSink())
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("continue"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   newEventSink(),
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	initialResult, initialErr := runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "Do the thing.",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(initialErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", initialErr)
	}
	if initialResult.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected initial status: %s", initialResult.State.Status)
	}
	if initialResult.State.Step != 1 {
		t.Fatalf("unexpected initial step: %d", initialResult.State.Step)
	}
	if initialResult.State.Version != 2 {
		t.Fatalf("unexpected initial version: %d", initialResult.State.Version)
	}

	loadedBeforeContinue, err := store.Load(context.Background(), initialResult.State.ID)
	if err != nil {
		t.Fatalf("load before continue: %v", err)
	}
	if !reflect.DeepEqual(loadedBeforeContinue, initialResult.State) {
		t.Fatalf("saved state mismatch before continue")
	}

	beforeMessages := agent.CloneMessages(initialResult.State.Messages)
	continuedResult, continueErr := runner.Continue(context.Background(), initialResult.State.ID, 3, []agent.ToolDefinition{
		{Name: "lookup"},
	}, nil)
	if continueErr != nil {
		t.Fatalf("continue returned error: %v", continueErr)
	}
	if continuedResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected continued status: %s", continuedResult.State.Status)
	}
	if continuedResult.State.Step != 2 {
		t.Fatalf("unexpected continued step: %d", continuedResult.State.Step)
	}
	if continuedResult.State.Version != initialResult.State.Version+1 {
		t.Fatalf("unexpected continued version: got=%d want=%d", continuedResult.State.Version, initialResult.State.Version+1)
	}
	if continuedResult.State.Output != "Final answer after continue." {
		t.Fatalf("unexpected continued output: %q", continuedResult.State.Output)
	}
	if len(continuedResult.State.Messages) <= len(beforeMessages) {
		t.Fatalf("expected transcript growth after continue")
	}
	if !reflect.DeepEqual(continuedResult.State.Messages[:len(beforeMessages)], beforeMessages) {
		t.Fatalf("continue mutated existing transcript prefix")
	}

	loadedAfterContinue, err := store.Load(context.Background(), initialResult.State.ID)
	if err != nil {
		t.Fatalf("load after continue: %v", err)
	}
	if !reflect.DeepEqual(loadedAfterContinue, continuedResult.State) {
		t.Fatalf("saved state mismatch after continue")
	}
}

func TestConformance_CommandAppliedContinueOrdering(t *testing.T) {
	t.Parallel()

	runID := agent.RunID("conformance-continue-command")
	store := newRunStore()
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "continue me"},
		},
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "continued",
		},
	})
	registry := newRegistry(map[string]handler{})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("continue-order"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Continue(context.Background(), runID, 2, nil, nil)
	if err != nil {
		t.Fatalf("continue returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	got := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{1, 1, 1, 1}
	if len(got) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(got), len(wantTypes))
	}
	for i := range wantTypes {
		if got[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, got[i].Type, wantTypes[i])
		}
		if got[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, got[i].Step, wantSteps[i])
		}
	}
	if got[3].CommandKind != agent.CommandKindContinue {
		t.Fatalf("unexpected command kind: got=%s want=%s", got[3].CommandKind, agent.CommandKindContinue)
	}
}

func TestConformance_CommandAppliedCancelOrdering(t *testing.T) {
	t.Parallel()

	runID := agent.RunID("conformance-cancel-command")
	store := newRunStore()
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusRunning,
		Step:   3,
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "unused",
		},
	})
	registry := newRegistry(map[string]handler{})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("cancel-order"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, err := runner.Cancel(context.Background(), runID)
	if err != nil {
		t.Fatalf("cancel returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCancelled {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	got := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunCancelled,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{3, 3}
	if len(got) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(got), len(wantTypes))
	}
	for i := range wantTypes {
		if got[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, got[i].Type, wantTypes[i])
		}
		if got[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, got[i].Step, wantSteps[i])
		}
	}
	if got[1].CommandKind != agent.CommandKindCancel {
		t.Fatalf("unexpected command kind: got=%s want=%s", got[1].CommandKind, agent.CommandKindCancel)
	}
}

func TestConformance_SteerThenFollowUpEventOrdering(t *testing.T) {
	t.Parallel()

	runID := agent.RunID("conformance-steer-followup-order")
	store := newRunStore()
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "follow-up answer",
		},
	})
	registry := newRegistry(map[string]handler{})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("steer-followup-order"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	if _, err := runner.Steer(context.Background(), runID, "steer now"); err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	if _, err := runner.FollowUp(context.Background(), runID, "follow up", 2, nil); err != nil {
		t.Fatalf("follow up returned error: %v", err)
	}

	got := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{0, 0, 1, 1, 1, 1}
	if len(got) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(got), len(wantTypes))
	}
	for i := range wantTypes {
		if got[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, got[i].Type, wantTypes[i])
		}
		if got[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, got[i].Step, wantSteps[i])
		}
		if got[i].RunID != runID {
			t.Fatalf("event[%d] run_id mismatch: got=%s want=%s", i, got[i].RunID, runID)
		}
	}
	if got[1].CommandKind != agent.CommandKindSteer {
		t.Fatalf("unexpected steer command kind: got=%s want=%s", got[1].CommandKind, agent.CommandKindSteer)
	}
	if got[5].CommandKind != agent.CommandKindFollowUp {
		t.Fatalf("unexpected follow-up command kind: got=%s want=%s", got[5].CommandKind, agent.CommandKindFollowUp)
	}
}

func TestConformance_TranscriptAppendOnlyAcrossRunSteerFollowUp(t *testing.T) {
	t.Parallel()

	runID := agent.RunID("conformance-append-run-steer-followup")
	store := newRunStore()
	events := newEventSink()
	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
					},
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "final after follow up",
			},
		},
	)
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-value", nil
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("append-run-steer-followup"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	initialResult, initialErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      runID,
		UserPrompt: "start",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(initialErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", initialErr)
	}
	if initialResult.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected initial status: %s", initialResult.State.Status)
	}

	runPrefix := agent.CloneMessages(initialResult.State.Messages)
	steeredResult, err := runner.Steer(context.Background(), runID, "steer instruction")
	if err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	if len(steeredResult.State.Messages) != len(runPrefix)+1 {
		t.Fatalf("unexpected steered message count: got=%d want=%d", len(steeredResult.State.Messages), len(runPrefix)+1)
	}
	if !reflect.DeepEqual(steeredResult.State.Messages[:len(runPrefix)], runPrefix) {
		t.Fatalf("steer mutated transcript prefix")
	}
	if steeredResult.State.Messages[len(runPrefix)].Role != agent.RoleUser || steeredResult.State.Messages[len(runPrefix)].Content != "steer instruction" {
		t.Fatalf("unexpected steer-appended message: %+v", steeredResult.State.Messages[len(runPrefix)])
	}

	steerPrefix := agent.CloneMessages(steeredResult.State.Messages)
	followUpResult, err := runner.FollowUp(context.Background(), runID, "follow up prompt", 3, []agent.ToolDefinition{
		{Name: "lookup"},
	})
	if err != nil {
		t.Fatalf("follow up returned error: %v", err)
	}
	if followUpResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected follow-up status: %s", followUpResult.State.Status)
	}
	if len(followUpResult.State.Messages) <= len(steerPrefix) {
		t.Fatalf("expected transcript growth after follow up")
	}
	if !reflect.DeepEqual(followUpResult.State.Messages[:len(steerPrefix)], steerPrefix) {
		t.Fatalf("follow up mutated transcript prefix")
	}
	if followUpResult.State.Messages[len(steerPrefix)].Role != agent.RoleUser || followUpResult.State.Messages[len(steerPrefix)].Content != "follow up prompt" {
		t.Fatalf("unexpected follow-up appended message: %+v", followUpResult.State.Messages[len(steerPrefix)])
	}
}

func TestConformance_DeterministicUnderIdenticalScriptedInputs(t *testing.T) {
	t.Parallel()

	runOnce := func(t *testing.T) (agent.RunResult, agent.RunResult, agent.RunResult, []agent.Event) {
		t.Helper()

		runID := agent.RunID("conformance-deterministic-run")
		store := newRunStore()
		events := newEventSink()
		model := newScriptedModel(
			response{
				Message: agent.Message{
					Role:    agent.RoleAssistant,
					Content: "need tool",
					ToolCalls: []agent.ToolCall{
						{
							ID:   "call-1",
							Name: "lookup",
						},
					},
				},
			},
			response{
				Message: agent.Message{
					Role:    agent.RoleAssistant,
					Content: "final deterministic answer",
				},
			},
		)
		registry := newRegistry(map[string]handler{
			"lookup": func(_ context.Context, _ map[string]any) (string, error) {
				return "tool-value", nil
			},
		})
		loop, err := agentreact.New(model, registry, events)
		if err != nil {
			t.Fatalf("new loop: %v", err)
		}
		runner, err := agent.NewRunner(agent.Dependencies{
			IDGenerator: newCounterIDGenerator("deterministic"),
			RunStore:    store,
			Engine:      loop,
			EventSink:   events,
		})
		if err != nil {
			t.Fatalf("new runner: %v", err)
		}

		runResult, runErr := runner.Run(context.Background(), agent.RunInput{
			RunID:      runID,
			UserPrompt: "start",
			MaxSteps:   1,
			Tools: []agent.ToolDefinition{
				{Name: "lookup"},
			},
		})
		if !errors.Is(runErr, agent.ErrMaxStepsExceeded) {
			t.Fatalf("expected ErrMaxStepsExceeded, got %v", runErr)
		}

		steerResult, err := runner.Steer(context.Background(), runID, "steer deterministic")
		if err != nil {
			t.Fatalf("steer returned error: %v", err)
		}

		followUpResult, err := runner.FollowUp(context.Background(), runID, "follow up deterministic", 3, []agent.ToolDefinition{
			{Name: "lookup"},
		})
		if err != nil {
			t.Fatalf("follow up returned error: %v", err)
		}

		return runResult, steerResult, followUpResult, events.Events()
	}

	firstRun, firstSteer, firstFollowUp, firstEvents := runOnce(t)
	secondRun, secondSteer, secondFollowUp, secondEvents := runOnce(t)

	if !reflect.DeepEqual(secondRun, firstRun) {
		t.Fatalf("run result mismatch under identical scripted inputs")
	}
	if !reflect.DeepEqual(secondSteer, firstSteer) {
		t.Fatalf("steer result mismatch under identical scripted inputs")
	}
	if !reflect.DeepEqual(secondFollowUp, firstFollowUp) {
		t.Fatalf("follow-up result mismatch under identical scripted inputs")
	}
	if !reflect.DeepEqual(secondEvents, firstEvents) {
		t.Fatalf("event stream mismatch under identical scripted inputs")
	}
}

func TestConformance_ReactLoopRejectsSuspendedInputWithoutResolution(t *testing.T) {
	t.Parallel()

	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "final",
		},
	})
	loop, err := agentreact.New(model, newRegistry(map[string]handler{}), newEventSink())
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}

	initial := agent.RunState{
		ID:     "react-loop-suspended-direct-execute",
		Status: agent.RunStatusSuspended,
		PendingRequirement: &agent.PendingRequirement{
			ID:     "req-1",
			Kind:   agent.RequirementKindApproval,
			Origin: agent.RequirementOriginModel,
		},
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}
	next, execErr := loop.Execute(context.Background(), initial, agent.EngineInput{MaxSteps: 2})
	if !errors.Is(execErr, agent.ErrResolutionRequired) {
		t.Fatalf("expected ErrResolutionRequired, got %v", execErr)
	}
	if !reflect.DeepEqual(next, initial) {
		t.Fatalf("state changed on suspended direct execute rejection")
	}
}

func TestConformance_ContinueFromSuspendedWithResolutionUsingReactLoop(t *testing.T) {
	t.Parallel()

	runID := agent.RunID("conformance-continue-suspended-resolution")
	store := newRunStore()
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusSuspended,
		Step:   2,
		PendingRequirement: &agent.PendingRequirement{
			ID:     "req-user-input",
			Kind:   agent.RequirementKindUserInput,
			Origin: agent.RequirementOriginModel,
		},
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "seed"},
		},
	}
	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "final after resolution",
		},
	})
	registry := newRegistry(map[string]handler{})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("continue-suspended"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	loadedBefore, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load before continue: %v", err)
	}
	prefix := agent.CloneMessages(loadedBefore.Messages)

	result, err := runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    runID,
		MaxSteps: 5,
		Resolution: &agent.Resolution{
			RequirementID: "req-user-input",
			Kind:          agent.RequirementKindUserInput,
			Outcome:       agent.ResolutionOutcomeProvided,
			Value:         "provided input",
		},
	})
	if err != nil {
		t.Fatalf("continue returned error: %v", err)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared after continue")
	}
	if len(result.State.Messages) <= len(prefix) {
		t.Fatalf("expected transcript growth after continue")
	}
	if !reflect.DeepEqual(result.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("continue mutated transcript prefix")
	}

	loadedAfter, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load after continue: %v", err)
	}
	if !reflect.DeepEqual(loadedAfter, result.State) {
		t.Fatalf("saved state mismatch after continue")
	}

	gotEvents := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	if len(gotEvents) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(gotEvents), len(wantTypes))
	}
	for i := range wantTypes {
		if gotEvents[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, gotEvents[i].Type, wantTypes[i])
		}
	}
	if gotEvents[len(gotEvents)-1].CommandKind != agent.CommandKindContinue {
		t.Fatalf("unexpected command kind: got=%s want=%s", gotEvents[len(gotEvents)-1].CommandKind, agent.CommandKindContinue)
	}
}

func TestConformance_RunSuspendsWhenModelEmitsRequirementWithOrigin(t *testing.T) {
	t.Parallel()

	runID := agent.RunID("conformance-run-suspends-on-requirement")
	events := newEventSink()
	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need approval before tool execution",
				Requirement: &agent.PendingRequirement{
					ID:     "req-approval",
					Kind:   agent.RequirementKindApproval,
					Origin: agent.RequirementOriginModel,
					Prompt: "approve execution",
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "approved and completed",
			},
		},
	)
	loop, err := agentreact.New(model, newRegistry(map[string]handler{}), events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("suspend-flow"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	runResult, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      runID,
		UserPrompt: "start",
		MaxSteps:   4,
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if runResult.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected run status: %s", runResult.State.Status)
	}
	if runResult.State.PendingRequirement == nil {
		t.Fatalf("expected pending requirement on suspended run")
	}
	if runResult.State.PendingRequirement.ID != "req-approval" {
		t.Fatalf("unexpected requirement id: %q", runResult.State.PendingRequirement.ID)
	}
	prefix := agent.CloneMessages(runResult.State.Messages)

	continued, continueErr := runner.Continue(
		context.Background(),
		runID,
		4,
		nil,
		&agent.Resolution{
			RequirementID: "req-approval",
			Kind:          agent.RequirementKindApproval,
			Outcome:       agent.ResolutionOutcomeApproved,
		},
	)
	if continueErr != nil {
		t.Fatalf("continue returned error: %v", continueErr)
	}
	if continued.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected continued status: %s", continued.State.Status)
	}
	if continued.State.PendingRequirement != nil {
		t.Fatalf("pending requirement should be cleared after continue")
	}
	if len(continued.State.Messages) <= len(prefix) {
		t.Fatalf("expected transcript growth after continue")
	}
	if !reflect.DeepEqual(continued.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("continue mutated transcript prefix")
	}

	gotEvents := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunSuspended,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{0, 1, 1, 1, 1, 2, 2, 2, 2}
	if len(gotEvents) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(gotEvents), len(wantTypes))
	}
	for i := range wantTypes {
		if gotEvents[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, gotEvents[i].Type, wantTypes[i])
		}
		if gotEvents[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, gotEvents[i].Step, wantSteps[i])
		}
	}
	if gotEvents[4].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected start command kind: got=%s want=%s", gotEvents[4].CommandKind, agent.CommandKindStart)
	}
	if gotEvents[8].CommandKind != agent.CommandKindContinue {
		t.Fatalf("unexpected continue command kind: got=%s want=%s", gotEvents[8].CommandKind, agent.CommandKindContinue)
	}
}

func TestConformance_ModelRequirementMissingOriginFailsRun(t *testing.T) {
	t.Parallel()

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "approval required",
			Requirement: &agent.PendingRequirement{
				ID:   "req-missing-origin",
				Kind: agent.RequirementKindApproval,
			},
		},
	})
	loop, err := agentreact.New(model, newRegistry(map[string]handler{}), events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("missing-origin"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "conformance-model-missing-origin",
		UserPrompt: "start",
		MaxSteps:   3,
	})
	if !errors.Is(runErr, agent.ErrRunStateInvalid) {
		t.Fatalf("expected ErrRunStateInvalid, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared after invalid requirement failure")
	}
	if !strings.Contains(result.State.Error, "pending_requirement.origin") {
		t.Fatalf("unexpected state error: %q", result.State.Error)
	}
	if countEventType(events.Events(), agent.EventTypeRunSuspended) != 0 {
		t.Fatalf("unexpected run_suspended event for invalid model requirement")
	}
	if countEventType(events.Events(), agent.EventTypeRunFailed) != 1 {
		t.Fatalf("expected single run_failed event")
	}
}

func TestConformance_ModelRequirementWithToolOriginFailsDeterministically(t *testing.T) {
	t.Parallel()

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "approval required",
			Requirement: &agent.PendingRequirement{
				ID:     "req-model-wrong-origin",
				Kind:   agent.RequirementKindApproval,
				Origin: agent.RequirementOriginTool,
			},
		},
	})
	loop, err := agentreact.New(model, newRegistry(map[string]handler{}), events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("model-wrong-origin"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "conformance-model-wrong-origin",
		UserPrompt: "start",
		MaxSteps:   3,
	})
	if !errors.Is(runErr, agent.ErrRunStateInvalid) {
		t.Fatalf("expected ErrRunStateInvalid, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared after invalid model requirement")
	}
	if !strings.Contains(result.State.Error, "source=model") {
		t.Fatalf("unexpected state error: %q", result.State.Error)
	}
	if countEventType(events.Events(), agent.EventTypeRunSuspended) != 0 {
		t.Fatalf("unexpected run_suspended event for invalid model requirement origin")
	}
	if countEventType(events.Events(), agent.EventTypeRunFailed) != 1 {
		t.Fatalf("expected single run_failed event")
	}
}

func TestConformance_RunSuspendsWhenToolExecutorRequestsSuspension(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("conformance-tool-suspends-run")
	events := newEventSink()
	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool input",
				ToolCalls: []agent.ToolCall{
					{
						ID:   "call-1",
						Name: "lookup",
					},
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "completed after resolution",
			},
		},
	)
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:          "req-tool-input",
					Kind:        agent.RequirementKindUserInput,
					Origin:      agent.RequirementOriginTool,
					Fingerprint: "fp-call-1",
					Prompt:      "provide tool input",
				},
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-suspend"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      runID,
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.PendingRequirement == nil {
		t.Fatalf("expected pending requirement")
	}
	if result.State.PendingRequirement.ID != "req-tool-input" {
		t.Fatalf("unexpected requirement id: %q", result.State.PendingRequirement.ID)
	}
	if result.State.PendingRequirement.Kind != agent.RequirementKindUserInput {
		t.Fatalf("unexpected requirement kind: %s", result.State.PendingRequirement.Kind)
	}
	if result.State.PendingRequirement.Origin != agent.RequirementOriginTool {
		t.Fatalf("unexpected requirement origin: %s", result.State.PendingRequirement.Origin)
	}
	if result.State.PendingRequirement.ToolCallID != "call-1" {
		t.Fatalf("unexpected requirement tool call id: %q", result.State.PendingRequirement.ToolCallID)
	}
	if result.State.PendingRequirement.Fingerprint != "fp-call-1" {
		t.Fatalf("unexpected requirement fingerprint: %q", result.State.PendingRequirement.Fingerprint)
	}
	if countEventType(events.Events(), agent.EventTypeRunSuspended) != 1 {
		t.Fatalf("expected single run_suspended event")
	}
}

func TestConformance_ToolSuspensionEmitsSuspendedToolResult(t *testing.T) {
	t.Parallel()

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "need tool input",
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-1",
					Name: "lookup",
				},
			},
		},
	})
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:          "req-tool-input",
					Kind:        agent.RequirementKindUserInput,
					Origin:      agent.RequirementOriginTool,
					Fingerprint: "fp-call-1",
				},
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-result-suspended"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "conformance-suspended-tool-result",
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	var toolResultEvent *agent.Event
	for i := range events.Events() {
		current := events.Events()[i]
		if current.Type == agent.EventTypeToolResult {
			toolResultEvent = &current
			break
		}
	}
	if toolResultEvent == nil || toolResultEvent.ToolResult == nil {
		t.Fatalf("expected tool_result event")
	}
	if !toolResultEvent.ToolResult.IsError {
		t.Fatalf("tool result must be marked error")
	}
	if toolResultEvent.ToolResult.FailureReason != agent.ToolFailureReasonSuspended {
		t.Fatalf("unexpected failure reason: %s", toolResultEvent.ToolResult.FailureReason)
	}
	if !strings.Contains(toolResultEvent.ToolResult.Content, string(agent.ToolFailureReasonSuspended)) {
		t.Fatalf("unexpected tool result content: %q", toolResultEvent.ToolResult.Content)
	}

	if len(result.State.Messages) < 3 {
		t.Fatalf("expected tool result message in transcript")
	}
	last := result.State.Messages[len(result.State.Messages)-1]
	if last.Role != agent.RoleTool {
		t.Fatalf("unexpected last transcript role: %s", last.Role)
	}
	if !strings.Contains(last.Content, string(agent.ToolFailureReasonSuspended)) {
		t.Fatalf("unexpected transcript tool content: %q", last.Content)
	}
}

func TestConformance_ToolSuspensionStopsRemainingToolCalls(t *testing.T) {
	t.Parallel()

	var firstCalls atomic.Int32
	var secondCalls atomic.Int32

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "issue two tool calls",
			ToolCalls: []agent.ToolCall{
				{ID: "call-suspend", Name: "suspender"},
				{ID: "call-second", Name: "secondary"},
			},
		},
	})
	registry := newRegistry(map[string]handler{
		"suspender": func(_ context.Context, _ map[string]any) (string, error) {
			firstCalls.Add(1)
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:          "req-stop",
					Kind:        agent.RequirementKindApproval,
					Origin:      agent.RequirementOriginTool,
					Fingerprint: "fp-call-suspend",
				},
			}
		},
		"secondary": func(_ context.Context, _ map[string]any) (string, error) {
			secondCalls.Add(1)
			return "unexpected", nil
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-stop"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "conformance-tool-stop-remaining",
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "suspender"},
			{Name: "secondary"},
		},
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if firstCalls.Load() != 1 {
		t.Fatalf("unexpected suspender call count: %d", firstCalls.Load())
	}
	if secondCalls.Load() != 0 {
		t.Fatalf("unexpected secondary call count: %d", secondCalls.Load())
	}
	if countEventType(events.Events(), agent.EventTypeToolResult) != 1 {
		t.Fatalf("expected one tool_result event")
	}
}

func TestConformance_ToolSuspensionEventOrdering(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("conformance-tool-suspension-ordering")
	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "needs tool input",
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "lookup"},
			},
		},
	})
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:          "req-tool-order",
					Kind:        agent.RequirementKindUserInput,
					Origin:      agent.RequirementOriginTool,
					Fingerprint: "fp-call-1",
				},
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-order"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      runID,
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}

	got := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeAssistantMessage,
		agent.EventTypeToolResult,
		agent.EventTypeRunSuspended,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{0, 1, 1, 1, 1, 1}
	if len(got) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(got), len(wantTypes))
	}
	for i := range wantTypes {
		if got[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, got[i].Type, wantTypes[i])
		}
		if got[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, got[i].Step, wantSteps[i])
		}
		if got[i].RunID != runID {
			t.Fatalf("event[%d] run id mismatch: got=%s want=%s", i, got[i].RunID, runID)
		}
	}
	if got[5].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected command kind: got=%s want=%s", got[5].CommandKind, agent.CommandKindStart)
	}
	if got[2].ToolResult == nil || got[2].ToolResult.FailureReason != agent.ToolFailureReasonSuspended {
		t.Fatalf("unexpected tool result payload in ordering test")
	}
}

func TestConformance_ContinueFromToolSuspensionRequiresResolution(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("conformance-tool-suspension-continue-requires-resolution")
	store := newRunStore()
	events := newEventSink()
	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool input",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "lookup"},
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "should not be reached",
			},
		},
	)
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:          "req-tool-continue",
					Kind:        agent.RequirementKindUserInput,
					Origin:      agent.RequirementOriginTool,
					Fingerprint: "fp-call-1",
				},
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-continue-required"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	runResult, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      runID,
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if runResult.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected status: %s", runResult.State.Status)
	}
	if runResult.State.PendingRequirement == nil {
		t.Fatalf("expected pending requirement")
	}
	if runResult.State.PendingRequirement.ToolCallID != "call-1" {
		t.Fatalf("unexpected requirement tool call id: %q", runResult.State.PendingRequirement.ToolCallID)
	}
	if runResult.State.PendingRequirement.Fingerprint != "fp-call-1" {
		t.Fatalf("unexpected requirement fingerprint: %q", runResult.State.PendingRequirement.Fingerprint)
	}
	persistedBeforeContinue, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load before continue: %v", err)
	}
	eventCountBeforeContinue := len(events.Events())

	continueResult, continueErr := runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    runID,
		MaxSteps: 3,
	})
	if !errors.Is(continueErr, agent.ErrResolutionRequired) {
		t.Fatalf("expected ErrResolutionRequired, got %v", continueErr)
	}
	if !reflect.DeepEqual(continueResult.State, persistedBeforeContinue) {
		t.Fatalf("continue mutated state on resolution-required rejection")
	}
	if len(events.Events()) != eventCountBeforeContinue {
		t.Fatalf("unexpected events emitted on resolution-required rejection")
	}
}

func TestConformance_ContinueFromToolSuspensionWithMatchingResolution(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("conformance-tool-suspension-continue-success")
	store := newRunStore()
	events := newEventSink()
	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool input",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "lookup"},
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "completed after tool resolution",
			},
		},
	)
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:          "req-tool-continue",
					Kind:        agent.RequirementKindUserInput,
					Origin:      agent.RequirementOriginTool,
					Fingerprint: "fp-call-1",
				},
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-continue-success"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	runResult, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      runID,
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if runResult.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected status: %s", runResult.State.Status)
	}
	if runResult.State.PendingRequirement == nil {
		t.Fatalf("expected pending requirement")
	}
	if runResult.State.PendingRequirement.ToolCallID != "call-1" {
		t.Fatalf("unexpected requirement tool call id: %q", runResult.State.PendingRequirement.ToolCallID)
	}
	if runResult.State.PendingRequirement.Fingerprint != "fp-call-1" {
		t.Fatalf("unexpected requirement fingerprint: %q", runResult.State.PendingRequirement.Fingerprint)
	}
	prefix := agent.CloneMessages(runResult.State.Messages)

	continueResult, continueErr := runner.Dispatch(context.Background(), agent.ContinueCommand{
		RunID:    runID,
		MaxSteps: 3,
		Resolution: &agent.Resolution{
			RequirementID: "req-tool-continue",
			Kind:          agent.RequirementKindUserInput,
			Outcome:       agent.ResolutionOutcomeProvided,
			Value:         "input provided",
		},
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if continueErr != nil {
		t.Fatalf("continue returned error: %v", continueErr)
	}
	if continueResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected continue status: %s", continueResult.State.Status)
	}
	if continueResult.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared after continue")
	}
	if len(continueResult.State.Messages) <= len(prefix) {
		t.Fatalf("expected transcript growth after continue")
	}
	if !reflect.DeepEqual(continueResult.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("continue mutated transcript prefix")
	}

	gotEvents := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeAssistantMessage,
		agent.EventTypeToolResult,
		agent.EventTypeRunSuspended,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{0, 1, 1, 1, 1, 1, 2, 2, 2, 2}
	if len(gotEvents) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(gotEvents), len(wantTypes))
	}
	for i := range wantTypes {
		if gotEvents[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, gotEvents[i].Type, wantTypes[i])
		}
		if gotEvents[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, gotEvents[i].Step, wantSteps[i])
		}
	}
	if gotEvents[5].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected start command kind: got=%s want=%s", gotEvents[5].CommandKind, agent.CommandKindStart)
	}
	if gotEvents[9].CommandKind != agent.CommandKindContinue {
		t.Fatalf("unexpected continue command kind: got=%s want=%s", gotEvents[9].CommandKind, agent.CommandKindContinue)
	}
}

func TestConformance_ContinueFromApprovedToolSuspensionReplaysBlockedCallOnceBeforeModel(t *testing.T) {
	t.Parallel()

	const runID = agent.RunID("conformance-tool-approval-replay")

	store := newRunStore()
	events := newEventSink()
	model := newScriptedModel(
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need approval for blocked tool call",
				ToolCalls: []agent.ToolCall{
					{ID: "call-1", Name: "lookup"},
				},
			},
		},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "completed after replay",
			},
		},
	)

	var toolExecutions atomic.Int32
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			switch toolExecutions.Add(1) {
			case 1:
				return "", &agent.SuspendRequestError{
					Requirement: &agent.PendingRequirement{
						ID:          "req-tool-approval",
						Kind:        agent.RequirementKindApproval,
						Origin:      agent.RequirementOriginTool,
						Fingerprint: "fp-call-1",
					},
				}
			case 2:
				return "replayed-ok", nil
			default:
				return "unexpected-extra-execution", nil
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-approval-replay"),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	runResult, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      runID,
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if runResult.State.Status != agent.RunStatusSuspended {
		t.Fatalf("unexpected run status: %s", runResult.State.Status)
	}
	if runResult.State.PendingRequirement == nil {
		t.Fatalf("expected pending requirement")
	}
	if runResult.State.PendingRequirement.ToolCallID != "call-1" {
		t.Fatalf("unexpected requirement tool call id: %q", runResult.State.PendingRequirement.ToolCallID)
	}
	if runResult.State.PendingRequirement.Fingerprint != "fp-call-1" {
		t.Fatalf("unexpected requirement fingerprint: %q", runResult.State.PendingRequirement.Fingerprint)
	}
	if toolExecutions.Load() != 1 {
		t.Fatalf("expected one tool execution during initial run, got %d", toolExecutions.Load())
	}
	prefix := agent.CloneMessages(runResult.State.Messages)

	continueResult, continueErr := runner.Continue(context.Background(), runID, 3, []agent.ToolDefinition{
		{Name: "lookup"},
	}, &agent.Resolution{
		RequirementID: "req-tool-approval",
		Kind:          agent.RequirementKindApproval,
		Outcome:       agent.ResolutionOutcomeApproved,
	})
	if continueErr != nil {
		t.Fatalf("continue returned error: %v", continueErr)
	}
	if continueResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("unexpected continue status: %s", continueResult.State.Status)
	}
	if continueResult.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared after continue")
	}
	if toolExecutions.Load() != 2 {
		t.Fatalf("blocked call replay must execute exactly once on continue, got %d", toolExecutions.Load())
	}
	if len(continueResult.State.Messages) <= len(prefix) {
		t.Fatalf("expected transcript growth after continue")
	}
	if !reflect.DeepEqual(continueResult.State.Messages[:len(prefix)], prefix) {
		t.Fatalf("continue mutated transcript prefix")
	}
	delta := continueResult.State.Messages[len(prefix):]
	if len(delta) < 3 {
		t.Fatalf("expected resolution, replay tool result, and assistant completion messages")
	}
	if delta[0].Role != agent.RoleUser || !strings.HasPrefix(delta[0].Content, "[resolution]") {
		t.Fatalf("unexpected resolution message: %+v", delta[0])
	}
	if delta[1].Role != agent.RoleTool || delta[1].ToolCallID != "call-1" || delta[1].Content != "replayed-ok" {
		t.Fatalf("unexpected replay tool message: %+v", delta[1])
	}
	if delta[2].Role != agent.RoleAssistant || delta[2].Content != "completed after replay" {
		t.Fatalf("unexpected assistant completion message: %+v", delta[2])
	}

	gotEvents := events.Events()
	wantTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeAssistantMessage,
		agent.EventTypeToolResult,
		agent.EventTypeRunSuspended,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
		agent.EventTypeToolResult,
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	wantSteps := []int{0, 1, 1, 1, 1, 1, 1, 2, 2, 2, 2}
	if len(gotEvents) != len(wantTypes) {
		t.Fatalf("unexpected event count: got=%d want=%d", len(gotEvents), len(wantTypes))
	}
	for i := range wantTypes {
		if gotEvents[i].Type != wantTypes[i] {
			t.Fatalf("event[%d] type mismatch: got=%s want=%s", i, gotEvents[i].Type, wantTypes[i])
		}
		if gotEvents[i].Step != wantSteps[i] {
			t.Fatalf("event[%d] step mismatch: got=%d want=%d", i, gotEvents[i].Step, wantSteps[i])
		}
	}
	if gotEvents[5].CommandKind != agent.CommandKindStart {
		t.Fatalf("unexpected start command kind: got=%s want=%s", gotEvents[5].CommandKind, agent.CommandKindStart)
	}
	if gotEvents[10].CommandKind != agent.CommandKindContinue {
		t.Fatalf("unexpected continue command kind: got=%s want=%s", gotEvents[10].CommandKind, agent.CommandKindContinue)
	}
	if gotEvents[2].ToolResult == nil || gotEvents[2].ToolResult.FailureReason != agent.ToolFailureReasonSuspended {
		t.Fatalf("unexpected initial suspended tool result payload")
	}
	if gotEvents[6].ToolResult == nil {
		t.Fatalf("missing replay tool result payload")
	}
	if gotEvents[6].ToolResult.CallID != "call-1" || gotEvents[6].ToolResult.FailureReason != "" {
		t.Fatalf("unexpected replay tool result payload: %+v", *gotEvents[6].ToolResult)
	}
}

func TestConformance_ApprovedToolReplayMismatchFailsDeterministically(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		override        agent.ApprovedToolCallReplayOverride
		wantErrContains string
	}{
		{
			name: "tool_call_id_mismatch",
			override: agent.ApprovedToolCallReplayOverride{
				ToolCallID:  "call-other",
				Fingerprint: "fp-call-1",
			},
			wantErrContains: "approved_tool_replay_override.tool_call_id",
		},
		{
			name: "fingerprint_mismatch",
			override: agent.ApprovedToolCallReplayOverride{
				ToolCallID:  "call-1",
				Fingerprint: "fp-call-other",
			},
			wantErrContains: "approved_tool_replay_override.fingerprint",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events := newEventSink()
			model := newScriptedModel()
			var toolExecutions atomic.Int32
			registry := newRegistry(map[string]handler{
				"lookup": func(_ context.Context, _ map[string]any) (string, error) {
					toolExecutions.Add(1)
					return "unexpected", nil
				},
			})
			loop, err := agentreact.New(model, registry, events)
			if err != nil {
				t.Fatalf("new loop: %v", err)
			}

			state := agent.RunState{
				ID:     agent.RunID("replay-mismatch-" + tc.name),
				Status: agent.RunStatusRunning,
				Step:   3,
				Messages: []agent.Message{
					{Role: agent.RoleUser, Content: "start"},
					{
						Role:    agent.RoleAssistant,
						Content: "tool call",
						ToolCalls: []agent.ToolCall{
							{ID: "call-1", Name: "lookup"},
						},
					},
					{
						Role:       agent.RoleTool,
						Name:       "lookup",
						ToolCallID: "call-1",
						Content:    "blocked",
					},
				},
			}
			ctx := agent.WithApprovedToolCallReplayOverride(context.Background(), tc.override)
			result, runErr := loop.Execute(ctx, state, agent.EngineInput{
				MaxSteps: 4,
				Tools: []agent.ToolDefinition{
					{Name: "lookup"},
				},
				Resolution: &agent.Resolution{
					RequirementID: "req-tool-approval",
					Kind:          agent.RequirementKindApproval,
					Outcome:       agent.ResolutionOutcomeApproved,
				},
				ResolvedRequirement: &agent.PendingRequirement{
					ID:          "req-tool-approval",
					Kind:        agent.RequirementKindApproval,
					Origin:      agent.RequirementOriginTool,
					ToolCallID:  "call-1",
					Fingerprint: "fp-call-1",
				},
			})
			if !errors.Is(runErr, agent.ErrRunStateInvalid) {
				t.Fatalf("expected ErrRunStateInvalid, got %v", runErr)
			}
			if result.Status != agent.RunStatusFailed {
				t.Fatalf("unexpected status: %s", result.Status)
			}
			if !strings.Contains(result.Error, tc.wantErrContains) {
				t.Fatalf("unexpected state error: %q", result.Error)
			}
			if toolExecutions.Load() != 0 {
				t.Fatalf("tool replay must not execute on contract mismatch, calls=%d", toolExecutions.Load())
			}
			if countEventType(events.Events(), agent.EventTypeRunFailed) != 1 {
				t.Fatalf("expected one run_failed event")
			}
			if countEventType(events.Events(), agent.EventTypeRunSuspended) != 0 {
				t.Fatalf("unexpected run_suspended event on replay mismatch")
			}
			if countEventType(events.Events(), agent.EventTypeToolResult) != 0 {
				t.Fatalf("unexpected tool_result event on replay mismatch")
			}
		})
	}
}

func TestConformance_ToolSuspensionInvalidRequirementFailsRun(t *testing.T) {
	t.Parallel()

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "need tool input",
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "lookup"},
			},
		},
	})
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:   "req-invalid",
					Kind: agent.RequirementKindApproval,
				},
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-invalid-requirement"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "conformance-tool-invalid-requirement",
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(runErr, agent.ErrRunStateInvalid) {
		t.Fatalf("expected ErrRunStateInvalid, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared on invalid tool suspension failure")
	}
	if !strings.Contains(result.State.Error, "pending_requirement.origin") {
		t.Fatalf("unexpected state error: %q", result.State.Error)
	}
	if countEventType(events.Events(), agent.EventTypeToolResult) != 1 {
		t.Fatalf("expected one tool_result event before failure")
	}
	if countEventType(events.Events(), agent.EventTypeRunFailed) != 1 {
		t.Fatalf("expected one run_failed event")
	}
	if countEventType(events.Events(), agent.EventTypeRunSuspended) != 0 {
		t.Fatalf("unexpected run_suspended event for invalid requirement")
	}
}

func TestConformance_ToolSuspendRequestWithModelOriginFailsDeterministically(t *testing.T) {
	t.Parallel()

	events := newEventSink()
	model := newScriptedModel(response{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: "need tool input",
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "lookup"},
			},
		},
	})
	registry := newRegistry(map[string]handler{
		"lookup": func(_ context.Context, _ map[string]any) (string, error) {
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:     "req-tool-wrong-origin",
					Kind:   agent.RequirementKindUserInput,
					Origin: agent.RequirementOriginModel,
				},
			}
		},
	})
	loop, err := agentreact.New(model, registry, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newCounterIDGenerator("tool-wrong-origin"),
		RunStore:    newRunStore(),
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result, runErr := runner.Run(context.Background(), agent.RunInput{
		RunID:      "conformance-tool-wrong-origin",
		UserPrompt: "start",
		MaxSteps:   3,
		Tools: []agent.ToolDefinition{
			{Name: "lookup"},
		},
	})
	if !errors.Is(runErr, agent.ErrRunStateInvalid) {
		t.Fatalf("expected ErrRunStateInvalid, got %v", runErr)
	}
	if result.State.Status != agent.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.State.Status)
	}
	if result.State.PendingRequirement != nil {
		t.Fatalf("pending requirement must be cleared on invalid tool suspend origin")
	}
	if !strings.Contains(result.State.Error, "source=tool") {
		t.Fatalf("unexpected state error: %q", result.State.Error)
	}

	var toolResultEvent *agent.Event
	for i := range events.Events() {
		current := events.Events()[i]
		if current.Type == agent.EventTypeToolResult {
			toolResultEvent = &current
			break
		}
	}
	if toolResultEvent == nil || toolResultEvent.ToolResult == nil {
		t.Fatalf("expected tool_result event")
	}
	if toolResultEvent.ToolResult.FailureReason != agent.ToolFailureReasonExecutorError {
		t.Fatalf("unexpected tool failure reason: %s", toolResultEvent.ToolResult.FailureReason)
	}
	if countEventType(events.Events(), agent.EventTypeRunSuspended) != 0 {
		t.Fatalf("unexpected run_suspended event for invalid tool requirement origin")
	}
	if countEventType(events.Events(), agent.EventTypeRunFailed) != 1 {
		t.Fatalf("expected one run_failed event")
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
