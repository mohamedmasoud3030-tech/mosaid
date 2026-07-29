package tools

import (
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"time"
)

func RegisterWorkspace(r *Registry, w *Workspace, p *ProcessRunner) error {
	specs := []Registered{
		{policy.Tool{Name: "workspace.list", Version: "1", Risk: policy.Safe, Modes: []policy.Mode{policy.Read, policy.Write, policy.Admin}, Timeout: 5 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.Idempotent}, w.List},
		{policy.Tool{Name: "workspace.read", Version: "1", Risk: policy.Safe, Modes: []policy.Mode{policy.Read, policy.Write, policy.Admin}, Timeout: 5 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.Idempotent}, w.Read},
		{policy.Tool{Name: "workspace.search", Version: "1", Risk: policy.Low, Modes: []policy.Mode{policy.Read, policy.Write, policy.Admin}, Timeout: 10 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.Idempotent}, w.Search},
		{policy.Tool{Name: "workspace.write", Version: "1", Risk: policy.Medium, Modes: []policy.Mode{policy.Write, policy.Admin}, Approval: true, Timeout: 5 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.AtLeastOnce}, w.Write},
		{policy.Tool{Name: "workspace.patch", Version: "1", Risk: policy.Medium, Modes: []policy.Mode{policy.Write, policy.Admin}, Approval: true, Timeout: 5 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.AtLeastOnce}, w.Patch},
		{policy.Tool{Name: "workspace.mkdir", Version: "1", Risk: policy.Medium, Modes: []policy.Mode{policy.Write, policy.Admin}, Approval: true, Timeout: 5 * time.Second, OutputLimit: 4096, PathScope: []string{w.Root}, Idempotency: policy.Idempotent}, w.Mkdir},
		{policy.Tool{Name: "workspace.move_to_trash", Version: "1", Risk: policy.High, Modes: []policy.Mode{policy.Write, policy.Admin}, Approval: true, Timeout: 5 * time.Second, OutputLimit: 4096, PathScope: []string{w.Root}, Idempotency: policy.NonRepeatable}, w.Trash},
		{policy.Tool{Name: "process.run", Version: "1", Risk: policy.Medium, Modes: []policy.Mode{policy.Write, policy.Admin}, Approval: true, Timeout: 5 * time.Minute, OutputLimit: int64(p.MaxOutput), PathScope: []string{w.Root}, Idempotency: policy.AtLeastOnce}, p.Run}}
	for _, x := range specs {
		if err := r.Register(x); err != nil {
			return err
		}
	}
	return nil
}
