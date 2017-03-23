package jobs

import (
	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

// EventTrigger implements Trigger for realtime triggered events
type EventTrigger struct {
	unscheduled chan struct{}
	infos       *TriggerInfos
	mask        permissions.Rule
}

// NewEventTrigger returns a new instance of EventTrigger given the specified
// options.
func NewEventTrigger(infos *TriggerInfos) (*EventTrigger, error) {
	rule, err := permissions.UnmarshalRuleString(infos.Arguments)
	if err != nil {
		return nil, err
	}

	return &EventTrigger{
		unscheduled: make(chan struct{}),
		infos:       infos,
		mask:        rule,
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

func (t *EventTrigger) interestedBy(e *realtime.Event) bool {

	if !t.mask.Verbs.Contains(permissions.Verb(e.Type)) {
		return false
	}

	if len(t.mask.Values) == 0 {
		return true
	}

	if t.mask.Selector == "" {
		return t.mask.ValuesContain(e.Doc.ID())
	}

	if v, ok := e.Doc.(permissions.Validable); ok {
		return t.mask.ValuesValid(v)
	}

	return false
}

func addEventToMessage(e *realtime.Event, base *Message) (*Message, error) {
	var basemsg interface{}
	base.Unmarshal(&basemsg)
	return NewMessage(JSONEncoding, map[string]interface{}{
		"message": basemsg,
		"event":   e,
	})
}

// Schedule implements the Schedule method of the Trigger interface.
func (t *EventTrigger) Schedule() <-chan *JobRequest {
	ch := make(chan *JobRequest)
	go func() {
		c := realtime.MainHub().Subscribe(t.mask.Type)
		for {
			select {
			case e := <-c.Read():
				if t.interestedBy(e) {
					msg, err := addEventToMessage(e, t.infos.Message)
					if err != nil {
						log.Error(err)
						continue
					}

					ch <- &JobRequest{
						WorkerType: t.infos.WorkerType,
						Message:    msg,
						Options:    t.infos.Options,
					}
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
