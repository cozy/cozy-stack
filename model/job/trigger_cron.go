package job

import (
	"fmt"
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

var (
	cronParser     = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	periodicParser = NewPeriodicParser()
)

// NewCronTrigger returns a new instance of CronTrigger given the specified options.
func NewCronTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	schedule, err := cronParser.Parse(infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	return &CronTrigger{
		TriggerInfos: infos,
		sched:        schedule,
		done:         make(chan struct{}),
	}, nil
}

// NewEveryTrigger returns a new instance of CronTrigger given the specified
// options as @every.
func NewEveryTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	schedule, err := cronParser.Parse("@every " + infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	return &CronTrigger{
		TriggerInfos: infos,
		sched:        schedule,
		done:         make(chan struct{}),
	}, nil
}

// NewMonthlyTrigger returns a new instance of CronTrigger given the specified
// options as @monthly. It will take a random day/hour in the possible range to
// spread the triggers from the same app manifest.
func NewMonthlyTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	return newPeriodicTrigger(infos, MonthlyKind)
}

// NewWeeklyTrigger returns a new instance of CronTrigger given the specified
// options as @weekly. It will take a random day/hour in the possible range to
// spread the triggers from the same app manifest.
func NewWeeklyTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	return newPeriodicTrigger(infos, WeeklyKind)
}

// NewDailyTrigger returns a new instance of CronTrigger given the specified
// options as @daily. It will take a random hour in the possible range to
// spread the triggers from the same app manifest.
func NewDailyTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	return newPeriodicTrigger(infos, DailyKind)
}

// NewHourlyTrigger returns a new instance of CronTrigger given the specified
// options as @hourly. It will take a random minute in the possible range to
// spread the triggers from the same app manifest.
func NewHourlyTrigger(infos *TriggerInfos) (*CronTrigger, error) {
	return newPeriodicTrigger(infos, HourlyKind)
}

func newPeriodicTrigger(infos *TriggerInfos, frequency FrequencyKind) (*CronTrigger, error) {
	spec, err := periodicParser.Parse(frequency, infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	seed := fmt.Sprintf("%s/%s/%v", infos.Domain, infos.WorkerType, infos.Message)
	crontab := spec.ToRandomCrontab(seed)
	schedule, err := cronParser.Parse(crontab)
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

// CombineRequest implements the CombineRequest method of the Trigger interface.
func (c *CronTrigger) CombineRequest() string {
	return keepOriginalRequest
}

var _ Trigger = &CronTrigger{}
