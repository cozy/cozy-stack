package jobs

import "fmt"

// WorkersList is a map associating a worker type with its acutal
// configuration.
type WorkersList map[string]*WorkerConfig

// WorkersList is the list of available workers with their associated Do
// function.
var workersList = WorkersList{}

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
