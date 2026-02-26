package agent_test

import (
	"context"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
)

func TestApprovedToolCallReplayOverrideContextRoundTrip(t *testing.T) {
	t.Parallel()

	payload := agent.ApprovedToolCallReplayOverride{
		ToolCallID:  "call-1",
		Fingerprint: "fp-call-1",
	}
	ctx := agent.WithApprovedToolCallReplayOverride(nil, payload)

	got, ok := agent.ApprovedToolCallReplayOverrideFromContext(ctx)
	if !ok {
		t.Fatalf("expected payload in context")
	}
	if got != payload {
		t.Fatalf("unexpected payload: got=%+v want=%+v", got, payload)
	}
}

func TestApprovedToolCallReplayOverrideFromContextMissing(t *testing.T) {
	t.Parallel()

	got, ok := agent.ApprovedToolCallReplayOverrideFromContext(context.Background())
	if ok {
		t.Fatalf("expected no payload, got=%+v", got)
	}
}
