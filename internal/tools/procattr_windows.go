//go:build windows

package tools

import (
	"os/exec"
	"syscall"
)

// isolateProcess gives the child its own console process group so Ctrl+C in the
// parent console does not reach it.
func isolateProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// terminateProcess stops the child process itself. Windows has no portable
// equivalent of killing a process group without spawning taskkill, and this
// runtime never uses a shell, so grandchildren started by an allowlisted tool
// are not guaranteed to stop. process.run is therefore opt-in and approval
// gated; see docs/agent-core/README.md.
func terminateProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
