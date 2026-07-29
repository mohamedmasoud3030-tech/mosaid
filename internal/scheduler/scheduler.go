package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Scheduler struct {
	Store        *Store
	Executor     Executor
	Authorizer   Authorizer
	Owner        string
	LockTTL      time.Duration
	PollInterval time.Duration

	mu     sync.Mutex
	active map[int64]activeRun
}

type activeRun struct {
	jobID  string
	cancel context.CancelFunc
}

func New(store *Store, executor Executor, owner string) *Scheduler {
	return &Scheduler{
		Store:        store,
		Executor:     executor,
		Authorizer:   PolicyGate{},
		Owner:        owner,
		LockTTL:      5 * time.Minute,
		PollInterval: time.Second,
		active:       make(map[int64]activeRun),
	}
}

func (s *Scheduler) validate() error {
	if s.Store == nil || s.Executor == nil || s.Authorizer == nil {
		return errors.New("scheduler dependencies are required")
	}
	if s.Owner == "" || s.LockTTL <= 0 || s.PollInterval <= 0 {
		return errors.New("scheduler owner and limits are required")
	}
	return nil
}

func (s *Scheduler) RunOnce(ctx context.Context) (bool, error) {
	if err := s.validate(); err != nil {
		return false, err
	}
	run, err := s.Store.Claim(ctx, s.Owner, s.LockTTL)
	if err != nil || run == nil {
		return false, err
	}
	return true, s.execute(ctx, *run)
}

func (s *Scheduler) execute(parent context.Context, run Run) error {
	ctx, cancel := context.WithTimeout(parent, run.Job.Timeout)
	s.mu.Lock()
	s.active[run.ID] = activeRun{jobID: run.Job.ID, cancel: cancel}
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.active, run.ID)
		s.mu.Unlock()
	}()

	invocation := Invocation{
		RunID:          run.ID,
		JobID:          run.Job.ID,
		SkillID:        run.Job.SkillID,
		Input:          append([]byte(nil), run.Job.Input...),
		Class:          run.Job.Class,
		Risk:           run.Job.Risk,
		ScheduledFor:   run.ScheduledFor,
		IdempotencyKey: run.IdempotencyKey,
		ApprovalRef:    run.Job.ApprovalRef,
	}
	if err := s.Authorizer.Authorize(ctx, invocation); err != nil {
		if finishErr := s.Store.Deny(context.WithoutCancel(parent), run.ID, s.Owner, err); finishErr != nil {
			return finishErr
		}
		return nil
	}

	err := s.Executor.Execute(ctx, invocation)
	if err == nil {
		return s.Store.Complete(context.WithoutCancel(parent), run.ID, s.Owner)
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		cancelled, checkErr := s.Store.IsCancelled(context.WithoutCancel(parent), run.Job.ID)
		if checkErr != nil {
			return checkErr
		}
		if cancelled {
			return s.Store.MarkCancelled(context.WithoutCancel(parent), run.ID, s.Owner)
		}
		return s.Store.Requeue(context.WithoutCancel(parent), run.ID, s.Owner, "scheduler shutdown before completion")
	}
	return s.Store.Fail(context.WithoutCancel(parent), run.ID, s.Owner, err)
}

func (s *Scheduler) CancelJob(ctx context.Context, jobID string) error {
	if err := s.Store.Cancel(ctx, jobID); err != nil {
		return err
	}
	s.mu.Lock()
	for _, active := range s.active {
		if active.jobID == jobID {
			active.cancel()
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) Run(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	if _, err := s.Store.RecoverStale(ctx); err != nil {
		return fmt.Errorf("recover scheduler: %w", err)
	}
	for {
		didWork, err := s.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if didWork {
			continue
		}
		timer := time.NewTimer(s.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}
