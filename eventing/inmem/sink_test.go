package inmem_test

import (
	"context"
	"testing"

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
