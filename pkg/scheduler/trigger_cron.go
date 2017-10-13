package scheduler

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/robfig/cron"
)

// CronTrigger implements the @cron trigger type. It schedules recurring jobs with
// the weird but very used Cron syntax.
type CronTrigger struct {
	sched cron.Schedule
	infos *TriggerInfos
	done  chan struct{}
}

// NewCronTrigger returns a new instance of CronTrigger given the specified options.
func NewCronTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	schedule, err := cron.Parse(infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	return &CronTrigger{
		sched: schedule,
		infos: infos,
		done:  make(chan struct{}),
	}, nil
}

// NewEveryTrigger returns an new instance of CronTrigger given the specified
// options as @every.
func NewEveryTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	schedule, err := cron.Parse("@every " + infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	return &CronTrigger{
		sched: schedule,
		infos: infos,
		done:  make(chan struct{}),
	}, nil
}

// Type implements the Type method of the Trigger interface.
func (c *CronTrigger) Type() string {
	return c.infos.Type
}

// DocType implements the permissions.Validable interface
func (c *CronTrigger) DocType() string {
	return consts.Triggers
}

// ID implements the permissions.Validable interface
func (c *CronTrigger) ID() string {
	return c.infos.TID
}

// Valid implements the permissions.Validable interface
func (c *CronTrigger) Valid(key, value string) bool {
	switch key {
	case jobs.WorkerType:
		return c.infos.WorkerType == value
	}
	return false
}

// NextExecution returns the next time when a job should be fired for this trigger
func (c *CronTrigger) NextExecution(last time.Time) time.Time {
	return c.sched.Next(last)
}

// Schedule implements the Schedule method of the Trigger interface.
func (c *CronTrigger) Schedule() <-chan *jobs.JobRequest {
	ch := make(chan *jobs.JobRequest)
	go func() {
		next := time.Now()
		for {
			next = c.NextExecution(next)
			select {
			case <-time.After(-time.Since(next)):
				ch <- c.infos.JobRequest()
			case <-c.done:
				close(ch)
				return
			}
		}
	}()
	return ch
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (c *CronTrigger) Unschedule() {
	close(c.done)
}

// Infos implements the Infos method of the Trigger interface.
func (c *CronTrigger) Infos() *TriggerInfos {
	return c.infos
}

var _ Trigger = &CronTrigger{}
