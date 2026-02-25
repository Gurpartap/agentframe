package inmem_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

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

func TestStore_SaveRejectsEmptyRunID(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	err := store.Save(context.Background(), agent.RunState{
		Status: agent.RunStatusPending,
	})
	if !errors.Is(err, agent.ErrInvalidRunID) {
		t.Fatalf("expected ErrInvalidRunID, got %v", err)
	}
}

func TestStore_LoadRejectsEmptyRunID(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	_, err := store.Load(context.Background(), "")
	if !errors.Is(err, agent.ErrInvalidRunID) {
		t.Fatalf("expected ErrInvalidRunID, got %v", err)
	}
}

func TestStore_SaveFailsFastOnDoneContext(t *testing.T) {
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

			store := runstoreinmem.New()
			state := agent.RunState{
				ID:     agent.RunID("run-fast-fail-save"),
				Status: agent.RunStatusPending,
			}

			err := store.Save(tc.newContext(), state)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if _, loadErr := store.Load(context.Background(), state.ID); !errors.Is(loadErr, agent.ErrRunNotFound) {
				t.Fatalf("expected ErrRunNotFound after failed save, got %v", loadErr)
			}
		})
	}
}

func TestStore_LoadFailsFastOnDoneContext(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	state := agent.RunState{
		ID:     agent.RunID("run-fast-fail-load"),
		Status: agent.RunStatusPending,
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("seed state: %v", err)
	}

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

			loaded, err := store.Load(tc.newContext(), state.ID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if !reflect.DeepEqual(loaded, agent.RunState{}) {
				t.Fatalf("unexpected state on context failure: %+v", loaded)
			}
		})
	}
}
