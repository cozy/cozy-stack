package scheduler

import (
	"context"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// TriggerInfos is a struct containing all the options of a trigger.
type TriggerInfos struct {
	TID        string           `json:"_id,omitempty"`
	TRev       string           `json:"_rev,omitempty"`
	Domain     string           `json:"domain"`
	Type       string           `json:"type"`
	WorkerType string           `json:"worker"`
	Arguments  string           `json:"arguments"`
	Debounce   string           `json:"debounce,omitempty"`
	Options    *jobs.JobOptions `json:"options"`
	Message    *jobs.Message    `json:"message"`
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
		tmp := *t.Message
		cloned.Message = &tmp
	}
	return &cloned
}

// SetID implements the couchdb.Doc interface
func (t *TriggerInfos) SetID(id string) { t.TID = id }

// SetRev implements the couchdb.Doc interface
func (t *TriggerInfos) SetRev(rev string) { t.TRev = rev }

var _ couchdb.Doc = &TriggerInfos{}

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
