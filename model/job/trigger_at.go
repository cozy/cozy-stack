package job

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
)

// maxPastTriggerTime is the maximum duration in the past for which the at
// triggers are executed immediately instead of discarded.
var maxPastTriggerTime = 24 * time.Hour

// AtTrigger implements the @at trigger type. It schedules a job at a specified
// time in the future.
type AtTrigger struct {
	*TriggerInfos
	at   time.Time
	done chan struct{}
}

// NewAtTrigger returns a new instance of AtTrigger given the specified
// options.
func NewAtTrigger(infos *TriggerInfos) (*AtTrigger, error) {
	at, err := time.Parse(time.RFC3339, infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	return &AtTrigger{
		TriggerInfos: infos,
		at:           at,
		done:         make(chan struct{}),
	}, nil
}

// NewInTrigger returns a new instance of AtTrigger given the specified
// options as @in.
func NewInTrigger(infos *TriggerInfos) (*AtTrigger, error) {
	d, err := time.ParseDuration(infos.Arguments)
	if err != nil {
		return nil, ErrMalformedTrigger
	}
	at := time.Now().Add(d)
	return &AtTrigger{
		TriggerInfos: infos,
		at:           at,
		done:         make(chan struct{}),
	}, nil
}

// Type implements the Type method of the Trigger interface.
func (a *AtTrigger) Type() string {
	return a.TriggerInfos.Type
}

// DocType implements the permissions.Matcher interface
func (a *AtTrigger) DocType() string {
	return consts.Triggers
}

// ID implements the permissions.Matcher interface
func (a *AtTrigger) ID() string {
	return a.TriggerInfos.TID
}

// Match implements the permissions.Matcher interface
func (a *AtTrigger) Match(key, value string) bool {
	switch key {
	case WorkerType:
		return a.TriggerInfos.WorkerType == value
	}
	return false
}

// Schedule implements the Schedule method of the Trigger interface.
func (a *AtTrigger) Schedule() <-chan *JobRequest {
	ch := make(chan *JobRequest)
	go func() {
		duration := -time.Since(a.at)
		if duration < 0 {
			if duration > -maxPastTriggerTime {
				ch <- a.TriggerInfos.JobRequest()
			}
			close(ch)
			return
		}
		select {
		case <-time.After(duration):
			ch <- a.TriggerInfos.JobRequest()
		case <-a.done:
		}
		close(ch)
	}()
	return ch
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (a *AtTrigger) Unschedule() {
	close(a.done)
}

// Infos implements the Infos method of the Trigger interface.
func (a *AtTrigger) Infos() *TriggerInfos {
	return a.TriggerInfos
}

var _ Trigger = &AtTrigger{}
