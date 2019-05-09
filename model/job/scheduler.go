package job

import (
	"context"
	"time"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

type (
	// Trigger interface is used to represent a trigger.
	Trigger interface {
		prefixer.Prefixer
		permission.Matcher
		Type() string
		Infos() *TriggerInfos
		// Schedule should return a channel on which the trigger can send job
		// requests when it decides to.
		Schedule() <-chan *JobRequest
		// Unschedule should be used to clean the trigger states and should close
		// the returns jobs channel.
		Unschedule()
	}

	// Scheduler interface is used to represent a scheduler that is responsible
	// to listen respond to triggers jobs requests and send them to the broker.
	Scheduler interface {
		StartScheduler(broker Broker) error
		ShutdownScheduler(ctx context.Context) error
		PollScheduler(now int64) error
		AddTrigger(trigger Trigger) error
		GetTrigger(db prefixer.Prefixer, id string) (Trigger, error)
		DeleteTrigger(db prefixer.Prefixer, id string) error
		GetAllTriggers(db prefixer.Prefixer) ([]Trigger, error)
		CleanRedis() error
		RebuildRedis(db prefixer.Prefixer) error
	}

	// TriggerInfos is a struct containing all the options of a trigger.
	TriggerInfos struct {
		TID          string        `json:"_id,omitempty"`
		TRev         string        `json:"_rev,omitempty"`
		Domain       string        `json:"domain"`
		Prefix       string        `json:"prefix,omitempty"`
		Type         string        `json:"type"`
		WorkerType   string        `json:"worker"`
		Arguments    string        `json:"arguments"`
		Debounce     string        `json:"debounce"`
		Options      *JobOptions   `json:"options"`
		Message      Message       `json:"message"`
		CurrentState *TriggerState `json:"current_state,omitempty"`
	}

	// TriggerState represent the current state of the trigger
	TriggerState struct {
		TID                 string     `json:"trigger_id"`
		Status              State      `json:"status"`
		LastSuccess         *time.Time `json:"last_success,omitempty"`
		LastSuccessfulJobID string     `json:"last_successful_job_id,omitempty"`
		LastExecution       *time.Time `json:"last_execution,omitempty"`
		LastExecutedJobID   string     `json:"last_executed_job_id,omitempty"`
		LastFailure         *time.Time `json:"last_failure,omitempty"`
		LastFailedJobID     string     `json:"last_failed_job_id,omitempty"`
		LastError           string     `json:"last_error,omitempty"`
		LastManualExecution *time.Time `json:"last_manual_execution,omitempty"`
		LastManualJobID     string     `json:"last_manual_job_id,omitempty"`
	}
)

// DBPrefix implements the prefixer.Prefixer interface.
func (t *TriggerInfos) DBPrefix() string {
	if t.Prefix != "" {
		return t.Prefix
	}
	return t.Domain
}

// DomainName implements the prefixer.Prefixer interface.
func (t *TriggerInfos) DomainName() string {
	return t.Domain
}

// NewTrigger creates the trigger associates with the specified trigger
// options.
func NewTrigger(db prefixer.Prefixer, infos TriggerInfos, data interface{}) (Trigger, error) {
	var msg Message
	var err error
	if data != nil {
		msg, err = NewMessage(data)
		if err != nil {
			return nil, err
		}
	}
	infos.Message = msg
	infos.Prefix = db.DBPrefix()
	infos.Domain = db.DomainName()
	return fromTriggerInfos(&infos)
}

func fromTriggerInfos(infos *TriggerInfos) (Trigger, error) {
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
	if t.CurrentState != nil {
		tmp := *t.CurrentState
		cloned.CurrentState = &tmp
	}
	return &cloned
}

// JobRequest returns a job request associated with the scheduler informations.
func (t *TriggerInfos) JobRequest() *JobRequest {
	trigger, _ := fromTriggerInfos(t)
	return &JobRequest{
		WorkerType: t.WorkerType,
		TriggerID:  t.ID(),
		Trigger:    trigger,
		Message:    t.Message,
		Options:    t.Options,
	}
}

// JobRequestWithEvent returns a job request associated with the scheduler
// informations associated to the specified realtime event.
func (t *TriggerInfos) JobRequestWithEvent(event *realtime.Event) (*JobRequest, error) {
	evt, err := NewEvent(event)
	if err != nil {
		return nil, err
	}
	req := t.JobRequest()
	req.Event = evt
	return req, nil
}

// SetID implements the couchdb.Doc interface
func (t *TriggerInfos) SetID(id string) { t.TID = id }

// SetRev implements the couchdb.Doc interface
func (t *TriggerInfos) SetRev(rev string) { t.TRev = rev }

// Match implements the permission.Matcher interface
func (t *TriggerInfos) Match(key, value string) bool {
	switch key {
	case "worker":
		return t.WorkerType == value
	}
	return false
}

// GetJobs returns the jobs launched by the given trigger.
func GetJobs(db prefixer.Prefixer, triggerID string, limit int) ([]*Job, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	var jobs []*Job
	req := &couchdb.FindRequest{
		UseIndex: "by-trigger-id",
		Selector: mango.Equal("trigger_id", triggerID),
		Sort: mango.SortBy{
			{Field: "trigger_id", Direction: mango.Desc},
			{Field: "queued_at", Direction: mango.Desc},
		},
		Limit: limit,
	}
	err := couchdb.FindDocs(db, consts.Jobs, req, &jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// GetTriggerState returns the state of the trigger, calculated from the last
// launched jobs.
func GetTriggerState(db prefixer.Prefixer, triggerID string) (*TriggerState, error) {
	js, err := GetJobs(db, triggerID, 0)
	if err != nil {
		return nil, err
	}

	var state TriggerState

	state.Status = Done
	state.TID = triggerID

	// jobs are ordered from the oldest to most recent job
	for i := len(js) - 1; i >= 0; i-- {
		j := js[i]
		startedAt := &j.StartedAt

		state.Status = j.State
		state.LastExecution = startedAt
		state.LastExecutedJobID = j.ID()

		if j.Manual {
			state.LastManualExecution = startedAt
			state.LastManualJobID = j.ID()
		}

		switch j.State {
		case Errored:
			state.LastFailure = startedAt
			state.LastFailedJobID = j.ID()
			state.LastError = j.Error
		case Done:
			state.LastSuccess = startedAt
			state.LastSuccessfulJobID = j.ID()
		default:
			// skip any job that is not done or errored
			continue
		}
	}

	return &state, nil
}

var _ couchdb.Doc = &TriggerInfos{}
