package workers

import (
	"context"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/jobs"
)

func init() {
	jobs.AddWorker("log", &jobs.WorkerConfig{
		Concurrency:  1,
		MaxExecCount: 1,
		Timeout:      1 * time.Second,
		WorkerFunc:   LogWorker,
	})
}

// LogWorker is the worker that just logs its message (useful for debugging)
func LogWorker(ctx context.Context, m *jobs.Message) error {
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	log.Printf("[jobs] log %s: %s", domain, m.Data)
	return nil
}
