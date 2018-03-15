package jobs

import "context"

// JobSystem is a pair of broker, scheduler linked together.
type JobSystem struct {
	Broker
	Scheduler
}

// Shutdown shuts down the job system. Implement the utils.Shutdowner
// interface.
func (j *JobSystem) Shutdown(ctx context.Context) error {
	if err := j.Broker.Shutdown(ctx); err != nil {
		return err
	}
	if err := j.Scheduler.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}

var globalJobSystem *JobSystem

// SystemStart initializes and starts the global jobs system with the given
// broker, scheduler instances and workers list.
func SystemStart(b Broker, s Scheduler, workersList WorkersList) error {
	globalJobSystem = &JobSystem{b, s}
	if err := b.StartWorkers(workersList); err != nil {
		return err
	}
	if err := s.StartScheduler(b); err != nil {
		return err
	}
	return nil
}

// System returns the global job system.
func System() *JobSystem {
	if globalJobSystem == nil {
		panic("Job system not initialized")
	}
	return globalJobSystem
}
