package jobs

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

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

// DocType implements the permissions.Validable interface
func (t *EventTrigger) DocType() string {
	return consts.Triggers
}

// ID implements the permissions.Validable interface
func (t *EventTrigger) ID() string {
	return ""
}

// Valid implements the permissions.Validable interface
func (t *EventTrigger) Valid(key, value string) bool {
	switch key {
	case WorkerType:
		return t.infos.WorkerType == value
	}
	return false
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
