package jobs

import "time"

// AtTrigger implements the @at trigger type. It schedules a job at a specified
// time in the future.
type AtTrigger struct {
	typ  string
	at   time.Time
	in   *TriggerInfos
	done chan struct{}
}

// NewAtTrigger returns a new instance of AtTrigger given the specified
// options.
func NewAtTrigger(infos *TriggerInfos) (*AtTrigger, error) {
	at, err := time.Parse(time.RFC3339, infos.Arguments)
	if err != nil {
		return nil, err
	}
	return &AtTrigger{
		typ:  "@at",
		at:   at,
		in:   infos,
		done: make(chan struct{}),
	}, nil
}

// NewInTrigger returns a new instance of InTrigger given the specified
// options.
func NewInTrigger(infos *TriggerInfos) (*AtTrigger, error) {
	d, err := time.ParseDuration(infos.Arguments)
	if err != nil {
		return nil, err
	}
	at := time.Now().Add(d)
	return &AtTrigger{
		typ:  "@in",
		at:   at,
		in:   infos,
		done: make(chan struct{}),
	}, nil
}

// Type implements the Type method of the Trigger interface.
func (a *AtTrigger) Type() string {
	return a.typ
}

// Schedule implements the Schedule method of the Trigger interface.
func (a *AtTrigger) Schedule() <-chan *JobRequest {
	ch := make(chan *JobRequest)
	at := a.at
	duration := time.Since(at)
	if duration >= 0 {
		close(ch)
		return ch
	}
	go func() {
		select {
		case <-time.After(-duration):
			req := &JobRequest{
				WorkerType: a.in.WorkerType,
				Message:    a.in.Message,
				Options:    a.in.Options,
			}
			ch <- req
			close(ch)
		case <-a.done:
		}
	}()
	return ch
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (a *AtTrigger) Unschedule() {
	close(a.done)
}

// Infos implements the Infos method of the Trigger interface.
func (a *AtTrigger) Infos() *TriggerInfos {
	return a.in
}

var _ Trigger = &AtTrigger{}
