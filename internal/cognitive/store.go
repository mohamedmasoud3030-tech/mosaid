package cognitive

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of GoalStore for testing.
// Production use will persist to SQLite.
type MemoryStore struct {
	mu    sync.RWMutex
	goals map[string]Goal
	runs  map[string]Run
}

// NewMemoryStore creates a new in-memory goal store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		goals: make(map[string]Goal),
		runs:  make(map[string]Run),
	}
}

func (s *MemoryStore) SaveGoal(ctx context.Context, goal Goal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goals[goal.ID] = goal
	return nil
}

func (s *MemoryStore) GetGoal(ctx context.Context, id string) (Goal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	goal, exists := s.goals[id]
	if !exists {
		return Goal{}, fmt.Errorf("%w: %s", ErrGoalNotFound, id)
	}
	return goal, nil
}

func (s *MemoryStore) SaveRun(ctx context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	return nil
}

func (s *MemoryStore) GetRun(ctx context.Context, id string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, exists := s.runs[id]
	if !exists {
		return Run{}, fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}
	return run, nil
}

func (s *MemoryStore) ListRuns(ctx context.Context, goalID string) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Run
	for _, run := range s.runs {
		if run.GoalID == goalID {
			result = append(result, run)
		}
	}
	return result, nil
}

// generateID creates a unique ID with a prefix.
func generateID(prefix string) string {
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixNano(), randomHex(4))
}

// randomHex generates a random hex string of n bytes.
func randomHex(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = "0123456789abcdef"[time.Now().UnixNano()%16]
	}
	return fmt.Sprintf("%x", b)
}
