package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
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

var idRand *rand.Rand

func init() {
	idRand = rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
}

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

	// Job struct encapsulates all the parameters of a job.
	Job struct {
		ID         string    `json:"id"`
		WorkerType string    `json:"worker_type"`
		Message    *Message  `json:"-"`
		State      State     `json:"state"`
		QueuedAt   time.Time `json:"queued_at"`
	}

	// JobRequest struct is used to represent a new job request.
	JobRequest struct {
		WorkerType string
		Message    *Message
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

func makeQueueName(domain, workerType string) string {
	return fmt.Sprintf("%s/%s", domain, workerType)
}

func makeID() string {
	const (
		idLen   = 16
		letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	)
	b := make([]byte, idLen)
	lenLetters := len(letters)
	for i := 0; i < idLen; i++ {
		idx := idRand.Intn(lenLetters)
		b[i] = letters[idx]
	}
	return string(b)
}
