package stack

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/utils"
)

var (
	broker jobs.Broker
	schder scheduler.Scheduler
)

// Start is used to initialize all the
func Start() (utils.Shutdowner, error) {
	if config.IsDevRelease() {
		fmt.Println(`                           !! DEVELOPMENT RELEASE !!
You are running a development release which may deactivate some very important
security features. Please do not use this binary as your production server.
`)
	}

	// Init the main global connection to the swift server
	fsURL := config.FsURL()
	if fsURL.Scheme == config.SchemeSwift {
		if err := config.InitSwiftConnection(fsURL); err != nil {
			return nil, err
		}
	}

	return startJobSystem()
}

// startJobSystem starts the jobs and scheduler systems
func startJobSystem() (utils.Shutdowner, error) {
	cfg := config.GetConfig().Jobs
	nbWorkers := cfg.Workers
	if cli := cfg.Redis.Client(); cli != nil {
		broker = jobs.NewRedisBroker(nbWorkers, cli)
		schder = scheduler.NewRedisScheduler(cli)
	} else {
		broker = jobs.NewMemBroker(nbWorkers)
		schder = scheduler.NewMemScheduler()
	}
	if err := broker.Start(jobs.GetWorkersList()); err != nil {
		return nil, err
	}
	if err := schder.Start(broker); err != nil {
		return nil, err
	}
	return utils.NewGroupShutdown(broker, schder), nil
}

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
