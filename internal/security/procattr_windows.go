//go:build windows

package security

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func prepareShellCmdSession(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}
	// Independent process group so that taskkill /T can terminate the entire child process tree.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = syscall.CREATE_NEW_PROCESS_GROUP
	return nil
}

// terminateProcessGroup terminates a process and its children using taskkill /F /T; falls back to cmd.Process.Pid when rootPID is 0.
func terminateProcessGroup(rootPID int, cmd *exec.Cmd) {
	pid := rootPID
	if pid <= 0 && cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	if pid <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tk := exec.CommandContext(ctx, "taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	if err := tk.Run(); err != nil {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

// terminateCmdTree terminates a process and its children using taskkill /F /T (Process.Kill on Windows cannot guarantee killing grandchild processes such as python).
func terminateCmdTree(cmd *exec.Cmd) {
	terminateProcessGroup(0, cmd)
}

func stopProcessGroup(pid int, cmd *exec.Cmd) {
	// Windows has no portable SIGTERM equivalent for arbitrary console jobs.
	terminateProcessGroup(pid, cmd)
}

// Windows taskkill /T is best effort; unlike a Unix PGID it has no persistent
// group handle to query after the root exits. Job Objects are needed for that.
func processGroupExists(pid int) bool { return false }
