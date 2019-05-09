package job

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/sirupsen/logrus"
)

const (
	// Queued state
	Queued State = "queued"
	// Running state
	Running State = "running"
	// Done state
	Done State = "done"
	// Errored state
	Errored State = "errored"
)

// defaultMaxLimits defines the maximum limit of how much jobs will be returned
// for each job state
var defaultMaxLimits map[State]int = map[State]int{
	Queued:  50,
	Running: 50,
	Done:    50,
	Errored: 50,
}

const (
	// WorkerType is the key in JSON for the type of worker
	WorkerType = "worker"
)

type (
	// Broker interface is used to represent a job broker associated to a
	// particular domain. A broker can be used to create jobs that are pushed in
	// the job system.
	Broker interface {
		StartWorkers(workersList WorkersList) error
		ShutdownWorkers(ctx context.Context) error

		// PushJob will push try to push a new job from the specified job request.
		// This method is asynchronous.
		PushJob(db prefixer.Prefixer, request *JobRequest) (*Job, error)

		// WorkerQueueLen returns the total element in the queue of the specified
		// worker type.
		WorkerQueueLen(workerType string) (int, error)
		// WorkersTypes returns the list of registered workers types.
		WorkersTypes() []string
	}

	// State represent the state of a job.
	State string

	// Message is a json encoded job message.
	Message json.RawMessage

	// Event is a json encoded value of a realtime.Event
	Event json.RawMessage

	// Job contains all the metadata informations of a Job. It can be
	// marshalled in JSON.
	Job struct {
		JobID       string      `json:"_id,omitempty"`
		JobRev      string      `json:"_rev,omitempty"`
		Domain      string      `json:"domain"`
		Prefix      string      `json:"prefix,omitempty"`
		WorkerType  string      `json:"worker"`
		TriggerID   string      `json:"trigger_id,omitempty"`
		Message     Message     `json:"message"`
		Event       Event       `json:"event"`
		Manual      bool        `json:"manual_execution,omitempty"`
		Debounced   bool        `json:"debounced,omitempty"`
		Options     *JobOptions `json:"options,omitempty"`
		State       State       `json:"state"`
		QueuedAt    time.Time   `json:"queued_at"`
		StartedAt   time.Time   `json:"started_at"`
		FinishedAt  time.Time   `json:"finished_at"`
		Error       string      `json:"error,omitempty"`
		ForwardLogs bool        `json:"forward_logs,omitempty"`
	}

	// JobRequest struct is used to represent a new job request.
	JobRequest struct {
		WorkerType  string
		TriggerID   string
		Trigger     Trigger
		Message     Message
		Event       Event
		Manual      bool
		Debounced   bool
		ForwardLogs bool
		Admin       bool
		Options     *JobOptions
	}

	// JobOptions struct contains the execution properties of the jobs.
	JobOptions struct {
		MaxExecCount int           `json:"max_exec_count"`
		MaxExecTime  time.Duration `json:"max_exec_time"`
		Timeout      time.Duration `json:"timeout"`
	}
)

var joblog = logger.WithNamespace("jobs")

// DBPrefix implements the prefixer.Prefixer interface.
func (j *Job) DBPrefix() string {
	if j.Prefix != "" {
		return j.Prefix
	}
	return j.Domain
}

// DomainName implements the prefixer.Prefixer interface.
func (j *Job) DomainName() string {
	return j.Domain
}

// ID implements the couchdb.Doc interface
func (j *Job) ID() string { return j.JobID }

// Rev implements the couchdb.Doc interface
func (j *Job) Rev() string { return j.JobRev }

// Clone implements the couchdb.Doc interface
func (j *Job) Clone() couchdb.Doc {
	cloned := *j
	if j.Options != nil {
		tmp := *j.Options
		cloned.Options = &tmp
	}
	if j.Message != nil {
		tmp := j.Message
		j.Message = make([]byte, len(tmp))
		copy(j.Message[:], tmp)
	}
	if j.Event != nil {
		tmp := j.Event
		j.Event = make([]byte, len(tmp))
		copy(j.Event[:], tmp)
	}
	return &cloned
}

// DocType implements the couchdb.Doc interface
func (j *Job) DocType() string { return consts.Jobs }

// SetID implements the couchdb.Doc interface
func (j *Job) SetID(id string) { j.JobID = id }

// SetRev implements the couchdb.Doc interface
func (j *Job) SetRev(rev string) { j.JobRev = rev }

// Match implements the permission.Matcher interface
func (j *Job) Match(key, value string) bool {
	switch key {
	case WorkerType:
		return j.WorkerType == value
	}
	return false
}

// ID implements the permission.Matcher interface
func (jr *JobRequest) ID() string { return "" }

// DocType implements the permission.Matcher interface
func (jr *JobRequest) DocType() string { return consts.Jobs }

// Match implements the permission.Matcher interface
func (jr *JobRequest) Match(key, value string) bool {
	switch key {
	case WorkerType:
		return jr.WorkerType == value
	}
	return false
}

// Logger returns a logger associated with the job domain
func (j *Job) Logger() *logrus.Entry {
	return logger.WithDomain(j.Domain).WithField("nspace", "jobs")
}

// AckConsumed sets the job infos state to Running an sends the new job infos
// on the channel.
func (j *Job) AckConsumed() error {
	j.Logger().Debugf("ack_consume %s ", j.ID())
	j.StartedAt = time.Now()
	j.State = Running
	return j.Update()
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *Job) Ack() error {
	j.Logger().Debugf("ack %s ", j.ID())
	j.FinishedAt = time.Now()
	j.State = Done
	j.Event = nil
	return j.Update()
}

// Nack sets the job infos state to Errored, set the specified error has the
// error field and sends the new job infos on the channel.
func (j *Job) Nack(err error) error {
	j.Logger().Debugf("nack %s ", j.ID())
	j.FinishedAt = time.Now()
	j.State = Errored
	j.Error = err.Error()
	j.Event = nil
	return j.Update()
}

// Update updates the job in couchdb
func (j *Job) Update() error {
	return couchdb.UpdateDoc(j, j)
}

// Create creates the job in couchdb
func (j *Job) Create() error {
	return couchdb.CreateDoc(j, j)
}

// UnmarshalJSON implements json.Unmarshaler on Message. It should be retro-
// compatible with the old Message representation { Data, Type }.
func (m *Message) UnmarshalJSON(data []byte) error {
	// For retro-compatibility purposes
	var mm struct {
		Data []byte `json:"Data"`
		Type string `json:"Type"`
	}
	if err := json.Unmarshal(data, &mm); err == nil && mm.Type == "json" {
		var v json.RawMessage
		if err = json.Unmarshal(mm.Data, &v); err != nil {
			return err
		}
		*m = Message(v)
		return nil
	}
	var v json.RawMessage
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = Message(v)
	return nil
}

// MarshalJSON implements json.Marshaler on Message.
func (m Message) MarshalJSON() ([]byte, error) {
	v := json.RawMessage(m)
	return json.Marshal(v)
}

// NewJob creates a new Job instance from a job request.
func NewJob(db prefixer.Prefixer, req *JobRequest) *Job {
	return &Job{
		Domain:      db.DomainName(),
		Prefix:      db.DBPrefix(),
		WorkerType:  req.WorkerType,
		TriggerID:   req.TriggerID,
		Manual:      req.Manual,
		Message:     req.Message,
		Debounced:   req.Debounced,
		Event:       req.Event,
		Options:     req.Options,
		ForwardLogs: req.ForwardLogs,
		State:       Queued,
		QueuedAt:    time.Now(),
	}
}

// Get returns the informations about a job.
func Get(db prefixer.Prefixer, jobID string) (*Job, error) {
	var job Job
	if err := couchdb.GetDoc(db, consts.Jobs, jobID, &job); err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, ErrNotFoundJob
		}
		return nil, err
	}
	return &job, nil
}

// GetQueuedJobs returns the list of jobs which states is "queued" or "running"
func GetQueuedJobs(db prefixer.Prefixer, workerType string) ([]*Job, error) {
	var results []*Job
	req := &couchdb.FindRequest{
		UseIndex: "by-worker-and-state",
		Selector: mango.And(
			mango.Equal("worker", workerType),
			mango.Exists("state"), // XXX it is needed by couchdb to use the index
			mango.Or(
				mango.Equal("state", Queued),
				mango.Equal("state", Running),
			),
		),
		Limit: 200,
	}
	err := couchdb.FindDocs(db, consts.Jobs, req, &results)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// GetJobsBeforeDate returns alls jobs queued before the specified date
func GetJobsBeforeDate(db prefixer.Prefixer, date time.Time) ([]*Job, error) {
	var jobs []*Job

	req := &couchdb.FindRequest{
		UseIndex: "by-queued-at",
		Selector: mango.Lt("queued_at", date.Format(time.RFC3339Nano)),
	}

	err := couchdb.FindDocs(db, consts.Jobs, req, &jobs)
	if err != nil {
		return nil, err
	}

	return jobs, err
}

// GetLastsJobs returns the N lasts job of each state for an instance/worker
// type pair
func GetLastsJobs(db prefixer.Prefixer, workerType string) ([]*Job, error) {
	var result []*Job

	for _, state := range []State{Queued, Running, Done, Errored} {
		jobs := []*Job{}
		limit := defaultMaxLimits[state]

		req := &couchdb.FindRequest{
			Selector: mango.And(
				mango.Equal("worker", workerType),
				mango.Equal("state", state),
			),
			Sort: mango.SortBy{
				{Field: "queued_at", Direction: mango.Desc},
			},
			Limit: limit,
		}
		err := couchdb.FindDocs(db, consts.Jobs, req, &jobs)
		if err != nil {
			return nil, err
		}
		result = append(result, jobs...)
	}

	return result, nil
}

// NewMessage returns a json encoded data
func NewMessage(data interface{}) (Message, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return Message(b), nil
}

// NewEvent return a json encoded realtime.Event
func NewEvent(data *realtime.Event) (Event, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return Event(b), nil
}

// Unmarshal can be used to unmarshal the encoded message value in the
// specified interface's type.
func (m Message) Unmarshal(msg interface{}) error {
	if m == nil {
		return ErrMessageNil
	}
	if err := json.Unmarshal(m, &msg); err != nil {
		return ErrMessageUnmarshal
	}
	return nil
}

// Unmarshal can be used to unmarshal the encoded message value in the
// specified interface's type.
func (e Event) Unmarshal(evt interface{}) error {
	if e == nil {
		return ErrMessageNil
	}
	if err := json.Unmarshal(e, &evt); err != nil {
		return ErrMessageUnmarshal
	}
	return nil
}

var (
	_ permission.Matcher = (*JobRequest)(nil)
	_ permission.Matcher = (*Job)(nil)
)
