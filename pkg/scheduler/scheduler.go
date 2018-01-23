package scheduler

import (
	"context"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
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
		TID          string           `json:"_id,omitempty"`
		TRev         string           `json:"_rev,omitempty"`
		Domain       string           `json:"domain"`
		Type         string           `json:"type"`
		WorkerType   string           `json:"worker"`
		Arguments    string           `json:"arguments"`
		Debounce     string           `json:"debounce"`
		Options      *jobs.JobOptions `json:"options"`
		Message      jobs.Message     `json:"message"`
		CurrentState *TriggerState    `json:"current_state,omitempty"`
	}

	// TriggerState represent the current state of the trigger
	TriggerState struct {
		TID                 string     `json:"trigger_id"`
		Status              jobs.State `json:"status"`
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
		TriggerID:  t.ID(),
		Message:    t.Message,
		Options:    t.Options,
	}
}

// JobRequestWithEvent returns a job request associated with the scheduler
// informations associated to the specified realtime event.
func (t *TriggerInfos) JobRequestWithEvent(event *realtime.Event) (*jobs.JobRequest, error) {
	evt, err := jobs.NewEvent(event)
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

// Valid implements the permissions.Validable interface
func (t *TriggerInfos) Valid(key, value string) bool {
	switch key {
	case "worker":
		return t.WorkerType == value
	}
	return false
}

// GetJobs returns the jobs launched by the given trigger.
func GetJobs(t Trigger, limit int) ([]*jobs.Job, error) {
	triggerInfos := t.Infos()
	db := couchdb.SimpleDatabasePrefix(triggerInfos.Domain)
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	var jobs []*jobs.Job
	req := &couchdb.FindRequest{
		UseIndex: "by-trigger-id",
		Selector: mango.Equal("trigger_id", triggerInfos.ID()),
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
func GetTriggerState(t Trigger) (*TriggerState, error) {
	js, err := GetJobs(t, 0)
	if err != nil {
		return nil, err
	}

	var state TriggerState

	state.Status = jobs.Done
	state.TID = t.ID()

	for _, j := range js {
		startedAt := &j.StartedAt
		switch j.State {
		case jobs.Errored:
			state.LastFailure = startedAt
			state.LastFailedJobID = j.ID()
			state.LastError = j.Error
		case jobs.Done:
			state.LastSuccess = startedAt
			state.LastSuccessfulJobID = j.ID()
		}
		if j.Manual && (j.State == jobs.Done || j.State == jobs.Errored) {
			state.LastManualExecution = startedAt
			state.LastManualJobID = j.ID()
		}
		state.LastExecution = startedAt
		state.LastExecutedJobID = j.ID()
		state.Status = j.State
	}

	return &state, nil
}

var _ couchdb.Doc = &TriggerInfos{}
