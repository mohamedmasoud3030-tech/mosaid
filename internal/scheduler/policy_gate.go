package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

type ApprovalBinding struct {
	Reference      string
	SkillID        string
	InputHash      string
	Class          Class
	ScheduledFor   time.Time
	IdempotencyKey string
}

type ApprovalVerifier interface {
	VerifyScheduled(context.Context, ApprovalBinding) error
}

type PolicyGate struct {
	Approvals ApprovalVerifier
}

func (g PolicyGate) Authorize(ctx context.Context, in Invocation) error {
	mode := policy.Read
	risk := in.Risk
	switch in.Class {
	case Reminder, ReadOnly:
		if risk == "" {
			risk = policy.Safe
		}
	case Write:
		mode = policy.Write
		if risk == "" {
			risk = policy.Medium
		}
	case Publish:
		mode = policy.Publish
		if risk == "" {
			risk = policy.High
		}
	default:
		return fmt.Errorf("%w: unknown job class", ErrPolicyDenied)
	}

	spec := policy.Tool{
		Name:        "skill." + in.SkillID,
		Version:     "1",
		Risk:        risk,
		Modes:       []policy.Mode{mode},
		Timeout:     time.Minute,
		OutputLimit: 1 << 20,
		Idempotency: policy.AtLeastOnce,
	}
	decision := policy.Evaluate(spec, mode)
	if decision.Allowed {
		return nil
	}
	if !decision.NeedsApproval {
		return fmt.Errorf("%w: %s", ErrPolicyDenied, decision.Reason)
	}
	if in.ApprovalRef == "" || g.Approvals == nil {
		return fmt.Errorf("%w: bound approval required", ErrPolicyDenied)
	}
	h := sha256.Sum256(in.Input)
	binding := ApprovalBinding{
		Reference:      in.ApprovalRef,
		SkillID:        in.SkillID,
		InputHash:      hex.EncodeToString(h[:]),
		Class:          in.Class,
		ScheduledFor:   in.ScheduledFor.UTC(),
		IdempotencyKey: in.IdempotencyKey,
	}
	if err := g.Approvals.VerifyScheduled(ctx, binding); err != nil {
		return fmt.Errorf("%w: approval verification failed", ErrPolicyDenied)
	}
	return nil
}
