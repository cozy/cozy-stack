package stack

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/go-redis/redis"
)

var (
	broker jobs.Broker
	sched  scheduler.Scheduler
)

// Start is used to initialize all the
func Start() error {
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
			return err
		}
	}
	return startJobSystem()
}

// startJobSystem starts the jobs and scheduler systems
func startJobSystem() error {
	cfg := config.GetConfig().Jobs
	if cfg.URL == "" || strings.HasPrefix(cfg.URL, "mem") {
		return startMemJobSystem(cfg.Workers)
	}
	if strings.HasPrefix(cfg.URL, "redis") {
		opts, err := redis.ParseURL(cfg.URL)
		if err != nil {
			return err
		}
		return startRedisJobSystem(cfg.Workers, opts)
	}
	return errors.New("Invalid jobs URL")
}

func startMemJobSystem(nbWorkers int) error {
	// TODO limit the number of workers to nbWorkers
	broker = jobs.NewMemBroker(jobs.GetWorkersList())
	sched = scheduler.NewMemScheduler()
	return sched.Start(broker)
}

func startRedisJobSystem(nbWorkers int, opts *redis.Options) error {
	client := redis.NewClient(opts)
	// TODO limit the number of workers to nbWorkers
	broker = jobs.NewRedisBroker(client)
	sched = scheduler.NewRedisScheduler(client)
	return sched.Start(broker)
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
	if sched == nil {
		panic("Job system not initialized")
	}
	return sched
}
