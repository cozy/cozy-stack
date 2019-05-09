package job

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/robfig/cron"
)

// CronTrigger implements the @cron trigger type. It schedules recurring jobs with
// the weird but very used Cron syntax.
type CronTrigger struct {
	*TriggerInfos
	sched cron.Schedule
	done  chan struct{}
}

// NewCronTrigger returns a new instance of CronTrigger given the specified options.
func NewCronTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	schedule, err := cron.Parse(infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	return &CronTrigger{
		TriggerInfos: infos,
		sched:        schedule,
		done:         make(chan struct{}),
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
		TriggerInfos: infos,
		sched:        schedule,
		done:         make(chan struct{}),
	}, nil
}

// Type implements the Type method of the Trigger interface.
func (c *CronTrigger) Type() string {
	return c.TriggerInfos.Type
}

// DocType implements the permissions.Matcher interface
func (c *CronTrigger) DocType() string {
	return consts.Triggers
}

// ID implements the permissions.Matcher interface
func (c *CronTrigger) ID() string {
	return c.TriggerInfos.TID
}

// Match implements the permissions.Matcher interface
func (c *CronTrigger) Match(key, value string) bool {
	switch key {
	case WorkerType:
		return c.TriggerInfos.WorkerType == value
	}
	return false
}

// NextExecution returns the next time when a job should be fired for this trigger
func (c *CronTrigger) NextExecution(last time.Time) time.Time {
	return c.sched.Next(last)
}

// Schedule implements the Schedule method of the Trigger interface.
func (c *CronTrigger) Schedule() <-chan *JobRequest {
	ch := make(chan *JobRequest)
	go func() {
		next := time.Now()
		for {
			next = c.NextExecution(next)
			select {
			case <-time.After(-time.Since(next)):
				ch <- c.TriggerInfos.JobRequest()
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
	return c.TriggerInfos
}

var _ Trigger = &CronTrigger{}
