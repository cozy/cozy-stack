package stack

import (
	"errors"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsswift"
	"github.com/go-redis/redis"
)

var (
	broker jobs.Broker
	sched  scheduler.Scheduler
)

// Start is used to initialize all the
func Start() error {
	// Init the main global connection to the swift server
	fsURL := config.FsURL()
	if fsURL.Scheme == "swift" {
		if err := vfsswift.InitConnection(fsURL); err != nil {
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
	sched = scheduler.NewMemScheduler(scheduler.NewTriggerCouchStorage())
	return sched.Start(broker)
}

func startRedisJobSystem(nbWorkers int, opts *redis.Options) error {
	// client := redis.NewClient(opts)
	// TODO limit the number of workers to nbWorkers
	// TODO use a redis broker
	broker = jobs.NewMemBroker(jobs.GetWorkersList())
	// TODO use a redis scheduler
	sched = scheduler.NewMemScheduler(scheduler.NewTriggerCouchStorage())
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
