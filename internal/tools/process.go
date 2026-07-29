package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type CommandProfile struct {
	Executable   string
	AllowedFirst map[string]bool
	RiskNetwork  map[string]bool
}
type ProcessRunner struct {
	Workspace string
	Profiles  map[string]CommandProfile
	MaxOutput int
}
type ProcessResult struct {
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
}

func DefaultProfiles() map[string]CommandProfile {
	return map[string]CommandProfile{
		"go":   {Executable: "go", AllowedFirst: allow("test", "build", "vet", "fmt", "version", "list")},
		"git":  {Executable: "git", AllowedFirst: allow("status", "diff", "log", "show", "branch", "worktree", "fetch", "commit", "push")},
		"node": {Executable: "node", AllowedFirst: allow("--version")}, "npm": {Executable: "npm", AllowedFirst: allow("test", "run", "--version")}, "pnpm": {Executable: "pnpm", AllowedFirst: allow("test", "run", "--version")},
		"python": {Executable: "python", AllowedFirst: allow("--version", "-m")}, "pytest": {Executable: "pytest", AllowedFirst: allow("--version", "-q")}, "cargo": {Executable: "cargo", AllowedFirst: allow("test", "build", "check", "fmt", "--version")}}
}
func allow(x ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range x {
		m[v] = true
	}
	return m
}
func (p *ProcessRunner) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var a struct {
		Profile string   `json:"profile"`
		Argv    []string `json:"argv"`
		CWD     string   `json:"cwd"`
	}
	if json.Unmarshal(args, &a) != nil {
		return nil, errors.New("bad args")
	}
	prof, ok := p.Profiles[a.Profile]
	if !ok {
		return nil, errors.New("profile denied")
	}
	if len(a.Argv) == 0 || !prof.AllowedFirst[a.Argv[0]] {
		return nil, errors.New("subcommand denied")
	}
	for _, v := range a.Argv {
		if strings.ContainsAny(v, "\x00\n\r") {
			return nil, errors.New("invalid argument")
		}
	}
	root, err := filepath.Abs(p.Workspace)
	if err != nil {
		return nil, err
	}
	cwd := root
	if a.CWD != "" {
		cwd = filepath.Join(root, filepath.Clean(a.CWD))
	}
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil, errors.New("cwd escape")
	}
	cmd := exec.CommandContext(ctx, prof.Executable, a.Argv...)
	cmd.Dir = cwd
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(root, ".mosaid-home"), "TMPDIR=" + filepath.Join(root, ".mosaid-tmp"), "LANG=C.UTF-8"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	buf := &limitedBuffer{max: p.MaxOutput}
	cmd.Stdout = buf
	cmd.Stderr = buf
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = <-done
	}
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			return nil, err
		}
	}
	return ProcessResult{code, buf.String(), buf.truncated}, nil
}

type limitedBuffer struct {
	b         strings.Builder
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.max - b.b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			b.b.Write(p[:remaining])
			b.truncated = true
		} else {
			b.b.Write(p)
		}
	} else {
		b.truncated = true
	}
	return n, nil
}
func (b *limitedBuffer) String() string { return b.b.String() }

var _ = time.Second
