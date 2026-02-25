package inmem_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentruntime/agent"
	eventinginmem "agentruntime/eventing/inmem"
)

func TestSink_EventsReturnsDeepClonedSnapshot(t *testing.T) {
	t.Parallel()

	sink := eventinginmem.New()
	message := agent.Message{Role: agent.RoleAssistant, Content: "hello"}
	toolResult := agent.ToolResult{CallID: "call-1", Name: "lookup", Content: "result"}

	input := agent.Event{
		RunID:      "run-1",
		Step:       1,
		Type:       agent.EventTypeAssistantMessage,
		Message:    &message,
		ToolResult: &toolResult,
	}
	if err := sink.Publish(context.Background(), input); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	input.Message.Content = "mutated"
	input.ToolResult.Content = "mutated"

	snapshot := sink.Events()
	if len(snapshot) != 1 {
		t.Fatalf("unexpected snapshot length: %d", len(snapshot))
	}
	if snapshot[0].Message == nil || snapshot[0].Message.Content != "hello" {
		t.Fatalf("unexpected message snapshot: %+v", snapshot[0].Message)
	}
	if snapshot[0].ToolResult == nil || snapshot[0].ToolResult.Content != "result" {
		t.Fatalf("unexpected tool result snapshot: %+v", snapshot[0].ToolResult)
	}

	snapshot[0].Message.Content = "changed"
	snapshot[0].ToolResult.Content = "changed"

	next := sink.Events()
	if next[0].Message == nil || next[0].Message.Content != "hello" {
		t.Fatalf("snapshot mutation leaked into sink message: %+v", next[0].Message)
	}
	if next[0].ToolResult == nil || next[0].ToolResult.Content != "result" {
		t.Fatalf("snapshot mutation leaked into sink tool result: %+v", next[0].ToolResult)
	}
}

func TestSink_PublishFailsFastOnDoneContext(t *testing.T) {
	t.Parallel()

	newCanceledContext := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	newDeadlineContext := func() context.Context {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		cancel()
		return ctx
	}

	tests := []struct {
		name       string
		newContext func() context.Context
		wantErr    error
	}{
		{
			name:       "canceled",
			newContext: newCanceledContext,
			wantErr:    context.Canceled,
		},
		{
			name:       "deadline_exceeded",
			newContext: newDeadlineContext,
			wantErr:    context.DeadlineExceeded,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := eventinginmem.New()
			err := sink.Publish(tc.newContext(), agent.Event{
				RunID: "run-ctx-fail-fast",
				Type:  agent.EventTypeRunCheckpoint,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if got := sink.Events(); len(got) != 0 {
				t.Fatalf("expected no events after failed publish, got %d", len(got))
			}
		})
	}
}
