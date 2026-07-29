package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

var (
	ErrDuplicateCreationKey = errors.New("scheduler creation idempotency key already exists")
	ErrInvalidJob           = errors.New("invalid scheduled job")
	ErrPolicyDenied         = errors.New("scheduled invocation denied by policy")
	ErrJobCancelled         = errors.New("scheduled job cancelled")
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type Kind string

const (
	OneTime   Kind = "one_time"
	Recurring Kind = "recurring"
)

type Class string

const (
	Reminder Class = "reminder"
	ReadOnly Class = "read"
	Write    Class = "write"
	Publish  Class = "publish"
)

type MissedPolicy string

const (
	Skip    MissedPolicy = "skip"
	RunOnce MissedPolicy = "run_once"
)

type Job struct {
	ID           string
	SkillID      string
	Input        []byte
	Kind         Kind
	Class        Class
	Risk         policy.Risk
	Timezone     string
	Every        time.Duration
	NextRun      time.Time
	Enabled      bool
	Missed       MissedPolicy
	MaxAttempts  int
	RetryBackoff time.Duration
	Timeout      time.Duration
	CreationKey  string
	ApprovalRef  string
	CancelledAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Run struct {
	ID             int64
	Job            Job
	ScheduledFor   time.Time
	IdempotencyKey string
	State          string
	Attempts       int
	AvailableAt    time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	LastError      string
}

type Invocation struct {
	RunID          int64
	JobID          string
	SkillID        string
	Input          []byte
	Class          Class
	Risk           policy.Risk
	ScheduledFor   time.Time
	IdempotencyKey string
	ApprovalRef    string
}

type Executor interface {
	Execute(context.Context, Invocation) error
}

type Authorizer interface {
	Authorize(context.Context, Invocation) error
}

type ExecutorFunc func(context.Context, Invocation) error

func (f ExecutorFunc) Execute(ctx context.Context, in Invocation) error { return f(ctx, in) }

type AuthorizerFunc func(context.Context, Invocation) error

func (f AuthorizerFunc) Authorize(ctx context.Context, in Invocation) error { return f(ctx, in) }
