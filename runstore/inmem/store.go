package inmem

import (
	"context"
	"fmt"
	"sync"

	"agentruntime/agent"
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

func (s *Store) Save(_ context.Context, state agent.RunState) error {
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

func (s *Store) Load(_ context.Context, runID agent.RunID) (agent.RunState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[runID]
	if !ok {
		return agent.RunState{}, agent.ErrRunNotFound
	}
	return agent.CloneRunState(state), nil
}
