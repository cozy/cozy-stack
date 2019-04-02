// +build !windows

package exec

import (
	"os/exec"
	"syscall"
)

func CreateCmd(cmdStr, workDir string) *exec.Cmd {
	c := exec.Command(cmdStr, workDir)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

func KillCmd(c *exec.Cmd) error {
	return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
}
