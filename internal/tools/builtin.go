package tools

import (
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"time"
)

// RegisterWorkspace registers the sandboxed workspace tools. The process runner
// is optional: when it is nil, process.run is not registered and therefore can
// never be reached by a model. Process execution is opt-in because the Telegram
// gateway drains its inbox serially and a long child process would stall every
// other update on the phone.
func RegisterWorkspace(r *Registry, w *Workspace, p *ProcessRunner) error {
	readModes := []policy.Mode{policy.Read, policy.Write, policy.Admin}
	writeModes := []policy.Mode{policy.Write, policy.Admin}
	specs := []Registered{
		{
			Spec: policy.Tool{
				Name: "workspace.list", Version: "1", Risk: policy.Safe, Modes: readModes,
				Description: "List the direct entries of one directory inside the sandboxed workspace. Returns name, dir and size for each entry. Use it before reading files when you are unsure what exists.",
				InputSchema: schema(map[string]map[string]any{
					"path": stringProperty("Workspace-relative directory path. Use \".\" for the workspace root."),
				}),
				Timeout: 5 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.Idempotent,
			},
			Run: w.List,
		},
		{
			Spec: policy.Tool{
				Name: "workspace.read", Version: "1", Risk: policy.Safe, Modes: readModes,
				Description: "Read a UTF-8 text file from the workspace. Returns the file text, or a binary marker for non-text files. Paths are relative to the workspace root and may not escape it.",
				InputSchema: schema(map[string]map[string]any{
					"path": stringProperty("Workspace-relative file path, for example \"notes/todo.md\"."),
				}, "path"),
				Timeout: workspaceReadTimeout, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.Idempotent,
			},
			Run: w.Read,
		},
		{
			Spec: policy.Tool{
				Name: "workspace.search", Version: "1", Risk: policy.Low, Modes: readModes,
				Description: "Search the workspace for an exact substring and return up to 100 matching lines as path:line:text. Use it to find where something is instead of reading every file.",
				InputSchema: schema(map[string]map[string]any{
					"query": stringProperty("Exact substring to look for. Not a regular expression."),
					"path":  stringProperty("Workspace-relative directory to search. Use \".\" for the whole workspace."),
				}, "query"),
				Timeout: 10 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.Idempotent,
			},
			Run: w.Search,
		},
		{
			Spec: policy.Tool{
				Name: "workspace.write", Version: "1", Risk: policy.Medium, Modes: writeModes, Approval: true,
				Description: "Create or replace an entire text file in the workspace. This overwrites the file, so read it first and prefer workspace.patch for small edits. Requires owner approval.",
				InputSchema: schema(map[string]map[string]any{
					"path":    stringProperty("Workspace-relative file path to create or replace."),
					"content": stringProperty("Complete new file content."),
				}, "path", "content"),
				Timeout: 5 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.AtLeastOnce,
			},
			Run: w.Write,
		},
		{
			Spec: policy.Tool{
				Name: "workspace.patch", Version: "1", Risk: policy.Medium, Modes: writeModes, Approval: true,
				Description: "Replace one exact occurrence of a string in a workspace file. The old text must appear exactly once, otherwise nothing is written. Requires owner approval.",
				InputSchema: schema(map[string]map[string]any{
					"path": stringProperty("Workspace-relative file path to edit."),
					"old":  stringProperty("Exact existing text that must occur exactly once."),
					"new":  stringProperty("Replacement text. May be empty to delete the matched text."),
				}, "path", "old", "new"),
				Timeout: 5 * time.Second, OutputLimit: 1 << 20, PathScope: []string{w.Root}, Idempotency: policy.AtLeastOnce,
			},
			Run: w.Patch,
		},
		{
			Spec: policy.Tool{
				Name: "workspace.mkdir", Version: "1", Risk: policy.Medium, Modes: writeModes, Approval: true,
				Description: "Create a directory (and any missing parents) inside the workspace. Requires owner approval.",
				InputSchema: schema(map[string]map[string]any{
					"path": stringProperty("Workspace-relative directory path to create."),
				}, "path"),
				Timeout: 5 * time.Second, OutputLimit: 4096, PathScope: []string{w.Root}, Idempotency: policy.Idempotent,
			},
			Run: w.Mkdir,
		},
		{
			Spec: policy.Tool{
				Name: "workspace.move_to_trash", Version: "1", Risk: policy.High, Modes: writeModes, Approval: true,
				Description: "Move a workspace file into the workspace trash directory so it can be restored by hand. Requires owner approval and is never a permanent delete.",
				InputSchema: schema(map[string]map[string]any{
					"path": stringProperty("Workspace-relative path to move to trash."),
				}, "path"),
				Timeout: 5 * time.Second, OutputLimit: 4096, PathScope: []string{w.Root}, Idempotency: policy.NonRepeatable,
			},
			Run: w.Trash,
		},
	}
	if p != nil {
		specs = append(specs, Registered{
			Spec: policy.Tool{
				Name: "process.run", Version: "1", Risk: policy.Medium, Modes: writeModes, Approval: true,
				Description: "Run one allowlisted developer subcommand with a structured argv inside the workspace. No shell, no pipes, no redirection. Requires owner approval and blocks other updates while it runs.",
				InputSchema: schema(map[string]map[string]any{
					"profile": stringProperty("Registered command profile, for example \"go\" or \"git\"."),
					"argv":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Structured arguments; the first element must be an allowlisted subcommand."},
					"cwd":     stringProperty("Workspace-relative working directory. Defaults to the workspace root."),
				}, "profile", "argv"),
				Timeout: 5 * time.Minute, OutputLimit: int64(p.MaxOutput), PathScope: []string{w.Root}, Idempotency: policy.AtLeastOnce,
			},
			Run: p.Run,
		})
	}
	for _, x := range specs {
		if err := r.Register(x); err != nil {
			return err
		}
	}
	return nil
}
