package log

import (
	"context"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
)

func init() {
	jobs.AddWorker("log", &jobs.WorkerConfig{
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Timeout:      1 * time.Second,
		WorkerFunc:   Worker,
	})
}

// Worker is the worker that just logs its message (useful for debugging)
func Worker(ctx context.Context, m jobs.Message) error {
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	logger.WithDomain(domain).Infof(string(m))
	return nil
}
