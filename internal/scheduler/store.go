package scheduler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

const jobSelectColumns = `id,skill_id,input_json,kind,class,risk,timezone,interval_ns,next_run,enabled,missed_policy,max_attempts,retry_backoff_ns,timeout_ns,creation_key,approval_ref,cancelled_at,created_at,updated_at`

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type Store struct {
	db    *sql.DB
	clock Clock
}

func NewStore(db *sql.DB, clock Clock) *Store {
	if clock == nil {
		clock = RealClock{}
	}
	return &Store{db: db, clock: clock}
}

func (s *Store) Create(ctx context.Context, input Job) (Job, error) {
	job, err := normalizeJob(input, s.clock.Now())
	if err != nil {
		return Job{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM scheduled_jobs WHERE creation_key=?`, job.CreationKey).Scan(&exists); err != nil {
		return Job{}, err
	}
	if exists != 0 {
		return Job{}, ErrDuplicateCreationKey
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO scheduled_jobs(id,skill_id,input_json,kind,class,risk,timezone,interval_ns,next_run,enabled,missed_policy,max_attempts,retry_backoff_ns,timeout_ns,creation_key,approval_ref,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		job.ID, job.SkillID, job.Input, job.Kind, job.Class, job.Risk, job.Timezone, int64(job.Every), formatTime(job.NextRun), boolInt(job.Enabled), job.Missed, job.MaxAttempts, int64(job.RetryBackoff), int64(job.Timeout), job.CreationKey, job.ApprovalRef, formatTime(job.CreatedAt), formatTime(job.UpdatedAt))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Job{}, ErrDuplicateCreationKey
		}
		return Job{}, err
	}
	return job, tx.Commit()
}

func normalizeJob(input Job, now time.Time) (Job, error) {
	job := input
	now = now.UTC()
	if !identifierPattern.MatchString(job.ID) || !identifierPattern.MatchString(job.SkillID) {
		return Job{}, fmt.Errorf("%w: id and skill id must be bounded identifiers", ErrInvalidJob)
	}
	if len(job.Input) == 0 {
		job.Input = []byte(`{}`)
	}
	var object map[string]json.RawMessage
	if !json.Valid(job.Input) || json.Unmarshal(job.Input, &object) != nil || object == nil {
		return Job{}, fmt.Errorf("%w: input must be a JSON object", ErrInvalidJob)
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, job.Input); err != nil {
		return Job{}, fmt.Errorf("%w: malformed input", ErrInvalidJob)
	}
	job.Input = buffer.Bytes()
	if job.Timezone == "" {
		job.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(job.Timezone); err != nil {
		return Job{}, fmt.Errorf("%w: invalid timezone", ErrInvalidJob)
	}
	if job.NextRun.IsZero() {
		return Job{}, fmt.Errorf("%w: next run is required", ErrInvalidJob)
	}
	job.NextRun = job.NextRun.UTC()
	if job.Kind == "" {
		job.Kind = OneTime
	}
	switch job.Kind {
	case OneTime:
		if job.Every != 0 {
			return Job{}, fmt.Errorf("%w: one-time job cannot have an interval", ErrInvalidJob)
		}
	case Recurring:
		if job.Every <= 0 || job.Every > 366*24*time.Hour {
			return Job{}, fmt.Errorf("%w: invalid recurring interval", ErrInvalidJob)
		}
	default:
		return Job{}, fmt.Errorf("%w: invalid kind", ErrInvalidJob)
	}
	if job.Class == "" {
		job.Class = ReadOnly
	}
	switch job.Class {
	case Reminder, ReadOnly, Write, Publish:
	default:
		return Job{}, fmt.Errorf("%w: invalid class", ErrInvalidJob)
	}
	if job.Risk == "" {
		switch job.Class {
		case Reminder, ReadOnly:
			job.Risk = policy.Safe
		case Write:
			job.Risk = policy.Medium
		case Publish:
			job.Risk = policy.High
		}
	}
	switch job.Risk {
	case policy.Safe, policy.Low, policy.Medium, policy.High, policy.Critical:
	default:
		return Job{}, fmt.Errorf("%w: invalid risk", ErrInvalidJob)
	}
	if job.Missed == "" {
		job.Missed = RunOnce
	}
	if job.Missed != Skip && job.Missed != RunOnce {
		return Job{}, fmt.Errorf("%w: invalid missed-run policy", ErrInvalidJob)
	}
	if job.MaxAttempts == 0 {
		job.MaxAttempts = 3
	}
	if job.MaxAttempts < 1 || job.MaxAttempts > 20 {
		return Job{}, fmt.Errorf("%w: max attempts out of bounds", ErrInvalidJob)
	}
	if job.RetryBackoff == 0 {
		job.RetryBackoff = time.Second
	}
	if job.RetryBackoff < 0 || job.RetryBackoff > 24*time.Hour {
		return Job{}, fmt.Errorf("%w: retry backoff out of bounds", ErrInvalidJob)
	}
	if job.Timeout == 0 {
		job.Timeout = time.Minute
	}
	if job.Timeout <= 0 || job.Timeout > 24*time.Hour {
		return Job{}, fmt.Errorf("%w: timeout out of bounds", ErrInvalidJob)
	}
	if job.CreationKey == "" {
		job.CreationKey = "create:" + job.ID
	}
	if len(job.CreationKey) > 256 {
		return Job{}, fmt.Errorf("%w: creation key too long", ErrInvalidJob)
	}
	if job.Class == Publish && job.ApprovalRef == "" {
		return Job{}, fmt.Errorf("%w: publishing requires a bound approval reference", ErrInvalidJob)
	}
	job.Enabled = input.Enabled
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	return job, nil
}

func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobSelectColumns+` FROM scheduled_jobs WHERE id=?`, id))
}

func (s *Store) SetEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE scheduled_jobs SET enabled=?,updated_at=? WHERE id=? AND cancelled_at IS NULL`, boolInt(enabled), formatTime(s.clock.Now()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Cancel(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(s.clock.Now())
	res, err := tx.ExecContext(ctx, `UPDATE scheduled_jobs SET enabled=0,cancelled_at=COALESCE(cancelled_at,?),updated_at=? WHERE id=?`, now, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `UPDATE scheduled_runs SET state='cancelled',finished_at=?,last_error='job cancelled',updated_at=? WHERE job_id=? AND state='pending'`, now, now, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) IsCancelled(ctx context.Context, id string) (bool, error) {
	var cancelled sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT cancelled_at FROM scheduled_jobs WHERE id=?`, id).Scan(&cancelled)
	return cancelled.Valid, err
}

func (s *Store) Claim(ctx context.Context, owner string, lockTTL time.Duration) (*Run, error) {
	if owner == "" || lockTTL <= 0 {
		return nil, errors.New("scheduler owner and positive lock TTL required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.clock.Now().UTC()
	nowText := formatTime(now)
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_locks WHERE expires_at<=?`, nowText); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE scheduled_runs SET state='pending',available_at=?,last_error='recovered after stale lock',updated_at=? WHERE state='running' AND NOT EXISTS(SELECT 1 FROM job_locks l WHERE l.job_id=scheduled_runs.job_id)`, nowText, nowText); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE scheduled_runs SET state='dead',finished_at=?,last_error='retry budget exhausted during recovery',updated_at=? WHERE state='pending' AND attempts>=(SELECT max_attempts FROM scheduled_jobs j WHERE j.id=scheduled_runs.job_id)`, nowText, nowText); err != nil {
		return nil, err
	}

	run, err := claimPendingRun(ctx, tx, owner, now, lockTTL)
	if err != nil {
		return nil, err
	}
	if run != nil {
		return run, tx.Commit()
	}
	run, err = claimDueJob(ctx, tx, owner, now, lockTTL)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, tx.Commit()
	}
	return run, tx.Commit()
}

func claimPendingRun(ctx context.Context, tx *sql.Tx, owner string, now time.Time, lockTTL time.Duration) (*Run, error) {
	query := `SELECT r.id,r.scheduled_for,r.idempotency_key,r.state,r.attempts,r.available_at,r.started_at,r.finished_at,r.last_error,` + jobSelectColumnsWithPrefix("j") + ` FROM scheduled_runs r JOIN scheduled_jobs j ON j.id=r.job_id WHERE r.state='pending' AND r.available_at<=? AND j.cancelled_at IS NULL AND NOT EXISTS(SELECT 1 FROM job_locks l WHERE l.job_id=r.job_id) ORDER BY r.available_at,r.id LIMIT 1`
	run, err := scanRun(tx.QueryRowContext(ctx, query, formatTime(now)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ok, err := acquireLock(ctx, tx, run.Job, owner, now, lockTTL); err != nil || !ok {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE scheduled_runs SET state='running',attempts=attempts+1,started_at=?,updated_at=? WHERE id=? AND state='pending'`, formatTime(now), formatTime(now), run.ID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, nil
	}
	run.State = "running"
	run.Attempts++
	started := now
	run.StartedAt = &started
	return &run, nil
}

func claimDueJob(ctx context.Context, tx *sql.Tx, owner string, now time.Time, lockTTL time.Duration) (*Run, error) {
	query := `SELECT ` + jobSelectColumns + ` FROM scheduled_jobs j WHERE enabled=1 AND cancelled_at IS NULL AND next_run<=? AND NOT EXISTS(SELECT 1 FROM job_locks l WHERE l.job_id=j.id) ORDER BY next_run,id LIMIT 1`
	job, err := scanJob(tx.QueryRowContext(ctx, query, formatTime(now)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ok, err := acquireLock(ctx, tx, job, owner, now, lockTTL); err != nil || !ok {
		return nil, err
	}
	scheduled := job.NextRun.UTC()
	key := runKey(job.ID, scheduled)
	if job.Kind == Recurring && job.Missed == Skip && !scheduled.Add(job.Every).After(now) {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO scheduled_runs(job_id,scheduled_for,idempotency_key,state,available_at,finished_at,last_error,created_at,updated_at) VALUES(?,?,?,'skipped',?,?, 'missed run skipped',?,?)`, job.ID, formatTime(scheduled), key, formatTime(now), formatTime(now), formatTime(now), formatTime(now)); err != nil {
			return nil, err
		}
		next := NextRecurring(scheduled, job.Every, now)
		if _, err = tx.ExecContext(ctx, `UPDATE scheduled_jobs SET next_run=?,updated_at=? WHERE id=?`, formatTime(next), formatTime(now), job.ID); err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM job_locks WHERE job_id=? AND owner=?`, job.ID, owner)
		return nil, err
	}
	if job.Kind == Recurring {
		next := NextRecurring(scheduled, job.Every, now)
		if _, err = tx.ExecContext(ctx, `UPDATE scheduled_jobs SET next_run=?,updated_at=? WHERE id=?`, formatTime(next), formatTime(now), job.ID); err != nil {
			return nil, err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE scheduled_jobs SET enabled=0,updated_at=? WHERE id=?`, formatTime(now), job.ID); err != nil {
			return nil, err
		}
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO scheduled_runs(job_id,scheduled_for,idempotency_key,state,attempts,available_at,started_at,created_at,updated_at) VALUES(?,?,?,'running',1,?,?,?,?)`, job.ID, formatTime(scheduled), key, formatTime(now), formatTime(now), formatTime(now), formatTime(now))
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, err = tx.ExecContext(ctx, `DELETE FROM job_locks WHERE job_id=? AND owner=?`, job.ID, owner)
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	started := now
	return &Run{ID: id, Job: job, ScheduledFor: scheduled, IdempotencyKey: key, State: "running", Attempts: 1, AvailableAt: now, StartedAt: &started}, nil
}

func acquireLock(ctx context.Context, tx *sql.Tx, job Job, owner string, now time.Time, lockTTL time.Duration) (bool, error) {
	ttl := lockTTL
	if job.Timeout+time.Second > ttl {
		ttl = job.Timeout + time.Second
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO job_locks(job_id,owner,acquired_at,expires_at) VALUES(?,?,?,?)`, job.ID, owner, formatTime(now), formatTime(now.Add(ttl)))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *Store) Complete(ctx context.Context, runID int64, owner string) error {
	return s.finish(ctx, runID, owner, "completed", "")
}

func (s *Store) Deny(ctx context.Context, runID int64, owner string, cause error) error {
	return s.finish(ctx, runID, owner, "denied", safeSummary(cause))
}

func (s *Store) MarkCancelled(ctx context.Context, runID int64, owner string) error {
	return s.finish(ctx, runID, owner, "cancelled", "job cancelled")
}

func (s *Store) finish(ctx context.Context, runID int64, owner, state, summary string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(s.clock.Now())
	res, err := tx.ExecContext(ctx, `UPDATE scheduled_runs SET state=?,finished_at=?,last_error=?,updated_at=? WHERE id=? AND state='running'`, state, now, summary, now, runID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("scheduled run is not running")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_locks WHERE job_id=(SELECT job_id FROM scheduled_runs WHERE id=?) AND owner=?`, runID, owner); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Fail(ctx context.Context, runID int64, owner string, cause error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempts, maxAttempts int
	var backoffNS int64
	if err = tx.QueryRowContext(ctx, `SELECT r.attempts,j.max_attempts,j.retry_backoff_ns FROM scheduled_runs r JOIN scheduled_jobs j ON j.id=r.job_id WHERE r.id=? AND r.state='running'`, runID).Scan(&attempts, &maxAttempts, &backoffNS); err != nil {
		return err
	}
	now := s.clock.Now().UTC()
	state := "pending"
	finished := any(nil)
	available := now
	if attempts >= maxAttempts {
		state = "dead"
		finished = formatTime(now)
	} else {
		delay := boundedBackoff(time.Duration(backoffNS), attempts)
		available = now.Add(delay)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE scheduled_runs SET state=?,available_at=?,finished_at=?,last_error=?,updated_at=? WHERE id=? AND state='running'`, state, formatTime(available), finished, safeSummary(cause), formatTime(now), runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_locks WHERE job_id=(SELECT job_id FROM scheduled_runs WHERE id=?) AND owner=?`, runID, owner); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Requeue(ctx context.Context, runID int64, owner string, summary string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(s.clock.Now())
	if _, err = tx.ExecContext(ctx, `UPDATE scheduled_runs SET state='pending',attempts=CASE WHEN attempts>0 THEN attempts-1 ELSE 0 END,available_at=?,started_at=NULL,last_error=?,updated_at=? WHERE id=? AND state='running'`, now, safeSummary(errors.New(summary)), now, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_locks WHERE job_id=(SELECT job_id FROM scheduled_runs WHERE id=?) AND owner=?`, runID, owner); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecoverStale(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := formatTime(s.clock.Now())
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_locks WHERE expires_at<=?`, now); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE scheduled_runs SET state=CASE WHEN EXISTS(SELECT 1 FROM scheduled_jobs j WHERE j.id=scheduled_runs.job_id AND j.cancelled_at IS NOT NULL) THEN 'cancelled' ELSE 'pending' END,available_at=?,finished_at=CASE WHEN EXISTS(SELECT 1 FROM scheduled_jobs j WHERE j.id=scheduled_runs.job_id AND j.cancelled_at IS NOT NULL) THEN ? ELSE NULL END,last_error='recovered after stale lock',updated_at=? WHERE state='running' AND NOT EXISTS(SELECT 1 FROM job_locks l WHERE l.job_id=scheduled_runs.job_id)`, now, now, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, tx.Commit()
}

func (s *Store) History(ctx context.Context, jobID string) ([]Run, error) {
	query := `SELECT r.id,r.scheduled_for,r.idempotency_key,r.state,r.attempts,r.available_at,r.started_at,r.finished_at,r.last_error,` + jobSelectColumnsWithPrefix("j") + ` FROM scheduled_runs r JOIN scheduled_jobs j ON j.id=r.job_id WHERE r.job_id=? ORDER BY r.id`
	rows, err := s.db.QueryContext(ctx, query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func NextRecurring(anchor time.Time, every time.Duration, now time.Time) time.Time {
	anchor = anchor.UTC()
	now = now.UTC()
	if every <= 0 || anchor.After(now) {
		return anchor
	}
	steps := now.Sub(anchor)/every + 1
	return anchor.Add(steps * every)
}

func runKey(jobID string, scheduled time.Time) string {
	return "job:" + jobID + ":" + formatTime(scheduled)
}

func boundedBackoff(base time.Duration, attempts int) time.Duration {
	if base <= 0 {
		return 0
	}
	shift := attempts - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 10 {
		shift = 10
	}
	delay := base * time.Duration(1<<shift)
	if delay > 24*time.Hour || delay < 0 {
		return 24 * time.Hour
	}
	return delay
}

func scanJob(scanner interface{ Scan(...any) error }) (Job, error) {
	var job Job
	var kind, class, risk, next, missed, cancelled, created, updated string
	var enabled int
	var intervalNS, backoffNS, timeoutNS int64
	var cancelledValue sql.NullString
	err := scanner.Scan(&job.ID, &job.SkillID, &job.Input, &kind, &class, &risk, &job.Timezone, &intervalNS, &next, &enabled, &missed, &job.MaxAttempts, &backoffNS, &timeoutNS, &job.CreationKey, &job.ApprovalRef, &cancelledValue, &created, &updated)
	if err != nil {
		return Job{}, err
	}
	job.Kind = Kind(kind)
	job.Class = Class(class)
	job.Risk = policy.Risk(risk)
	job.Every = time.Duration(intervalNS)
	job.Enabled = enabled == 1
	job.Missed = MissedPolicy(missed)
	job.RetryBackoff = time.Duration(backoffNS)
	job.Timeout = time.Duration(timeoutNS)
	job.NextRun, err = parseTime(next)
	if err != nil {
		return Job{}, err
	}
	job.CreatedAt, err = parseTime(created)
	if err != nil {
		return Job{}, err
	}
	job.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return Job{}, err
	}
	if cancelledValue.Valid {
		cancelled = cancelledValue.String
		t, parseErr := parseTime(cancelled)
		if parseErr != nil {
			return Job{}, parseErr
		}
		job.CancelledAt = &t
	}
	return job, nil
}

func scanRun(scanner interface{ Scan(...any) error }) (Run, error) {
	var run Run
	var scheduled, available string
	var started, finished sql.NullString
	var jobKind, jobClass, jobRisk, jobNext, jobMissed, jobCreated, jobUpdated string
	var jobEnabled int
	var jobInterval, jobBackoff, jobTimeout int64
	var jobCancelled sql.NullString
	err := scanner.Scan(&run.ID, &scheduled, &run.IdempotencyKey, &run.State, &run.Attempts, &available, &started, &finished, &run.LastError,
		&run.Job.ID, &run.Job.SkillID, &run.Job.Input, &jobKind, &jobClass, &jobRisk, &run.Job.Timezone, &jobInterval, &jobNext, &jobEnabled, &jobMissed, &run.Job.MaxAttempts, &jobBackoff, &jobTimeout, &run.Job.CreationKey, &run.Job.ApprovalRef, &jobCancelled, &jobCreated, &jobUpdated)
	if err != nil {
		return Run{}, err
	}
	run.ScheduledFor, err = parseTime(scheduled)
	if err != nil {
		return Run{}, err
	}
	run.AvailableAt, err = parseTime(available)
	if err != nil {
		return Run{}, err
	}
	if started.Valid {
		t, parseErr := parseTime(started.String)
		if parseErr != nil {
			return Run{}, parseErr
		}
		run.StartedAt = &t
	}
	if finished.Valid {
		t, parseErr := parseTime(finished.String)
		if parseErr != nil {
			return Run{}, parseErr
		}
		run.FinishedAt = &t
	}
	run.Job.Kind = Kind(jobKind)
	run.Job.Class = Class(jobClass)
	run.Job.Risk = policy.Risk(jobRisk)
	run.Job.Every = time.Duration(jobInterval)
	run.Job.Enabled = jobEnabled == 1
	run.Job.Missed = MissedPolicy(jobMissed)
	run.Job.RetryBackoff = time.Duration(jobBackoff)
	run.Job.Timeout = time.Duration(jobTimeout)
	run.Job.NextRun, err = parseTime(jobNext)
	if err != nil {
		return Run{}, err
	}
	run.Job.CreatedAt, err = parseTime(jobCreated)
	if err != nil {
		return Run{}, err
	}
	run.Job.UpdatedAt, err = parseTime(jobUpdated)
	if err != nil {
		return Run{}, err
	}
	if jobCancelled.Valid {
		t, parseErr := parseTime(jobCancelled.String)
		if parseErr != nil {
			return Run{}, parseErr
		}
		run.Job.CancelledAt = &t
	}
	return run, nil
}

func jobSelectColumnsWithPrefix(prefix string) string {
	parts := strings.Split(jobSelectColumns, ",")
	for i := range parts {
		parts[i] = prefix + "." + parts[i]
	}
	return strings.Join(parts, ",")
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func safeSummary(err error) string {
	if err == nil {
		return ""
	}
	value := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}
