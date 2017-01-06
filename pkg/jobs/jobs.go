package jobs

import (
	"bytes"
	"encoding/json"
	"time"
)

const (
	// Queued state
	Queued State = "queued"
	// Running state
	Running = "running"
	// Errored state
	Errored = "errored"
)

const (
	// JSONEncoding is a JSON encoding message type
	JSONEncoding = "json"
)

type (
	// Queue interface is used to represent a asynchronous queue of jobs from
	// which it is possible to enqueue and consume jobs.
	Queue interface {
		Enqueue(*Job) error
		Consume() (*Job, error)
		Len() int
		Close()
	}

	// Broker interface is used to represent a job broker associated to a
	// particular domain. A broker can be used to create jobs that are pushed in
	// the job system.
	Broker interface {
		Domain() string
		PushJob(*JobRequest) (*Job, error)
	}

	// State represent the state of a job.
	State string

	// Message is a byte slice representing an encoded job message type.
	Message struct {
		Data []byte
		Type string
	}

	// Job struct contains all the parameters of a job.
	Job struct {
		ID         string      `json:"id"`
		WorkerType string      `json:"worker_type"`
		Message    *Message    `json:"-"`
		Options    *JobOptions `json:"options"`
		State      State       `json:"state"`
		QueuedAt   time.Time   `json:"queued_at"`
	}

	// JobRequest struct is used to represent a new job request.
	JobRequest struct {
		WorkerType string
		Message    *Message
		Options    *JobOptions
		Done       <-chan *Job
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
