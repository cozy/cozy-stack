package jobs

import (
	"context"
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
			WorkerFunc: func(ctx context.Context, m *Message) error {
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
			WorkerFunc: func(ctx context.Context, _ *Message) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
	}
}

// GetWorkersList returns a globally defined worker config list.
func GetWorkersList() WorkersList {
	return workersList
}

// AddWorker adds a new worker to global list of available workers.
func AddWorker(name string, conf *WorkerConfig) {
	if _, ok := workersList[name]; ok {
		panic(fmt.Errorf("A worker with the name %s is already defined", name))
	}
	workersList[name] = conf
}
