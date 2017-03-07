package jobs

import "github.com/cozy/cozy-stack/pkg/realtime"

// EventTrigger implements Trigger for realtime triggered events
type EventTrigger struct {
	unscheduled chan struct{}
	infos       *TriggerInfos
}

// NewEventTrigger returns a new instance of AtTrigger given the specified
// options.
func NewEventTrigger(infos *TriggerInfos) (*EventTrigger, error) {
	return &EventTrigger{
		unscheduled: make(chan struct{}),
		infos:       infos,
	}, nil
}

// Type implements the Type method of the Trigger interface.
func (t *EventTrigger) Type() string {
	return "@event"
}

// Schedule implements the Schedule method of the Trigger interface.
func (t *EventTrigger) Schedule() <-chan *JobRequest {
	ch := make(chan *JobRequest)
	go func() {
		c := realtime.MainHub().Subscribe(t.infos.Arguments)
		for {
			select {
			case <-c.Read():
				ch <- &JobRequest{
					WorkerType: t.infos.WorkerType,
					Message:    t.infos.Message,
					Options:    t.infos.Options,
				}
			case <-t.unscheduled:
				close(ch)
				return
			}
		}
	}()
	return ch
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (t *EventTrigger) Unschedule() {
	close(t.unscheduled)
}

// Infos implements the Infos method of the Trigger interface.
func (t *EventTrigger) Infos() *TriggerInfos {
	return t.infos
}
