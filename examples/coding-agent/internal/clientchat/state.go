package clientchat

import "sync"

type State struct {
	mu          sync.RWMutex
	activeRunID string
	cursor      int64
}

func NewState() *State {
	return &State{}
}

func (s *State) SetActiveRun(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activeRunID = runID
	s.cursor = 0
}

func (s *State) ActiveRun() (runID string, cursor int64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.activeRunID == "" {
		return "", 0, false
	}
	return s.activeRunID, s.cursor, true
}

func (s *State) AdvanceCursor(runID string, cursor int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeRunID == "" || s.activeRunID != runID {
		return false
	}
	if cursor <= s.cursor {
		return false
	}
	s.cursor = cursor
	return true
}
