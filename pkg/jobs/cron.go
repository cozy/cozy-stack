package jobs

import (
	"context"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/robfig/cron"
)

// CronSpec defines a cron process run by the stack, executing a worker with
// the given cron schedule specification.
type CronSpec struct {
	Activated      bool
	Schedule       string
	WorkerType     string
	WorkerTemplate func() (Message, error)
}

type cronner struct {
	spec     CronSpec
	schedule cron.Schedule
	sys      JobSystem
	stopped  chan struct{}
	finished chan struct{}
}

// CronJobs starts a list of cron specification jobs. It is used to define a
// serie for recurring jobs that the stack may have to perform as a cron-like
// daemon.
func CronJobs(specs []CronSpec) (utils.Shutdowner, error) {
	var s []utils.Shutdowner
	sys := System()
	for _, spec := range specs {
		if !spec.Activated {
			continue
		}
		schedule, err := cron.Parse(strings.TrimPrefix(spec.Schedule, "@cron"))
		if err != nil {
			return nil, err
		}
		s = append(s, cronJob(spec, schedule, sys))
	}
	return utils.NewGroupShutdown(s...), nil
}

func cronJob(spec CronSpec, schedule cron.Schedule, sys JobSystem) *cronner {
	c := &cronner{
		spec:     spec,
		schedule: schedule,
		sys:      sys,
		stopped:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	go c.run()
	return c
}

func (c *cronner) run() {
	next := time.Now()
	defer func() {
		c.finished <- struct{}{}
	}()
	for {
		next = c.schedule.Next(next)
		select {
		case <-time.After(-time.Since(next)):
			msg, err := c.spec.WorkerTemplate()
			if err != nil {
				joblog.Errorf("cron: could not generate job template %q: %s",
					c.spec.WorkerType, err)
				continue
			}
			_, err = c.sys.PushJob(prefixer.GlobalPrefixer, &JobRequest{
				WorkerType: c.spec.WorkerType,
				Message:    msg,
			})
			if err != nil {
				joblog.Errorf("cron: could not push a new job %q: %s",
					c.spec.WorkerType, err)
			}
		case <-c.stopped:
			return
		}
	}
}

func (c *cronner) Shutdown(ctx context.Context) error {
	c.stopped <- struct{}{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.finished:
		return nil
	}
}
