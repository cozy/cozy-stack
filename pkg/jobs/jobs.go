package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/sirupsen/logrus"
)

const (
	// Queued state
	Queued State = "queued"
	// Running state
	Running = "running"
	// Done state
	Done = "done"
	// Errored state
	Errored = "errored"
)

const (
	// WorkerType is the key in JSON for the type of worker
	WorkerType = "worker"
)

type (
	// Broker interface is used to represent a job broker associated to a
	// particular domain. A broker can be used to create jobs that are pushed in
	// the job system.
	Broker interface {
		Start(workersList WorkersList) error
		Shutdown(ctx context.Context) error

		// PushJob will push try to push a new job from the specified job request.
		// This method is asynchronous.
		PushJob(request *JobRequest) (*Job, error)

		// QueueLen returns the total element in the queue of the specified worker
		// type.
		QueueLen(workerType string) (int, error)
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
		JobID      string      `json:"_id,omitempty"`
		JobRev     string      `json:"_rev,omitempty"`
		Domain     string      `json:"domain"`
		WorkerType string      `json:"worker"`
		Message    Message     `json:"message"`
		Event      Event       `json:"event"`
		Debounced  bool        `json:"debounced"`
		Options    *JobOptions `json:"options"`
		State      State       `json:"state"`
		QueuedAt   time.Time   `json:"queued_at"`
		StartedAt  time.Time   `json:"started_at,omitempty"`
		Error      string      `json:"error,omitempty"`
	}

	// JobRequest struct is used to represent a new job request.
	JobRequest struct {
		Domain     string
		WorkerType string
		Message    Message
		Event      Event
		Debounced  bool
		Options    *JobOptions
	}

	// JobOptions struct contains the execution properties of the jobs.
	JobOptions struct {
		MaxExecCount int           `json:"max_exec_count"`
		MaxExecTime  time.Duration `json:"max_exec_time"`
		Timeout      time.Duration `json:"timeout"`
	}
)

var joblog = logger.WithNamespace("jobs")

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

// Valid implements the permissions.Validable interface
func (j *Job) Valid(key, value string) bool {
	switch key {
	case WorkerType:
		return j.WorkerType == value
	}
	return false
}

// ID implements the permissions.Validable interface
func (jr *JobRequest) ID() string { return "" }

// DocType implements the permissions.Validable interface
func (jr *JobRequest) DocType() string { return consts.Jobs }

// Valid implements the permissions.Validable interface
func (jr *JobRequest) Valid(key, value string) bool {
	switch key {
	case WorkerType:
		return jr.WorkerType == value
	}
	return false
}

// Logger returns a logger associated with the job domain
func (j *Job) Logger() *logrus.Entry {
	return logger.WithDomain(j.Domain)
}

// AckConsumed sets the job infos state to Running an sends the new job infos
// on the channel.
func (j *Job) AckConsumed() error {
	j.Logger().Debugf("[jobs] ack_consume %s ", j.ID())
	j.StartedAt = time.Now()
	j.State = Running
	return j.Update()
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *Job) Ack() error {
	j.Logger().Debugf("[jobs] ack %s ", j.ID())
	j.State = Done
	return j.Update()
}

// Nack sets the job infos state to Errored, set the specified error has the
// error field and sends the new job infos on the channel.
func (j *Job) Nack(err error) error {
	j.Logger().Debugf("[jobs] nack %s ", j.ID())
	j.State = Errored
	j.Error = err.Error()
	return j.Update()
}

// Update updates the job in couchdb
func (j *Job) Update() error {
	return couchdb.UpdateDoc(j.db(), j)
}

// Create creates the job in couchdb
func (j *Job) Create() error {
	return couchdb.CreateDoc(j.db(), j)
}

func (j *Job) db() couchdb.Database {
	return couchdb.SimpleDatabasePrefix(j.Domain)
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
func NewJob(req *JobRequest) *Job {
	return &Job{
		Domain:     req.Domain,
		WorkerType: req.WorkerType,
		Message:    req.Message,
		Debounced:  req.Debounced,
		Event:      req.Event,
		Options:    req.Options,
		State:      Queued,
		QueuedAt:   time.Now(),
	}
}

// Get returns the informations about a job.
func Get(domain, jobID string) (*Job, error) {
	var job Job
	db := couchdb.SimpleDatabasePrefix(domain)
	if err := couchdb.GetDoc(db, consts.Jobs, jobID, &job); err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, ErrNotFoundJob
		}
		return nil, err
	}
	return &job, nil
}

// GetQueuedJobs returns the list of jobs which states is "queued" or "running"
func GetQueuedJobs(domain, workerType string) ([]*Job, error) {
	var results []*Job
	db := couchdb.SimpleDatabasePrefix(domain)
	req := &couchdb.FindRequest{
		UseIndex: "by-worker-and-state",
		Selector: mango.And(
			mango.Equal("worker", workerType),
			mango.Or(
				mango.Equal("state", Queued),
				mango.Equal("state", Running),
			),
		),
		Limit: 200,
	}
	err := couchdb.FindDocs(db, consts.Jobs, req, &results)
	return results, err
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
	return json.Unmarshal(m, &msg)
}

// Unmarshal can be used to unmarshal the encoded message value in the
// specified interface's type.
func (e Event) Unmarshal(evt interface{}) error {
	if e == nil {
		return ErrMessageNil
	}
	return json.Unmarshal(e, &evt)
}

// Clone clones the worker config
func (w *WorkerConfig) Clone() *WorkerConfig {
	return &WorkerConfig{
		WorkerInit:         w.WorkerInit,
		WorkerFunc:         w.WorkerFunc,
		WorkerThreadedFunc: w.WorkerThreadedFunc,
		WorkerCommit:       w.WorkerCommit,
		Concurrency:        w.Concurrency,
		MaxExecCount:       w.MaxExecCount,
		MaxExecTime:        w.MaxExecTime,
		Timeout:            w.Timeout,
		RetryDelay:         w.RetryDelay,
	}
}

var (
	_ permissions.Validable = (*JobRequest)(nil)
	_ permissions.Validable = (*Job)(nil)
)
