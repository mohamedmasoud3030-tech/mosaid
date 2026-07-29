package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func testStore(t *testing.T) (*Store, *fakeClock, *storage.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "mosaid.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &fakeClock{now: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)}
	return NewStore(db.SQL(), clock), clock, db
}

func baseJob(id string, at time.Time) Job {
	return Job{
		ID:           id,
		SkillID:      "test.skill",
		Input:        []byte(`{"message":"hello"}`),
		Kind:         OneTime,
		Class:        ReadOnly,
		Risk:         policy.Safe,
		Timezone:     "UTC",
		NextRun:      at,
		Enabled:      true,
		Missed:       RunOnce,
		MaxAttempts:  3,
		RetryBackoff: time.Nanosecond,
		Timeout:      time.Second,
		CreationKey:  "create:" + id,
	}
}

func allowAll(context.Context, Invocation) error { return nil }

func TestOneTimeJobFiresOnce(t *testing.T) {
	store, clock, _ := testStore(t)
	if _, err := store.Create(context.Background(), baseJob("once", clock.Now())); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	s := New(store, ExecutorFunc(func(context.Context, Invocation) error {
		calls.Add(1)
		return nil
	}), "worker-a")
	s.Authorizer = AuthorizerFunc(allowAll)
	worked, err := s.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("first run worked=%v err=%v", worked, err)
	}
	worked, err = s.RunOnce(context.Background())
	if err != nil || worked {
		t.Fatalf("second run worked=%v err=%v", worked, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	history, err := store.History(context.Background(), "once")
	if err != nil || len(history) != 1 || history[0].State != "completed" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestRecurringNextRunCalculation(t *testing.T) {
	anchor := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	now := anchor.Add(3*time.Hour + 12*time.Minute)
	got := NextRecurring(anchor, time.Hour, now)
	want := anchor.Add(4 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("next=%s want=%s", got, want)
	}
	store, clock, _ := testStore(t)
	job := baseJob("recurring", clock.Now())
	job.Kind = Recurring
	job.Every = time.Hour
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	run, err := store.Claim(context.Background(), "worker", time.Minute)
	if err != nil || run == nil {
		t.Fatalf("run=%v err=%v", run, err)
	}
	updated, err := store.Get(context.Background(), job.ID)
	if err != nil || !updated.NextRun.Equal(clock.Now().Add(time.Hour)) {
		t.Fatalf("next=%s err=%v", updated.NextRun, err)
	}
}

func TestInvalidTimezoneRejected(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("bad-zone", clock.Now())
	job.Timezone = "Mars/Olympus_Mons"
	if _, err := store.Create(context.Background(), job); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("err=%v", err)
	}
}

func TestSkipMissedRun(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("skip-missed", clock.Now().Add(-4*time.Hour))
	job.Kind = Recurring
	job.Every = time.Hour
	job.Missed = Skip
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	run, err := store.Claim(context.Background(), "worker", time.Minute)
	if err != nil || run != nil {
		t.Fatalf("run=%v err=%v", run, err)
	}
	history, err := store.History(context.Background(), job.ID)
	if err != nil || len(history) != 1 || history[0].State != "skipped" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	updated, _ := store.Get(context.Background(), job.ID)
	if !updated.NextRun.After(clock.Now()) {
		t.Fatalf("next run was not advanced: %s", updated.NextRun)
	}
}

func TestRunOnceMissedRun(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("run-missed-once", clock.Now().Add(-4*time.Hour))
	job.Kind = Recurring
	job.Every = time.Hour
	job.Missed = RunOnce
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	run, err := store.Claim(context.Background(), "worker", time.Minute)
	if err != nil || run == nil || !run.ScheduledFor.Equal(job.NextRun) {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if err = store.Complete(context.Background(), run.ID, "worker"); err != nil {
		t.Fatal(err)
	}
	updated, _ := store.Get(context.Background(), job.ID)
	if !updated.NextRun.After(clock.Now()) {
		t.Fatalf("next run was not advanced: %s", updated.NextRun)
	}
}

func TestOverlapBlocked(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("overlap", clock.Now())
	job.Kind = Recurring
	job.Every = time.Second
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	first, err := store.Claim(context.Background(), "worker-a", time.Minute)
	if err != nil || first == nil {
		t.Fatalf("first=%v err=%v", first, err)
	}
	clock.Advance(2 * time.Second)
	second, err := store.Claim(context.Background(), "worker-b", time.Minute)
	if err != nil || second != nil {
		t.Fatalf("overlap was claimed: %+v err=%v", second, err)
	}
}

func TestExpiredLockRecovered(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("stale", clock.Now())
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	first, err := store.Claim(context.Background(), "dead-worker", time.Second)
	if err != nil || first == nil {
		t.Fatal(err)
	}
	clock.Advance(3 * time.Second)
	recovered, err := store.RecoverStale(context.Background())
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	second, err := store.Claim(context.Background(), "new-worker", time.Second)
	if err != nil || second == nil || second.ID != first.ID || second.Attempts != 2 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
}

func TestRestartBeforeExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart-before.db")
	clock := &fakeClock{now: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)}
	firstDB, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	firstStore := NewStore(firstDB.SQL(), clock)
	if _, err = firstStore.Create(context.Background(), baseJob("restart-before", clock.Now())); err != nil {
		t.Fatal(err)
	}
	if err = firstDB.Close(); err != nil {
		t.Fatal(err)
	}
	secondDB, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondDB.Close()
	run, err := NewStore(secondDB.SQL(), clock).Claim(context.Background(), "after-restart", time.Second)
	if err != nil || run == nil {
		t.Fatalf("run=%v err=%v", run, err)
	}
}

func TestRestartDuringExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart-running.db")
	clock := &fakeClock{now: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)}
	firstDB, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	firstStore := NewStore(firstDB.SQL(), clock)
	job := baseJob("restart-running", clock.Now())
	if _, err = firstStore.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if run, claimErr := firstStore.Claim(context.Background(), "crashed", time.Second); claimErr != nil || run == nil {
		t.Fatalf("run=%v err=%v", run, claimErr)
	}
	if err = firstDB.Close(); err != nil {
		t.Fatal(err)
	}
	clock.Advance(3 * time.Second)
	secondDB, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondDB.Close()
	secondStore := NewStore(secondDB.SQL(), clock)
	if n, recoverErr := secondStore.RecoverStale(context.Background()); recoverErr != nil || n != 1 {
		t.Fatalf("recovered=%d err=%v", n, recoverErr)
	}
	run, err := secondStore.Claim(context.Background(), "replacement", time.Second)
	if err != nil || run == nil || run.Attempts != 2 {
		t.Fatalf("run=%+v err=%v", run, err)
	}
}

func TestDuplicateCreationIdempotencyKey(t *testing.T) {
	store, clock, _ := testStore(t)
	first := baseJob("first", clock.Now())
	first.CreationKey = "request-123"
	if _, err := store.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := baseJob("second", clock.Now())
	second.CreationKey = first.CreationKey
	if _, err := store.Create(context.Background(), second); !errors.Is(err, ErrDuplicateCreationKey) {
		t.Fatalf("err=%v", err)
	}
}

func TestRetryExhaustion(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("retry", clock.Now())
	job.MaxAttempts = 2
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	s := New(store, ExecutorFunc(func(context.Context, Invocation) error { return errors.New("provider unavailable") }), "worker")
	s.Authorizer = AuthorizerFunc(allowAll)
	for i := 0; i < 2; i++ {
		worked, err := s.RunOnce(context.Background())
		if err != nil || !worked {
			t.Fatalf("attempt %d worked=%v err=%v", i+1, worked, err)
		}
		clock.Advance(time.Second)
	}
	history, err := store.History(context.Background(), job.ID)
	if err != nil || len(history) != 1 || history[0].State != "dead" || history[0].Attempts != 2 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	worked, err := s.RunOnce(context.Background())
	if err != nil || worked {
		t.Fatalf("retry budget was not final: worked=%v err=%v", worked, err)
	}
}

func TestDisabledJobIgnored(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("disabled", clock.Now())
	job.Enabled = false
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	run, err := store.Claim(context.Background(), "worker", time.Minute)
	if err != nil || run != nil {
		t.Fatalf("run=%v err=%v", run, err)
	}
}

func TestConcurrentWorkersCannotClaimSameRun(t *testing.T) {
	store, clock, _ := testStore(t)
	if _, err := store.Create(context.Background(), baseJob("concurrent", clock.Now())); err != nil {
		t.Fatal(err)
	}
	const workers = 12
	var claimed atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			run, err := store.Claim(context.Background(), "worker-"+time.Duration(worker).String(), time.Minute)
			if err != nil {
				errs <- err
				return
			}
			if run != nil {
				claimed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if claimed.Load() != 1 {
		t.Fatalf("claimed=%d", claimed.Load())
	}
}

func TestPolicyDenial(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("denied", clock.Now())
	job.Class = Write
	job.Risk = policy.High
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var executed atomic.Bool
	s := New(store, ExecutorFunc(func(context.Context, Invocation) error {
		executed.Store(true)
		return nil
	}), "worker")
	s.Authorizer = PolicyGate{}
	worked, err := s.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if executed.Load() {
		t.Fatal("executor ran after policy denial")
	}
	history, err := store.History(context.Background(), job.ID)
	if err != nil || len(history) != 1 || history[0].State != "denied" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestDatabaseTransactionFailure(t *testing.T) {
	store, clock, db := testStore(t)
	if _, err := store.Create(context.Background(), baseJob("db-failure", clock.Now())); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker", time.Minute); err == nil {
		t.Fatal("expected transaction failure from closed database")
	}
}

func TestCancellation(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("cancel", clock.Now())
	job.Timeout = time.Minute
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	s := New(store, ExecutorFunc(func(ctx context.Context, _ Invocation) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}), "worker")
	s.Authorizer = AuthorizerFunc(allowAll)
	done := make(chan error, 1)
	go func() {
		_, err := s.RunOnce(context.Background())
		done <- err
	}()
	<-started
	if err := s.CancelJob(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	history, err := store.History(context.Background(), job.ID)
	if err != nil || len(history) != 1 || history[0].State != "cancelled" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestCleanShutdownRequeuesActiveRun(t *testing.T) {
	store, clock, _ := testStore(t)
	job := baseJob("shutdown", clock.Now())
	job.Timeout = time.Minute
	if _, err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	s := New(store, ExecutorFunc(func(ctx context.Context, _ Invocation) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}), "worker")
	s.Authorizer = AuthorizerFunc(allowAll)
	s.PollInterval = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not shut down cleanly")
	}
	history, err := store.History(context.Background(), job.ID)
	if err != nil || len(history) != 1 || history[0].State != "pending" || history[0].Attempts != 0 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

type recordingApproval struct {
	binding ApprovalBinding
}

func (r *recordingApproval) VerifyScheduled(_ context.Context, binding ApprovalBinding) error {
	r.binding = binding
	return nil
}

func TestPublishApprovalBoundToContentAndTime(t *testing.T) {
	approval := &recordingApproval{}
	gate := PolicyGate{Approvals: approval}
	when := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	in := Invocation{SkillID: "social.publish", Input: []byte(`{"caption":"hello"}`), Class: Publish, Risk: policy.High, ScheduledFor: when, IdempotencyKey: "run-1", ApprovalRef: "approval-1"}
	if err := gate.Authorize(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if approval.binding.Reference != in.ApprovalRef || approval.binding.ScheduledFor != when || approval.binding.InputHash == "" || approval.binding.IdempotencyKey != in.IdempotencyKey {
		t.Fatalf("binding=%+v", approval.binding)
	}
}
