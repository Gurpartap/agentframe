package agent

import (
	"context"
	"testing"
	"time"
)

func TestSideEffectContext_ActiveContextUnchanged(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	got := sideEffectContext(ctx)
	if got != ctx {
		t.Fatalf("expected active context to remain unchanged")
	}

	wantDeadline, wantHasDeadline := ctx.Deadline()
	gotDeadline, gotHasDeadline := got.Deadline()
	if gotHasDeadline != wantHasDeadline {
		t.Fatalf("unexpected deadline presence: got=%t want=%t", gotHasDeadline, wantHasDeadline)
	}
	if gotHasDeadline && !gotDeadline.Equal(wantDeadline) {
		t.Fatalf("unexpected deadline: got=%v want=%v", gotDeadline, wantDeadline)
	}
}

func TestSideEffectContext_DoneContextReturnsNonCancelingContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	const key contextKey = "key"

	doneCtx, cancel := context.WithCancel(context.WithValue(context.Background(), key, "value"))
	cancel()

	got := sideEffectContext(doneCtx)
	if got.Err() != nil {
		t.Fatalf("expected non-canceling context, got err: %v", got.Err())
	}
	select {
	case <-got.Done():
		t.Fatalf("expected done context shielding, got done signal")
	default:
	}
	if got.Value(key) != "value" {
		t.Fatalf("expected context value propagation")
	}
}
