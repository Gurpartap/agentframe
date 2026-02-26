package inmem_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
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
	if !errors.Is(err, agent.ErrRunStateInvalid) {
		t.Fatalf("expected ErrRunStateInvalid, got %v", err)
	}
	if !errors.Is(err, agent.ErrInvalidRunID) {
		t.Fatalf("expected ErrInvalidRunID compatibility, got %v", err)
	}
	if _, loadErr := store.Load(context.Background(), agent.RunID("probe-empty-id")); !errors.Is(loadErr, agent.ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound after invalid save, got %v", loadErr)
	}
}

func TestStore_SaveRejectsStructurallyInvalidRunStateWithoutSideEffects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		state            agent.RunState
		probeRunID       agent.RunID
		wantInvalidRunID bool
	}{
		{
			name: "negative step",
			state: agent.RunState{
				ID:      "run-invalid-step",
				Version: 0,
				Step:    -1,
				Status:  agent.RunStatusPending,
			},
			probeRunID: "run-invalid-step",
		},
		{
			name: "negative version",
			state: agent.RunState{
				ID:      "run-invalid-version",
				Version: -1,
				Step:    0,
				Status:  agent.RunStatusPending,
			},
			probeRunID: "run-invalid-version",
		},
		{
			name: "empty status",
			state: agent.RunState{
				ID:      "run-invalid-status-empty",
				Version: 0,
				Step:    0,
				Status:  "",
			},
			probeRunID: "run-invalid-status-empty",
		},
		{
			name: "unknown status",
			state: agent.RunState{
				ID:      "run-invalid-status-unknown",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatus("mystery"),
			},
			probeRunID: "run-invalid-status-unknown",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := runstoreinmem.New()
			seed := agent.RunState{
				ID:      "seed-run",
				Version: 0,
				Step:    0,
				Status:  agent.RunStatusPending,
			}
			if err := store.Save(context.Background(), seed); err != nil {
				t.Fatalf("seed save: %v", err)
			}
			persistedBefore, err := store.Load(context.Background(), seed.ID)
			if err != nil {
				t.Fatalf("load seeded state: %v", err)
			}

			err = store.Save(context.Background(), tc.state)
			if !errors.Is(err, agent.ErrRunStateInvalid) {
				t.Fatalf("expected ErrRunStateInvalid, got %v", err)
			}
			if tc.wantInvalidRunID && !errors.Is(err, agent.ErrInvalidRunID) {
				t.Fatalf("expected ErrInvalidRunID compatibility, got %v", err)
			}
			if _, loadErr := store.Load(context.Background(), tc.probeRunID); !errors.Is(loadErr, agent.ErrRunNotFound) {
				t.Fatalf("expected ErrRunNotFound for rejected state probe load, got %v", loadErr)
			}

			persistedAfter, err := store.Load(context.Background(), seed.ID)
			if err != nil {
				t.Fatalf("reload seeded state: %v", err)
			}
			if !reflect.DeepEqual(persistedAfter, persistedBefore) {
				t.Fatalf("persisted seeded state changed after rejected save: got=%+v want=%+v", persistedAfter, persistedBefore)
			}
		})
	}
}

func TestStore_LoadRejectsEmptyRunID(t *testing.T) {
	t.Parallel()

	store := runstoreinmem.New()
	_, err := store.Load(context.Background(), "")
	if !errors.Is(err, agent.ErrInvalidRunID) {
		t.Fatalf("expected ErrInvalidRunID, got %v", err)
	}
	if errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("expected empty-id load not to match ErrRunNotFound, got %v", err)
	}
}

func TestStore_NilContextRejectedWithoutSideEffects(t *testing.T) {
	t.Parallel()

	t.Run("save", func(t *testing.T) {
		t.Parallel()

		store := runstoreinmem.New()
		state := agent.RunState{
			ID:     agent.RunID("run-nil-save"),
			Status: agent.RunStatusPending,
		}

		err := store.Save(nil, state)
		if !errors.Is(err, agent.ErrContextNil) {
			t.Fatalf("expected ErrContextNil, got %v", err)
		}
		if _, loadErr := store.Load(context.Background(), state.ID); !errors.Is(loadErr, agent.ErrRunNotFound) {
			t.Fatalf("expected ErrRunNotFound after nil-context save rejection, got %v", loadErr)
		}
	})

	t.Run("load", func(t *testing.T) {
		t.Parallel()

		store := runstoreinmem.New()
		seed := agent.RunState{
			ID:     agent.RunID("run-nil-load"),
			Status: agent.RunStatusPending,
		}
		if err := store.Save(context.Background(), seed); err != nil {
			t.Fatalf("seed state: %v", err)
		}
		persistedBefore, err := store.Load(context.Background(), seed.ID)
		if err != nil {
			t.Fatalf("load seed: %v", err)
		}

		loaded, err := store.Load(nil, seed.ID)
		if !errors.Is(err, agent.ErrContextNil) {
			t.Fatalf("expected ErrContextNil, got %v", err)
		}
		if !reflect.DeepEqual(loaded, agent.RunState{}) {
			t.Fatalf("unexpected state on nil-context load rejection: %+v", loaded)
		}

		persistedAfter, err := store.Load(context.Background(), seed.ID)
		if err != nil {
			t.Fatalf("reload seed: %v", err)
		}
		if !reflect.DeepEqual(persistedAfter, persistedBefore) {
			t.Fatalf("persisted state mutated by nil-context load: got=%+v want=%+v", persistedAfter, persistedBefore)
		}
	})
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
