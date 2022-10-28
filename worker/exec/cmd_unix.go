//go:build !windows
// +build !windows

package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// CreateCmd creates an exec.Cmd.
func CreateCmd(cmdStr, workDir string) *exec.Cmd {
	cwd := workDir
	if info, err := os.Stat(workDir); err == nil && !info.IsDir() {
		cwd = filepath.Dir(workDir)
	}
	c := exec.Command(cmdStr, workDir)
	c.Dir = cwd
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

// KillCmd sends a KILL signal to the command.
func KillCmd(c *exec.Cmd) error {
	return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
}
