package jobs

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
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
	// Queue interface is used to represent an asynchronous queue of jobs from
	// which it is possible to enqueue and consume jobs.
	Queue interface {
		Enqueue(job Job) error
		Consume() (Job, error)
		Len() int
		Close()
	}

	// Broker interface is used to represent a job broker associated to a
	// particular domain. A broker can be used to create jobs that are pushed in
	// the job system.
	Broker interface {
		// PushJob will push try to push a new job from the specified job request.
		//
		// This method is asynchronous and returns a chan of JobInfos to observe
		// the job changing states. This channel does not need to be subscribed,
		// messages will be dropped if no listeners.
		PushJob(request *JobRequest) (*JobInfos, <-chan *JobInfos, error)

		// QueueLen returns the total element in the queue of the specified worker
		// type.
		QueueLen(workerType string) (int, error)
	}

	// Job interface represents a job.
	Job interface {
		// Domain returns the domain name from which the job has been sent.
		Domain() string
		// Infos returns the JobInfos data associated with the job
		Infos() *JobInfos
		// AckConsumed should be used by the consumer of the job, ack-ing that
		// it has well received the job and is processing it.
		AckConsumed() error
		// Ack should be used by the consumer after the job has been processed,
		// ack-ing that the job was successfully executed.
		Ack() error
		// Nack should be used to tell that the job coult not be consumed or that
		// an error has happened during its processing. The error passed will be
		// used to inform in more detail about the error that happened.
		Nack(error) error
		// Marshal allows you to define how the job should be marshalled when put
		// into the queue.
		Marshal() ([]byte, error)
		// Unmarshal allows you to define how the job should be unmarshalled when
		// consumed from the queue.
		Unmarshal() error
	}

	// State represent the state of a job.
	State string

	// Message is a byte slice representing an encoded job message type.
	Message struct {
		Data []byte
		Type string
	}

	// JobInfos contains all the metadata informations of a Job. It can be
	// marshalled in JSON.
	JobInfos struct {
		JobID      string      `json:"_id,omitempty"`
		JobRev     string      `json:"_rev,omitempty"`
		Domain     string      `json:"domain"`
		WorkerType string      `json:"worker"`
		Message    *Message    `json:"message"`
		Options    *JobOptions `json:"options"`
		State      State       `json:"state"`
		QueuedAt   time.Time   `json:"queued_at"`
		StartedAt  time.Time   `json:"started_at"`
		Error      string      `json:"error"`
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
		MaxExecCount uint          `json:"max_exec_count"`
		MaxExecTime  time.Duration `json:"max_exec_time"`
		Timeout      time.Duration `json:"timeout"`
	}

	// WorkerConfig is the configuration parameter of a worker defined by the job
	// system. It contains parameters of the worker along with the worker main
	// function that perform the work against a job's message.
	WorkerConfig struct {
		WorkerFunc   WorkerFunc    `json:"worker_func"`
		Concurrency  uint          `json:"concurrency"`
		MaxExecCount uint          `json:"max_exec_count"`
		MaxExecTime  time.Duration `json:"max_exec_time"`
		Timeout      time.Duration `json:"timeout"`
		RetryDelay   time.Duration `json:"retry_delay"`
	}
)

// ID implements the couchdb.Doc interface
func (ji *JobInfos) ID() string { return ji.JobID }

// Rev implements the couchdb.Doc interface
func (ji *JobInfos) Rev() string { return ji.JobRev }

// Clone implements the couchdb.Doc interface
func (ji *JobInfos) Clone() couchdb.Doc { return ji }

// DocType implements the couchdb.Doc interface
func (ji *JobInfos) DocType() string { return consts.Jobs }

// SetID implements the couchdb.Doc interface
func (ji *JobInfos) SetID(id string) { ji.JobID = id }

// SetRev implements the couchdb.Doc interface
func (ji *JobInfos) SetRev(rev string) { ji.JobRev = rev }

// Valid implements the permissions.Validable interface
func (ji *JobInfos) Valid(key, value string) bool {
	switch key {
	case WorkerType:
		return ji.WorkerType == value
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

// NewJobInfos creates a new JobInfos instance from a job request.
func NewJobInfos(req *JobRequest) *JobInfos {
	return &JobInfos{
		Domain:     req.Domain,
		WorkerType: req.WorkerType,
		Message:    req.Message,
		Options:    req.Options,
		State:      Queued,
		QueuedAt:   time.Now(),
	}
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

func (w *WorkerConfig) clone() *WorkerConfig {
	return &WorkerConfig{
		WorkerFunc:   w.WorkerFunc,
		Concurrency:  w.Concurrency,
		MaxExecCount: w.MaxExecCount,
		MaxExecTime:  w.MaxExecTime,
		Timeout:      w.Timeout,
		RetryDelay:   w.RetryDelay,
	}
}

var (
	_ permissions.Validable = (*JobRequest)(nil)
	_ permissions.Validable = (*JobInfos)(nil)
)
