package chat

import "testing"

func TestStateActiveRunAndCursor(t *testing.T) {
	t.Parallel()

	state := NewState()
	if _, _, ok := state.ActiveRun(); ok {
		t.Fatalf("expected no active run")
	}

	state.SetActiveRun("run-1")
	runID, cursor, ok := state.ActiveRun()
	if !ok {
		t.Fatalf("expected active run")
	}
	if runID != "run-1" || cursor != 0 {
		t.Fatalf("unexpected active state: run_id=%q cursor=%d", runID, cursor)
	}

	if advanced := state.AdvanceCursor("run-1", 2); !advanced {
		t.Fatalf("expected cursor advance")
	}
	if advanced := state.AdvanceCursor("run-1", 2); advanced {
		t.Fatalf("expected duplicate cursor to be ignored")
	}
	if advanced := state.AdvanceCursor("run-2", 3); advanced {
		t.Fatalf("expected wrong run cursor to be ignored")
	}

	runID, cursor, ok = state.ActiveRun()
	if !ok || runID != "run-1" || cursor != 2 {
		t.Fatalf("unexpected final active state: run_id=%q cursor=%d ok=%v", runID, cursor, ok)
	}
}
