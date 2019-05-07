package log

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/jobs"
)

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "log",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Timeout:      1 * time.Second,
		WorkerFunc:   Worker,
	})
}

// Worker is the worker that just logs its message (useful for debugging)
func Worker(ctx *jobs.WorkerContext) error {
	var msg string
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	ctx.Logger().Info(msg)
	return nil
}
