package stack

import (
	"context"
	"fmt"
	"os"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/google/gops/agent"
)

var (
	broker jobs.Broker
	schder scheduler.Scheduler
)

var log = logger.WithNamespace("stack")

type gopAgent struct{}

func (g gopAgent) Shutdown(ctx context.Context) error {
	fmt.Print("  shutting down gops...")
	agent.Close()
	fmt.Println("ok.")
	return nil
}

// Start is used to initialize all the
func Start() (utils.Shutdowner, error) {
	if config.IsDevRelease() {
		fmt.Println(`                           !! DEVELOPMENT RELEASE !!
You are running a development release which may deactivate some very important
security features. Please do not use this binary as your production server.
`)
	}

	err := agent.Listen(agent.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error on gops agent: %s\n", err)
	}

	// Check that we can properly reach CouchDB.
	db, err := checkup.HTTPChecker{
		URL:         config.CouchURL().String(),
		MustContain: `"version":"2`,
	}.Check()
	if err != nil {
		return nil, fmt.Errorf("Could not reach Couchdb 2.0 database: %s", err.Error())
	}
	if db.Status() == checkup.Down {
		return nil, fmt.Errorf("Could not reach Couchdb 2.0 database:\n%s", db.String())
	}
	if db.Status() != checkup.Healthy {
		log.Warnf("CouchDB does not seem to be in a healthy state, "+
			"the cozy-stack will be starting anyway:\n%s", db.String())
	}

	// Init the main global connection to the swift server
	fsURL := config.FsURL()
	if fsURL.Scheme == config.SchemeSwift {
		if err := config.InitSwiftConnection(fsURL); err != nil {
			return nil, err
		}
	}

	jobsConfig := config.GetConfig().Jobs
	nbWorkers := jobsConfig.Workers
	if cli := jobsConfig.Redis.Client(); cli != nil {
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

	return utils.NewGroupShutdown(broker, schder, gopAgent{}), nil
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
