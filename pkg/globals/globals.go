package globals

import (
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/scheduler"
)

var (
	broker jobs.Broker
	schder scheduler.Scheduler
)

// GetBroker returns the global job broker.
func GetBroker() jobs.Broker {
	if broker == nil {
		panic("Job system not initialized")
	}
	return broker
}

// GetScheduler returns the global job scheduler.
func GetScheduler() scheduler.Scheduler {
	if schder == nil {
		panic("Job system not initialized")
	}
	return schder
}

// Set will set the globales values.
func Set(b jobs.Broker, s scheduler.Scheduler) {
	broker = b
	schder = s
}
