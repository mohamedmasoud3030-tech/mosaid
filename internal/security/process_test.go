package security

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("current process reported dead")
	}
	for _, pid := range []int{0, -1} {
		if processAlive(pid) {
			t.Fatalf("pid %d reported alive", pid)
		}
	}
	// A process that has exited must be reported dead, which is what makes the
	// stale-lock takeover safe after a crash or reboot.
	cmd := exec.Command(os.Args[0], "-test.run=TestProcessAliveHelper")
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	if processAlive(cmd.Process.Pid) {
		t.Fatalf("exited pid %d reported alive", cmd.Process.Pid)
	}
}

func TestProcessAliveHelper(t *testing.T) {
	if os.Getenv("MOSAID_PROCESS_HELPER") != "1" {
		return
	}
	os.Exit(0)
}

func TestStaleLockIsTakenOver(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestProcessAliveHelper")
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	lockDir := dir + string(os.PathSeparator) + "runtime.lock"
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockDir+string(os.PathSeparator)+"pid", []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(dir)
	if err != nil {
		t.Fatalf("stale lock not taken over: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	// A live pid must still block a second instance.
	live := exec.Command(os.Args[0], "-test.run=TestProcessAliveHelper")
	if err := live.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = live.Wait() }()
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockDir+string(os.PathSeparator)+"pid", []byte(strconv.Itoa(live.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(dir); err == nil {
		t.Fatal("live pid did not block a second instance")
	}
}
