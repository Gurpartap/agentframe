package inmem

import (
	"context"
	"fmt"
	"sync"

	"github.com/Gurpartap/agentframe/agent"
)

// Store persists run state in memory with optimistic version checks.
type Store struct {
	mu     sync.RWMutex
	states map[agent.RunID]agent.RunState
}

var _ agent.RunStore = (*Store)(nil)

func New() *Store {
	return &Store{states: map[agent.RunID]agent.RunState{}}
}

func (s *Store) Save(ctx context.Context, state agent.RunState) error {
	if ctx == nil {
		return agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err := agent.ValidateRunState(state); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.states[state.ID]
	switch {
	case !exists:
		if state.Version != 0 {
			return fmt.Errorf(
				"%w: run %q expected version 0 on create, got %d",
				agent.ErrRunVersionConflict,
				state.ID,
				state.Version,
			)
		}
		next := agent.CloneRunState(state)
		next.Version = 1
		s.states[state.ID] = next
		return nil
	case state.Version != current.Version:
		return fmt.Errorf(
			"%w: run %q expected version %d, got %d",
			agent.ErrRunVersionConflict,
			state.ID,
			current.Version,
			state.Version,
		)
	default:
		next := agent.CloneRunState(state)
		next.Version = current.Version + 1
		s.states[state.ID] = next
		return nil
	}
}

func (s *Store) Load(ctx context.Context, runID agent.RunID) (agent.RunState, error) {
	if ctx == nil {
		return agent.RunState{}, agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.RunState{}, ctxErr
	}
	if runID == "" {
		return agent.RunState{}, fmt.Errorf("%w: load with empty id", agent.ErrInvalidRunID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[runID]
	if !ok {
		return agent.RunState{}, agent.ErrRunNotFound
	}
	return agent.CloneRunState(state), nil
}
