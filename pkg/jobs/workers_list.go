package jobs

import (
	"fmt"
	"time"
)

// WorkersList is a map associating a worker type with its acutal
// configuration.
type WorkersList map[string]*WorkerConfig

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
			Timeout:     10 * time.Second,
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
