package policy

import (
	"errors"
	"time"
)

type Mode string

const (
	Read    Mode = "read"
	Write   Mode = "write"
	Publish Mode = "publish"
	Admin   Mode = "admin"
)

type Risk string

const (
	Safe     Risk = "safe"
	Low      Risk = "low"
	Medium   Risk = "medium"
	High     Risk = "high"
	Critical Risk = "critical"
)

type Idempotency string

const (
	Idempotent    Idempotency = "idempotent"
	AtLeastOnce   Idempotency = "at_least_once"
	NonRepeatable Idempotency = "non_repeatable"
)

type Tool struct {
	Name, Version           string
	InputSchema             []byte
	Risk                    Risk
	Modes                   []Mode
	Approval                bool
	Timeout                 time.Duration
	OutputLimit             int64
	PathScope, NetworkScope []string
	Idempotency             Idempotency
}
type Decision struct {
	Allowed, NeedsApproval bool
	Reason                 string
}

func Evaluate(t Tool, m Mode) Decision {
	ok := false
	for _, x := range t.Modes {
		if x == m {
			ok = true
		}
	}
	if !ok {
		return Decision{Reason: "mode denied"}
	}
	if t.Name == "" || t.Version == "" || t.Timeout <= 0 || t.OutputLimit <= 0 {
		return Decision{Reason: "invalid tool declaration"}
	}
	need := t.Approval || t.Risk == High || t.Risk == Critical || m == Publish
	return Decision{Allowed: !need, NeedsApproval: need, Reason: "policy"}
}
func Validate(t Tool) error {
	if t.Name == "" || t.Version == "" {
		return errors.New("name/version required")
	}
	if len(t.Modes) == 0 {
		return errors.New("modes required")
	}
	if t.Timeout <= 0 || t.OutputLimit <= 0 {
		return errors.New("limits required")
	}
	return nil
}
