//go:build !windows

package security

import (
	"os/exec"
	"syscall"
)

// prepareShellCmdSession makes the shell child process run in an independent session so that on timeout/cancel the entire group can be SIGKILL-ed (including child processes).
func prepareShellCmdSession(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	return nil
}

// terminateProcessGroup sends SIGKILL to the process group of rootPID; falls back to cmd.Process.Pid when rootPID is 0.
func terminateProcessGroup(rootPID int, cmd *exec.Cmd) {
	pid := rootPID
	if pid <= 0 && cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

// terminateCmdTree makes a best effort to terminate cmd and its process group (on Unix, PGID == first process PID after Setsid).
func terminateCmdTree(cmd *exec.Cmd) {
	terminateProcessGroup(0, cmd)
}

// stopProcessGroup gives the whole job a grace period to release resources.
func stopProcessGroup(pid int, cmd *exec.Cmd) {
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
}

func processGroupExists(pid int) bool {
	return pid > 0 && syscall.Kill(-pid, 0) != syscall.ESRCH
}
