package agentreact

import (
	"context"
	"errors"
	"testing"

	"agentruntime/agent"
)

func TestPublishEventRejectsInvalidPayloadBeforeSink(t *testing.T) {
	t.Parallel()

	sink := &countingEventSink{}
	err := publishEvent(context.Background(), sink, agent.Event{
		RunID: "run-1",
		Step:  1,
		Type:  agent.EventTypeToolResult,
		ToolResult: &agent.ToolResult{
			CallID: "call-1",
			Name:   "",
		},
	})
	if !errors.Is(err, agent.ErrEventInvalid) {
		t.Fatalf("expected ErrEventInvalid, got %v", err)
	}
	if sink.calls != 0 {
		t.Fatalf("sink should not be called for invalid event, got calls=%d", sink.calls)
	}
}

func TestPublishEventPublishesValidPayload(t *testing.T) {
	t.Parallel()

	sink := &countingEventSink{}
	err := publishEvent(context.Background(), sink, agent.Event{
		RunID: "run-1",
		Step:  1,
		Type:  agent.EventTypeToolResult,
		ToolResult: &agent.ToolResult{
			CallID: "call-1",
			Name:   "lookup",
		},
	})
	if err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if sink.calls != 1 {
		t.Fatalf("expected one sink call, got %d", sink.calls)
	}
}

type countingEventSink struct {
	calls int
}

func (s *countingEventSink) Publish(context.Context, agent.Event) error {
	s.calls++
	return nil
}
