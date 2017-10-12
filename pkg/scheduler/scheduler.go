package scheduler

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

type (
	// Trigger interface is used to represent a trigger.
	Trigger interface {
		permissions.Validable
		Type() string
		Infos() *TriggerInfos
		// Schedule should return a channel on which the trigger can send job
		// requests when it decides to.
		Schedule() <-chan *jobs.JobRequest
		// Unschedule should be used to clean the trigger states and should close
		// the returns jobs channel.
		Unschedule()
	}

	// Scheduler interface is used to represent a scheduler that is responsible
	// to listen respond to triggers jobs requests and send them to the broker.
	Scheduler interface {
		Start(broker jobs.Broker) error
		Shutdown(ctx context.Context) error
		Add(trigger Trigger) error
		Get(domain, id string) (Trigger, error)
		Delete(domain, id string) error
		GetAll(domain string) ([]Trigger, error)
		RebuildRedis(domain string) error
	}

	// TriggerInfos is a struct containing all the options of a trigger.
	TriggerInfos struct {
		TID        string           `json:"_id,omitempty"`
		TRev       string           `json:"_rev,omitempty"`
		Domain     string           `json:"domain"`
		Type       string           `json:"type"`
		WorkerType string           `json:"worker"`
		Arguments  string           `json:"arguments"`
		Debounce   string           `json:"debounce"`
		Options    *jobs.JobOptions `json:"options"`
		Message    jobs.Message     `json:"message"`
	}
)

// NewTrigger creates the trigger associates with the specified trigger
// options.
func NewTrigger(infos *TriggerInfos) (Trigger, error) {
	switch infos.Type {
	case "@at":
		return NewAtTrigger(infos)
	case "@in":
		return NewInTrigger(infos)
	case "@cron":
		return NewCronTrigger(infos)
	case "@every":
		return NewEveryTrigger(infos)
	case "@event":
		return NewEventTrigger(infos)
	default:
		return nil, ErrUnknownTrigger
	}
}

// ID implements the couchdb.Doc interface
func (t *TriggerInfos) ID() string { return t.TID }

// Rev implements the couchdb.Doc interface
func (t *TriggerInfos) Rev() string { return t.TRev }

// DocType implements the couchdb.Doc interface
func (t *TriggerInfos) DocType() string { return consts.Triggers }

// Clone implements the couchdb.Doc interface
func (t *TriggerInfos) Clone() couchdb.Doc {
	cloned := *t
	if t.Options != nil {
		tmp := *t.Options
		cloned.Options = &tmp
	}
	if t.Message != nil {
		tmp := t.Message
		t.Message = make([]byte, len(tmp))
		copy(t.Message[:], tmp)
	}
	return &cloned
}

// JobRequest returns a job request associated with the scheduler informations.
func (t *TriggerInfos) JobRequest() *jobs.JobRequest {
	return &jobs.JobRequest{
		Domain:     t.Domain,
		WorkerType: t.WorkerType,
		Message:    t.Message,
		Options:    t.Options,
	}
}

// JobRequestWithEvent returns a job request associated with the scheduler
// informations associated to the specified realtime event.
func (t *TriggerInfos) JobRequestWithEvent(event *realtime.Event) (*jobs.JobRequest, error) {
	req := t.JobRequest()
	evt, err := jobs.NewEvent(event)
	if err != nil {
		return nil, err
	}
	req.Event = evt
	return req, nil
}

// SetID implements the couchdb.Doc interface
func (t *TriggerInfos) SetID(id string) { t.TID = id }

// SetRev implements the couchdb.Doc interface
func (t *TriggerInfos) SetRev(rev string) { t.TRev = rev }

var _ couchdb.Doc = &TriggerInfos{}
