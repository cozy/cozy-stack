package hooks

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// ErrHookFailed is used when an hook script exits with a non-zero status
var ErrHookFailed = errors.New("Hook exited with non-zero status")

// Execute runs a pre-hook, then calls te function, and finally run the
// post-hook.
func Execute(name string, args []string, fn func() error) error {
	if err := runHook("pre", name, args); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return runHook("post", name, args)
}

func runHook(prefix, name string, args []string) error {
	dir := config.GetConfig().Hooks
	script := fmt.Sprintf("%s/%s-%s", dir, prefix, name)
	if !isExecutable(script) {
		return nil
	}
	log := logger.WithNamespace("hooks")
	log.Infof("Execute %s with %v", script, args)
	cmd := exec.Command(script, args...)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		log.Infof("Output: %s", out)
	}
	if err != nil {
		log.Errorf("Execution failed: %s", err)
		return ErrHookFailed
	}
	return nil
}

func isExecutable(script string) bool {
	stat, err := os.Stat(script)
	if err != nil {
		return false
	}
	return stat.Mode()&0100 > 0
}
