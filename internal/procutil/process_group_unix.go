//go:build !windows

package procutil

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup configures cmd to run in a new process group (Unix).
func SetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillProcessGroup terminates the entire process group led by cmd.
func KillProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process != nil {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
			return cmd.Process.Kill()
		}
	}
	return nil
}
