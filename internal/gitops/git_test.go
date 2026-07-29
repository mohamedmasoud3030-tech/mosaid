package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func cmd(t *testing.T, dir string, a ...string) {
	t.Helper()
	c := exec.Command("git", a...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
func TestProtectedBranchAndTaskCommit(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	os.Mkdir(repo, 0700)
	cmd(t, repo, "init", "-b", "main")
	cmd(t, repo, "config", "user.email", "test@example.invalid")
	cmd(t, repo, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(repo, "a"), []byte("a"), 0600)
	cmd(t, repo, "add", "a")
	cmd(t, repo, "commit", "-m", "init")
	s := Service{Root: root, Timeout: 0, MaxOutput: 1 << 20}
	if _, e := s.Commit(context.Background(), "repo", "bad"); e == nil {
		t.Fatal("main write")
	}
	if e := s.CreateBranch(context.Background(), "repo", "agent/task"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(repo, "a"), []byte("b"), 0600)
	if e := s.Stage(context.Background(), "repo", "a"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Commit(context.Background(), "repo", "feat: task"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Diff(context.Background(), "repo", false); e != nil {
		t.Fatal(e)
	}
}
func TestCloneAllowlist(t *testing.T) {
	s := Service{Root: t.TempDir(), AllowedClonePrefixes: []string{"https://github.com/allowed/"}}
	if e := s.Clone(context.Background(), "https://evil.invalid/r.git", "x"); e == nil {
		t.Fatal("clone allowed")
	}
}
