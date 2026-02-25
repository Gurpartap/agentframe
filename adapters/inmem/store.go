package inmem

import (
	"context"
	"sync"

	"agentruntime/agent"
)

// RunStore is a simple in-memory implementation for local development and tests.
type RunStore struct {
	mu    sync.RWMutex
	state map[agent.RunID]agent.RunState
}

func NewRunStore() *RunStore {
	return &RunStore{
		state: map[agent.RunID]agent.RunState{},
	}
}

func (s *RunStore) Save(_ context.Context, runState agent.RunState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[runState.ID] = agent.CloneRunState(runState)
	return nil
}

func (s *RunStore) Load(_ context.Context, runID agent.RunID) (agent.RunState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runState, ok := s.state[runID]
	if !ok {
		return agent.RunState{}, agent.ErrRunNotFound
	}
	return agent.CloneRunState(runState), nil
}
