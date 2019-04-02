// +build windows

package exec

import "os/exec"

func CreateCmd(cmdStr, workDir string) *exec.Cmd {
	return exec.Command(cmdStr, workDir)
}

func KillCmd(c *exec.Cmd) error {
	return c.Process.Kill()
}
