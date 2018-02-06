// +build windows

package exec

import "os/exec"

func createCmd(cmdStr, workDir string) *exec.Cmd {
	return exec.Command(cmdStr, workDir)
}

func killCmd(c *exec.Cmd) error {
	return c.Process.Kill()
}
