package jobs

import (
	"errors"
	"time"
)

// IntervalTrigger implements the @interval trigger. It triggers a job on a
// fixed period of time.
type IntervalTrigger struct {
	du   time.Duration
	in   *TriggerInfos
	done chan struct{}
}

// NewIntervalTrigger returns a new instace of IntervalTriven given the
// specified options.
func NewIntervalTrigger(infos *TriggerInfos) (*IntervalTrigger, error) {
	d, err := time.ParseDuration(infos.Arguments)
	if err != nil {
		return nil, err
	}
	if d < 1*time.Second {
		return nil, errors.New("Invalid interval duration")
	}
	return &IntervalTrigger{
		du:   d,
		in:   infos,
		done: make(chan struct{}),
	}, nil
}

// Type implements the Type method of the Trigger interface.
func (i *IntervalTrigger) Type() string {
	return "@interval"
}

// Schedule implements the Schedule method of the Trigger interface.
func (i *IntervalTrigger) Schedule() <-chan *JobRequest {
	ch := make(chan *JobRequest)
	go func() {
		for {
			select {
			case <-time.After(i.du):
				ch <- &JobRequest{
					WorkerType: i.in.WorkerType,
					Message:    i.in.Message,
					Options:    i.in.Options,
				}
			case <-i.done:
			}
		}
	}()
	return ch
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (i *IntervalTrigger) Unschedule() {
	close(i.done)
}

// Infos implements the Infos method of the Trigger interface.
func (i *IntervalTrigger) Infos() *TriggerInfos {
	return i.in
}

var _ Trigger = &IntervalTrigger{}
