package job

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/utils"
)

// JobSystem is a pair of broker, scheduler linked together.
type JobSystem interface {
	Broker
	Scheduler
	utils.Shutdowner
}

type jobSystem struct {
	Broker
	Scheduler
}

// Shutdown shuts down the job system. Implement the utils.Shutdowner
// interface.
func (j jobSystem) Shutdown(ctx context.Context) error {
	if err := j.Broker.ShutdownWorkers(ctx); err != nil {
		return err
	}
	return j.Scheduler.ShutdownScheduler(ctx)
}

var globalJobSystem JobSystem

// SystemStart initializes and starts the global jobs system with the given
// broker, scheduler instances and workers list.
func SystemStart(b Broker, s Scheduler, workersList WorkersList) error {
	if globalJobSystem != nil {
		panic("Job system already started")
	}
	globalJobSystem = jobSystem{b, s}
	if err := b.StartWorkers(workersList); err != nil {
		return err
	}
	return s.StartScheduler(b)
}

// System returns the global job system.
func System() JobSystem {
	if globalJobSystem == nil {
		panic("Job system not initialized")
	}
	return globalJobSystem
}
