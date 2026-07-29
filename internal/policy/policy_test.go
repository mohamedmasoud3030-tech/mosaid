package policy

import (
	"testing"
	"time"
)

func TestFailClosedAndApproval(t *testing.T) {
	x := Tool{Name: "x", Version: "1", Risk: High, Modes: []Mode{Write}, Approval: false, Timeout: time.Second, OutputLimit: 10}
	d := Evaluate(x, Read)
	if d.Allowed || d.NeedsApproval {
		t.Fatal(d)
	}
	d = Evaluate(x, Write)
	if d.Allowed || !d.NeedsApproval {
		t.Fatal(d)
	}
}
