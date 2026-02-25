package agentreact_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"agentruntime/agent"
	"agentruntime/agentreact"
	eventinginmem "agentruntime/eventing/inmem"
	runstoreinmem "agentruntime/runstore/inmem"
	toolingregistry "agentruntime/tooling/registry"
)

func TestPublicRuntimeCompositionSmoke(t *testing.T) {
	ctx := context.Background()

	const (
		wrapperRunID     = agent.RunID("public-wrapper-run")
		dispatchRunID    = agent.RunID("public-dispatch-run")
		continueRunID    = agent.RunID("public-continue-run")
		steerFollowRun   = agent.RunID("public-steer-follow-run")
		cancelRunID      = agent.RunID("public-cancel-run")
		toolNameLookup   = "lookup"
		wrapperOutput    = "wrapper complete"
		dispatchOutput   = "dispatch complete"
		continueOutput   = "continue complete"
		followUpOutput   = "follow-up complete"
		continueToolCall = "continue-call-1"
		cancelToolCall   = "cancel-call-1"
		steerToolCall    = "steer-call-1"
		steerPrompt      = "steer this run"
		followUpPrompt   = "follow up now"
	)

	model := newScriptedModel(
		response{Message: agent.Message{Role: agent.RoleAssistant, Content: wrapperOutput}},
		response{Message: agent.Message{Role: agent.RoleAssistant, Content: dispatchOutput}},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool before continue",
				ToolCalls: []agent.ToolCall{
					{ID: continueToolCall, Name: toolNameLookup},
				},
			},
		},
		response{Message: agent.Message{Role: agent.RoleAssistant, Content: continueOutput}},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool before steer/follow-up",
				ToolCalls: []agent.ToolCall{
					{ID: steerToolCall, Name: toolNameLookup},
				},
			},
		},
		response{Message: agent.Message{Role: agent.RoleAssistant, Content: followUpOutput}},
		response{
			Message: agent.Message{
				Role:    agent.RoleAssistant,
				Content: "need tool before cancel",
				ToolCalls: []agent.ToolCall{
					{ID: cancelToolCall, Name: toolNameLookup},
				},
			},
		},
	)

	tools, err := toolingregistry.New(map[string]toolingregistry.Handler{
		toolNameLookup: func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-result", nil
		},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	events := eventinginmem.New()
	store := runstoreinmem.New()
	loop, err := agentreact.New(model, tools, events)
	if err != nil {
		t.Fatalf("new loop: %v", err)
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: fixedIDGenerator{},
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	wrapperResult, err := runner.Run(ctx, agent.RunInput{
		RunID:      wrapperRunID,
		UserPrompt: "wrapper path",
		MaxSteps:   2,
	})
	if err != nil {
		t.Fatalf("wrapper run returned error: %v", err)
	}
	if wrapperResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("wrapper status mismatch: got=%s want=%s", wrapperResult.State.Status, agent.RunStatusCompleted)
	}
	if wrapperResult.State.Output != wrapperOutput {
		t.Fatalf("wrapper output mismatch: got=%q want=%q", wrapperResult.State.Output, wrapperOutput)
	}

	dispatchResult, err := runner.Dispatch(ctx, agent.StartCommand{Input: agent.RunInput{
		RunID:      dispatchRunID,
		UserPrompt: "dispatch start path",
		MaxSteps:   2,
	}})
	if err != nil {
		t.Fatalf("dispatch start returned error: %v", err)
	}
	if dispatchResult.State.Status != agent.RunStatusCompleted {
		t.Fatalf("dispatch status mismatch: got=%s want=%s", dispatchResult.State.Status, agent.RunStatusCompleted)
	}
	if dispatchResult.State.Output != dispatchOutput {
		t.Fatalf("dispatch output mismatch: got=%q want=%q", dispatchResult.State.Output, dispatchOutput)
	}

	initialContinue, initialContinueErr := runner.Run(ctx, agent.RunInput{
		RunID:      continueRunID,
		UserPrompt: "start continue flow",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: toolNameLookup},
		},
	})
	if !errors.Is(initialContinueErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("continue initial error mismatch: got=%v want=%v", initialContinueErr, agent.ErrMaxStepsExceeded)
	}
	if initialContinue.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("continue initial status mismatch: got=%s want=%s", initialContinue.State.Status, agent.RunStatusMaxStepsExceeded)
	}

	continued, err := runner.Continue(ctx, continueRunID, 2, []agent.ToolDefinition{{Name: toolNameLookup}}, nil)
	if err != nil {
		t.Fatalf("continue returned error: %v", err)
	}
	if continued.State.Status != agent.RunStatusCompleted {
		t.Fatalf("continue status mismatch: got=%s want=%s", continued.State.Status, agent.RunStatusCompleted)
	}
	if continued.State.Output != continueOutput {
		t.Fatalf("continue output mismatch: got=%q want=%q", continued.State.Output, continueOutput)
	}

	initialSteerFollow, initialErr := runner.Run(ctx, agent.RunInput{
		RunID:      steerFollowRun,
		UserPrompt: "start steer-follow flow",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: toolNameLookup},
		},
	})
	if !errors.Is(initialErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("steer/follow initial error mismatch: got=%v want=%v", initialErr, agent.ErrMaxStepsExceeded)
	}
	if initialSteerFollow.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("steer/follow initial status mismatch: got=%s want=%s", initialSteerFollow.State.Status, agent.RunStatusMaxStepsExceeded)
	}

	prefixBeforeSteer := agent.CloneMessages(initialSteerFollow.State.Messages)
	steered, err := runner.Steer(ctx, steerFollowRun, steerPrompt)
	if err != nil {
		t.Fatalf("steer returned error: %v", err)
	}
	if len(steered.State.Messages) != len(prefixBeforeSteer)+1 {
		t.Fatalf("steer message count mismatch: got=%d want=%d", len(steered.State.Messages), len(prefixBeforeSteer)+1)
	}
	if !reflect.DeepEqual(steered.State.Messages[:len(prefixBeforeSteer)], prefixBeforeSteer) {
		t.Fatalf("steer mutated transcript prefix")
	}
	appendedSteer := steered.State.Messages[len(prefixBeforeSteer)]
	if appendedSteer.Role != agent.RoleUser || appendedSteer.Content != steerPrompt {
		t.Fatalf("steer appended unexpected message: %+v", appendedSteer)
	}

	prefixBeforeFollowUp := agent.CloneMessages(steered.State.Messages)
	followed, err := runner.FollowUp(ctx, steerFollowRun, followUpPrompt, 2, []agent.ToolDefinition{{Name: toolNameLookup}})
	if err != nil {
		t.Fatalf("follow-up returned error: %v", err)
	}
	if followed.State.Status != agent.RunStatusCompleted {
		t.Fatalf("follow-up status mismatch: got=%s want=%s", followed.State.Status, agent.RunStatusCompleted)
	}
	if followed.State.Output != followUpOutput {
		t.Fatalf("follow-up output mismatch: got=%q want=%q", followed.State.Output, followUpOutput)
	}
	if len(followed.State.Messages) <= len(prefixBeforeFollowUp) {
		t.Fatalf("follow-up transcript did not grow: before=%d after=%d", len(prefixBeforeFollowUp), len(followed.State.Messages))
	}
	if !reflect.DeepEqual(followed.State.Messages[:len(prefixBeforeFollowUp)], prefixBeforeFollowUp) {
		t.Fatalf("follow-up mutated transcript prefix")
	}
	appendedFollowUp := followed.State.Messages[len(prefixBeforeFollowUp)]
	if appendedFollowUp.Role != agent.RoleUser || appendedFollowUp.Content != followUpPrompt {
		t.Fatalf("follow-up appended unexpected message: %+v", appendedFollowUp)
	}

	initialCancel, initialCancelErr := runner.Run(ctx, agent.RunInput{
		RunID:      cancelRunID,
		UserPrompt: "start cancel flow",
		MaxSteps:   1,
		Tools: []agent.ToolDefinition{
			{Name: toolNameLookup},
		},
	})
	if !errors.Is(initialCancelErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("cancel initial error mismatch: got=%v want=%v", initialCancelErr, agent.ErrMaxStepsExceeded)
	}
	if initialCancel.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("cancel initial status mismatch: got=%s want=%s", initialCancel.State.Status, agent.RunStatusMaxStepsExceeded)
	}

	cancelled, err := runner.Cancel(ctx, cancelRunID)
	if err != nil {
		t.Fatalf("cancel returned error: %v", err)
	}
	if cancelled.State.Status != agent.RunStatusCancelled {
		t.Fatalf("cancel status mismatch: got=%s want=%s", cancelled.State.Status, agent.RunStatusCancelled)
	}

	allEvents := events.Events()
	assertEventCorrelationKeys(t, allEvents)
	assertCommandKindsForRun(t, allEvents, wrapperRunID, []agent.CommandKind{
		agent.CommandKindStart,
	})
	assertCommandKindsForRun(t, allEvents, dispatchRunID, []agent.CommandKind{
		agent.CommandKindStart,
	})
	assertCommandKindsForRun(t, allEvents, continueRunID, []agent.CommandKind{
		agent.CommandKindStart,
		agent.CommandKindContinue,
	})
	assertCommandKindsForRun(t, allEvents, steerFollowRun, []agent.CommandKind{
		agent.CommandKindStart,
		agent.CommandKindSteer,
		agent.CommandKindFollowUp,
	})
	assertCommandKindsForRun(t, allEvents, cancelRunID, []agent.CommandKind{
		agent.CommandKindStart,
		agent.CommandKindCancel,
	})

	assertRunEventTimeline(t, allEvents, wrapperRunID, []runEventExpectation{
		{Type: agent.EventTypeRunStarted, Step: 0},
		{Type: agent.EventTypeAssistantMessage, Step: 1},
		{Type: agent.EventTypeRunCompleted, Step: 1},
		{Type: agent.EventTypeRunCheckpoint, Step: 1},
		{Type: agent.EventTypeCommandApplied, Step: 1, CommandKind: agent.CommandKindStart},
	})
	assertRunEventTimeline(t, allEvents, dispatchRunID, []runEventExpectation{
		{Type: agent.EventTypeRunStarted, Step: 0},
		{Type: agent.EventTypeAssistantMessage, Step: 1},
		{Type: agent.EventTypeRunCompleted, Step: 1},
		{Type: agent.EventTypeRunCheckpoint, Step: 1},
		{Type: agent.EventTypeCommandApplied, Step: 1, CommandKind: agent.CommandKindStart},
	})
	assertRunEventTimeline(t, allEvents, continueRunID, []runEventExpectation{
		{Type: agent.EventTypeRunStarted, Step: 0},
		{Type: agent.EventTypeAssistantMessage, Step: 1},
		{Type: agent.EventTypeToolResult, Step: 1},
		{Type: agent.EventTypeRunFailed, Step: 1},
		{Type: agent.EventTypeRunCheckpoint, Step: 1},
		{Type: agent.EventTypeCommandApplied, Step: 1, CommandKind: agent.CommandKindStart},
		{Type: agent.EventTypeAssistantMessage, Step: 2},
		{Type: agent.EventTypeRunCompleted, Step: 2},
		{Type: agent.EventTypeRunCheckpoint, Step: 2},
		{Type: agent.EventTypeCommandApplied, Step: 2, CommandKind: agent.CommandKindContinue},
	})
	assertRunEventTimeline(t, allEvents, steerFollowRun, []runEventExpectation{
		{Type: agent.EventTypeRunStarted, Step: 0},
		{Type: agent.EventTypeAssistantMessage, Step: 1},
		{Type: agent.EventTypeToolResult, Step: 1},
		{Type: agent.EventTypeRunFailed, Step: 1},
		{Type: agent.EventTypeRunCheckpoint, Step: 1},
		{Type: agent.EventTypeCommandApplied, Step: 1, CommandKind: agent.CommandKindStart},
		{Type: agent.EventTypeRunCheckpoint, Step: 1},
		{Type: agent.EventTypeCommandApplied, Step: 1, CommandKind: agent.CommandKindSteer},
		{Type: agent.EventTypeAssistantMessage, Step: 2},
		{Type: agent.EventTypeRunCompleted, Step: 2},
		{Type: agent.EventTypeRunCheckpoint, Step: 2},
		{Type: agent.EventTypeCommandApplied, Step: 2, CommandKind: agent.CommandKindFollowUp},
	})
	assertRunEventTimeline(t, allEvents, cancelRunID, []runEventExpectation{
		{Type: agent.EventTypeRunStarted, Step: 0},
		{Type: agent.EventTypeAssistantMessage, Step: 1},
		{Type: agent.EventTypeToolResult, Step: 1},
		{Type: agent.EventTypeRunFailed, Step: 1},
		{Type: agent.EventTypeRunCheckpoint, Step: 1},
		{Type: agent.EventTypeCommandApplied, Step: 1, CommandKind: agent.CommandKindStart},
		{Type: agent.EventTypeRunCancelled, Step: 1},
		{Type: agent.EventTypeCommandApplied, Step: 1, CommandKind: agent.CommandKindCancel},
	})
}

type fixedIDGenerator struct{}

func (fixedIDGenerator) NewRunID(context.Context) (agent.RunID, error) {
	return agent.RunID("public-generated-id"), nil
}

func assertCommandKindsForRun(t *testing.T, events []agent.Event, runID agent.RunID, want []agent.CommandKind) {
	t.Helper()

	got := make([]agent.CommandKind, 0, len(want))
	for _, event := range events {
		if event.RunID != runID || event.Type != agent.EventTypeCommandApplied {
			continue
		}
		got = append(got, event.CommandKind)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command kind ordering mismatch for run=%s: got=%v want=%v", runID, got, want)
	}
}

type runEventExpectation struct {
	Type        agent.EventType
	Step        int
	CommandKind agent.CommandKind
}

func assertEventCorrelationKeys(t *testing.T, events []agent.Event) {
	t.Helper()

	for i, event := range events {
		if event.RunID == "" {
			t.Fatalf("event[%d] missing run_id: %+v", i, event)
		}
		if event.Type == agent.EventTypeCommandApplied && event.CommandKind == "" {
			t.Fatalf("event[%d] command_applied missing command_kind: %+v", i, event)
		}
	}
}

func assertRunEventTimeline(t *testing.T, events []agent.Event, runID agent.RunID, want []runEventExpectation) {
	t.Helper()

	got := make([]agent.Event, 0, len(want))
	for _, event := range events {
		if event.RunID == runID {
			got = append(got, event)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("event timeline length mismatch for run=%s: got=%d want=%d", runID, len(got), len(want))
	}
	for i := range want {
		if got[i].Type != want[i].Type {
			t.Fatalf("event type mismatch for run=%s index=%d: got=%s want=%s", runID, i, got[i].Type, want[i].Type)
		}
		if got[i].Step != want[i].Step {
			t.Fatalf("event step mismatch for run=%s index=%d type=%s: got=%d want=%d", runID, i, got[i].Type, got[i].Step, want[i].Step)
		}
		if got[i].Type == agent.EventTypeCommandApplied {
			if got[i].CommandKind == "" {
				t.Fatalf("event command kind missing for run=%s index=%d", runID, i)
			}
			if got[i].CommandKind != want[i].CommandKind {
				t.Fatalf("event command kind mismatch for run=%s index=%d: got=%s want=%s", runID, i, got[i].CommandKind, want[i].CommandKind)
			}
		}
	}
}
