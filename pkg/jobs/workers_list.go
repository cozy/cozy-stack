package jobs

import (
	"fmt"
	"time"
)

// WorkersList is a map associating a worker type with its acutal
// configuration.
type WorkersList map[string]*WorkerConfig

// WorkerConfig is the configuration parameter of a worker defined by the job
// system. It contains parameters of the worker along with the worker main
// function that perform the work against a job's message.
type WorkerConfig struct {
	WorkerFunc   WorkerFunc
	Concurrency  uint
	MaxExecCount uint
	MaxExecTime  time.Duration
	Timeout      time.Duration
	RetryDelay   time.Duration
}

// WorkersList is the list of available workers with their associated Do
// function.
var workersList WorkersList

func init() {
	workersList = WorkersList{
		"print": {
			Concurrency: 4,
			WorkerFunc: func(m *Message, _ <-chan time.Time) error {
				var msg string
				if err := m.Unmarshal(&msg); err != nil {
					return err
				}
				_, err := fmt.Println(msg)
				return err
			},
		},
		"timeout": {
			Concurrency: 4,
			Timeout:     1 * time.Second,
			WorkerFunc: func(_ *Message, timeout <-chan time.Time) error {
				<-timeout
				return ErrTimedOut
			},
		},
	}
}

// GetWorkersList returns a globally defined worker config list
func GetWorkersList() WorkersList {
	return workersList
}
