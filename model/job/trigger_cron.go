package job

import (
	"time"

	"github.com/robfig/cron/v3"
)

// CronTrigger implements the @cron trigger type. It schedules recurring jobs with
// the weird but very used Cron syntax.
type CronTrigger struct {
	*TriggerInfos
	sched cron.Schedule
	done  chan struct{}
}

var parser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// NewCronTrigger returns a new instance of CronTrigger given the specified options.
func NewCronTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	schedule, err := parser.Parse(infos.Arguments)
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
	schedule, err := parser.Parse("@every " + infos.Arguments)
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
