package inmem_test

import (
	"context"
	"errors"
	"testing"

	"agentruntime/agent"
	runstoreinmem "agentruntime/runstore/inmem"
)

func TestStore_SaveVersioningAndConflict(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	runID := agent.RunID("run-1")
	initial := agent.RunState{
		ID:     runID,
		Status: agent.RunStatusPending,
	}

	if err := store.Save(context.Background(), initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	firstSnapshot, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load first snapshot: %v", err)
	}
	if firstSnapshot.Version != 1 {
		t.Fatalf("unexpected first version: %d", firstSnapshot.Version)
	}

	updated := firstSnapshot
	updated.Step = 1
	if err := store.Save(context.Background(), updated); err != nil {
		t.Fatalf("save updated state: %v", err)
	}

	secondSnapshot, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load second snapshot: %v", err)
	}
	if secondSnapshot.Version != 2 {
		t.Fatalf("unexpected second version: %d", secondSnapshot.Version)
	}

	stale := firstSnapshot
	stale.Step = 99
	err = store.Save(context.Background(), stale)
	if !errors.Is(err, agent.ErrRunVersionConflict) {
		t.Fatalf("expected ErrRunVersionConflict, got %v", err)
	}

	latest, err := store.Load(context.Background(), runID)
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	if latest.Version != secondSnapshot.Version || latest.Step != secondSnapshot.Step {
		t.Fatalf("state changed after stale write attempt: got=%+v want=%+v", latest, secondSnapshot)
	}
}
