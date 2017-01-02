package jobs

import "fmt"

// WorkersList is a map associating a worker type with its acutal
// configuration.
type WorkersList map[string]WorkerConfig

// WorkerConfig is the configuration parameter of a worker defined by the job
// system. It contains parameters of the worker along with the worker main
// function that perform the work against a job's message.
type WorkerConfig struct {
	Concurrency int
	WorkerFunc  WorkerFunc
}

// WorkersList is the list of available workers with their associated Do
// function.
var workersList = WorkersList{
	"print": {
		Concurrency: 4,
		WorkerFunc: func(m *Message) error {
			var msg string
			if err := m.Unmarshal(&msg); err != nil {
				return err
			}
			_, err := fmt.Println(msg)
			return err
		},
	},
}

// GetWorkersList returns a globally defined worker config list
func GetWorkersList() WorkersList {
	return workersList
}
