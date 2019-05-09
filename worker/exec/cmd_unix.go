// +build !windows

package exec

import (
	"os/exec"
	"syscall"
)

// CreateCmd creates an exec.Cmd.
func CreateCmd(cmdStr, workDir string) *exec.Cmd {
	c := exec.Command(cmdStr, workDir)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

// KillCmd sends a KILL signal to the command.
func KillCmd(c *exec.Cmd) error {
	return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
}
