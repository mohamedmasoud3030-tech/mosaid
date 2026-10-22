//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

// isolateProcess puts the child in its own process group so a timeout can kill
// the whole tree without touching Mosaid itself.
func isolateProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateProcess kills the child's process group.
func terminateProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
