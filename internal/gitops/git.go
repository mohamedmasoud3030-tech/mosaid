package gitops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	Root                 string
	AllowedClonePrefixes []string
	Timeout              time.Duration
	MaxOutput            int
}

func (s Service) repo(path string) (string, error) {
	p := filepath.Join(s.Root, filepath.Clean(path))
	r, err := filepath.Rel(s.Root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return "", errors.New("repo escape")
	}
	return p, nil
}
func (s Service) run(ctx context.Context, repo string, args ...string) (string, error) {
	if s.Timeout == 0 {
		s.Timeout = 2 * time.Minute
	}
	c, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	cmd := exec.CommandContext(c, "git", args...)
	cmd.Dir = repo
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(s.Root, ".git-home"), "GIT_TERMINAL_PROMPT=0", "LANG=C.UTF-8"}
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	err := cmd.Run()
	if b.Len() > s.MaxOutput && s.MaxOutput > 0 {
		return "", errors.New("git output too large")
	}
	if err != nil {
		return "", fmt.Errorf("git failed: %s", strings.TrimSpace(b.String()))
	}
	return strings.TrimSpace(b.String()), nil
}
func (s Service) Clone(ctx context.Context, remote, dest string) error {
	u, err := url.Parse(remote)
	if err != nil || u.Scheme != "https" {
		return errors.New("HTTPS clone required")
	}
	ok := false
	for _, p := range s.AllowedClonePrefixes {
		if strings.HasPrefix(remote, p) {
			ok = true
		}
	}
	if !ok {
		return errors.New("clone source denied")
	}
	d, err := s.repo(dest)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(d), 0700); err != nil {
		return err
	}
	_, err = s.run(ctx, s.Root, "clone", "--", remote, d)
	return err
}
func (s Service) CurrentBranch(ctx context.Context, repo string) (string, error) {
	p, e := s.repo(repo)
	if e != nil {
		return "", e
	}
	return s.run(ctx, p, "branch", "--show-current")
}
func (s Service) ensureTask(ctx context.Context, repo string) error {
	b, e := s.CurrentBranch(ctx, repo)
	if e != nil {
		return e
	}
	if b == "main" || b == "master" || b == "" {
		return errors.New("write on protected branch denied")
	}
	return nil
}
func (s Service) Status(ctx context.Context, repo string) (string, error) {
	p, e := s.repo(repo)
	if e != nil {
		return "", e
	}
	return s.run(ctx, p, "status", "--short", "--branch")
}
func (s Service) Diff(ctx context.Context, repo string, staged bool) (string, error) {
	p, e := s.repo(repo)
	if e != nil {
		return "", e
	}
	a := []string{"diff", "--no-ext-diff"}
	if staged {
		a = append(a, "--cached")
	}
	return s.run(ctx, p, a...)
}
func (s Service) Log(ctx context.Context, repo string) (string, error) {
	p, e := s.repo(repo)
	if e != nil {
		return "", e
	}
	return s.run(ctx, p, "log", "--oneline", "--decorate", "-n", "50")
}
func (s Service) CreateBranch(ctx context.Context, repo, branch string) error {
	if !strings.HasPrefix(branch, "agent/") {
		return errors.New("branch must start agent/")
	}
	p, e := s.repo(repo)
	if e != nil {
		return e
	}
	_, e = s.run(ctx, p, "switch", "-c", branch)
	return e
}
func (s Service) Worktree(ctx context.Context, repo, branch, dest string) error {
	if !strings.HasPrefix(branch, "agent/") {
		return errors.New("branch denied")
	}
	p, e := s.repo(repo)
	if e != nil {
		return e
	}
	d, e := s.repo(dest)
	if e != nil {
		return e
	}
	_, e = s.run(ctx, p, "worktree", "add", "-b", branch, d)
	return e
}
func (s Service) Stage(ctx context.Context, repo string, paths ...string) error {
	if e := s.ensureTask(ctx, repo); e != nil {
		return e
	}
	p, _ := s.repo(repo)
	a := []string{"add", "--"}
	for _, x := range paths {
		if filepath.IsAbs(x) || strings.HasPrefix(filepath.Clean(x), "..") {
			return errors.New("stage path denied")
		}
		a = append(a, x)
	}
	_, e := s.run(ctx, p, a...)
	return e
}
func (s Service) Commit(ctx context.Context, repo, msg string) (string, error) {
	if e := s.ensureTask(ctx, repo); e != nil {
		return "", e
	}
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("message required")
	}
	p, _ := s.repo(repo)
	return s.run(ctx, p, "commit", "-m", msg)
}
func (s Service) Push(ctx context.Context, repo string) (string, error) {
	if e := s.ensureTask(ctx, repo); e != nil {
		return "", e
	}
	p, _ := s.repo(repo)
	b, e := s.CurrentBranch(ctx, repo)
	if e != nil {
		return "", e
	}
	return s.run(ctx, p, "push", "--set-upstream", "origin", b)
}
