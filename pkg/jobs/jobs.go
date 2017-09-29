package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/permissions"
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
	// JSONEncoding is a JSON encoding message type
	JSONEncoding = "json"
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

	// Message is a byte slice representing an encoded job message type.
	Message struct {
		Data []byte
		Type string
	}

	// Job contains all the metadata informations of a Job. It can be
	// marshalled in JSON.
	Job struct {
		JobID      string      `json:"_id,omitempty"`
		JobRev     string      `json:"_rev,omitempty"`
		Domain     string      `json:"domain"`
		WorkerType string      `json:"worker"`
		Message    *Message    `json:"message"`
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
		Message    *Message
		Options    *JobOptions
	}

	// JobOptions struct contains the execution properties of the jobs.
	JobOptions struct {
		MaxExecCount int           `json:"max_exec_count"`
		MaxExecTime  time.Duration `json:"max_exec_time"`
		Timeout      time.Duration `json:"timeout"`
	}
)

// ID implements the couchdb.Doc interface
func (j *Job) ID() string { return j.JobID }

// Rev implements the couchdb.Doc interface
func (j *Job) Rev() string { return j.JobRev }

// Clone implements the couchdb.Doc interface
func (j *Job) Clone() couchdb.Doc {
	cloned := *j
	if j.Message != nil {
		tmp := *j.Message
		cloned.Message = &tmp
	}
	if j.Options != nil {
		tmp := *j.Options
		cloned.Options = &tmp
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
	job := *j
	j.Logger().Debugf("[jobs] ack_consume %s ", job.ID())
	job.StartedAt = time.Now()
	job.State = Running
	*j = job
	return j.Update()
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *Job) Ack() error {
	job := *j
	j.Logger().Debugf("[jobs] ack %s ", job.ID())
	job.State = Done
	*j = job
	return j.Update()
}

// Nack sets the job infos state to Errored, set the specified error has the
// error field and sends the new job infos on the channel.
func (j *Job) Nack(err error) error {
	job := *j
	j.Logger().Debugf("[jobs] nack %s ", job.ID())
	job.State = Errored
	job.Error = err.Error()
	*j = job
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

// Marshal should not be used for a Job
func (j *Job) Marshal() ([]byte, error) {
	return nil, errors.New("should not be marshaled")
}

// Unmarshal should not be used for a Job
func (j *Job) Unmarshal() error {
	return errors.New("should not be unmarshaled")
}

// NewJob creates a new Job instance from a job request.
func NewJob(req *JobRequest) *Job {
	return &Job{
		Domain:     req.Domain,
		WorkerType: req.WorkerType,
		Message:    req.Message,
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

// NewMessage returns a new Message encoded in the specified format.
func NewMessage(enc string, data interface{}) (*Message, error) {
	var b []byte
	var err error
	switch enc {
	case JSONEncoding:
		b, err = json.Marshal(data)
	default:
		err = ErrUnknownMessageType
	}
	if err != nil {
		return nil, err
	}
	return &Message{
		Type: enc,
		Data: b,
	}, nil
}

// Unmarshal can be used to unmarshal the encoded message value in the
// specified interface's type.
func (m *Message) Unmarshal(msg interface{}) error {
	switch m.Type {
	case JSONEncoding:
		return json.NewDecoder(bytes.NewReader(m.Data)).Decode(msg)
	default:
		return ErrUnknownMessageType
	}
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
