package scheduler

import (
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

type (
	// Scheduler interface is used to represent a scheduler that is responsible
	// to listen respond to triggers jobs requests and send them to the broker.
	Scheduler interface {
		Start(broker jobs.Broker) error
		Add(trigger Trigger) error
		Get(domain, id string) (Trigger, error)
		Delete(domain, id string) error
		GetAll(domain string) ([]Trigger, error)
	}

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

	// TriggerStorage interface is used to represent a persistent layer on which
	// triggers are stored.
	TriggerStorage interface {
		GetAll() ([]*TriggerInfos, error)
		Add(trigger Trigger) error
		Delete(trigger Trigger) error
	}

	// TriggerInfos is a struct containing all the options of a trigger.
	TriggerInfos struct {
		ID         string           `json:"_id,omitempty"`
		Rev        string           `json:"_rev,omitempty"`
		Domain     string           `json:"domain"`
		Type       string           `json:"type"`
		WorkerType string           `json:"worker"`
		Arguments  string           `json:"arguments"`
		Options    *jobs.JobOptions `json:"options"`
		Message    *jobs.Message    `json:"message"`
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
