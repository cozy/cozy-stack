package scheduler

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
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
	return t.infos.Type
}

// DocType implements the permissions.Validable interface
func (t *EventTrigger) DocType() string {
	return consts.Triggers
}

// ID implements the permissions.Validable interface
func (t *EventTrigger) ID() string {
	return t.infos.TID
}

// Valid implements the permissions.Validable interface
func (t *EventTrigger) Valid(key, value string) bool {
	switch key {
	case jobs.WorkerType:
		return t.infos.WorkerType == value
	}
	return false
}

// Schedule implements the Schedule method of the Trigger interface.
func (t *EventTrigger) Schedule() <-chan *jobs.JobRequest {
	ch := make(chan *jobs.JobRequest)
	go func() {
		c := realtime.GetHub().Subscribe(t.infos.Domain, t.mask.Type)
		for {
			select {
			case e := <-c.Read():
				if eventMatchPermission(e, &t.mask) {
					ch <- t.Trigger(e)
				}
			case <-t.unscheduled:
				close(ch)
				return
			}
		}
	}()
	return ch
}

// Trigger returns the triggered job request
func (t *EventTrigger) Trigger(e *realtime.Event) *jobs.JobRequest {
	var basemsg interface{}
	base := t.infos.Message
	if base != nil {
		base.Unmarshal(&basemsg)
	}
	msg, err := jobs.NewMessage(jobs.JSONEncoding, map[string]interface{}{
		"message": basemsg,
		"event":   e,
	})
	if err != nil {
		logger.WithNamespace("event-trigger").Error(err)
	}
	return &jobs.JobRequest{
		Domain:     t.infos.Domain,
		WorkerType: t.infos.WorkerType,
		Message:    msg,
		Options:    t.infos.Options,
	}
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (t *EventTrigger) Unschedule() {
	close(t.unscheduled)
}

// Infos implements the Infos method of the Trigger interface.
func (t *EventTrigger) Infos() *TriggerInfos {
	return t.infos
}

func eventMatchPermission(e *realtime.Event, rule *permissions.Rule) bool {
	if e.Doc.DocType() != rule.Type {
		return false
	}

	if !rule.Verbs.Contains(permissions.Verb(e.Type)) {
		return false
	}

	if len(rule.Values) == 0 {
		return true
	}

	if rule.Selector == "" {
		return rule.ValuesContain(e.Doc.ID())
	}

	if v, ok := e.Doc.(permissions.Validable); ok {
		if !rule.ValuesValid(v) {
			// Particular case where the new doc is not valid but the old one was.
			if e.OldDoc != nil {
				if vOld, okOld := e.OldDoc.(permissions.Validable); okOld {
					return rule.ValuesValid(vOld)
				}
			}
		} else {
			return true
		}
		return rule.ValuesValid(v)
	}

	return false
}
