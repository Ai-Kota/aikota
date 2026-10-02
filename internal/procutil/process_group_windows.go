//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup configures cmd to run in a new process group (Windows).
func SetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}

// KillProcessGroup terminates the process (Windows limitation).
func KillProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}
