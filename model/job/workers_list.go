package job

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// WorkersList is a map associating a worker type with its acutal
// configuration.
type WorkersList []*WorkerConfig

// workersList is the list of available workers with their associated to
// function.
var workersList WorkersList

// GetWorkersList returns a list of all activated workers, configured
// as defined by the configuration file.
func GetWorkersList() ([]*WorkerConfig, error) {
	jobsConf := config.GetConfig().Jobs
	workersConfs := jobsConf.Workers
	workers := make(WorkersList, 0, len(workersList))

	for _, w := range workersList {
		if config.GetConfig().Jobs.NoWorkers {
			w = w.Clone()
			w.Concurrency = 0
		} else {
			found := false
			for _, c := range workersConfs {
				if c.WorkerType == w.WorkerType {
					w = applyWorkerConfig(w, c)
					if found {
						logger.WithNamespace("workers_list").Warnf(
							"Configuration for the worker %q that is defined more than once",
							c.WorkerType)
					}
					found = true
				}
			}
			if jobsConf.WhiteList && !found {
				zero := 0
				w = applyWorkerConfig(w, config.Worker{Concurrency: &zero})
			}
		}
		workers = append(workers, w)
	}

	for _, c := range workersConfs {
		_, found := findWorkerByType(c.WorkerType)
		if !found {
			logger.WithNamespace("workers_list").Warnf(
				"Defined configuration for the worker %q that does not exist",
				c.WorkerType)
		}
	}

	return workers, nil
}

// GetWorkersNamesList returns the names of the configured workers
func GetWorkersNamesList() []string {
	workers, _ := GetWorkersList()
	workerNames := make([]string, len(workers))

	for i, w := range workers {
		workerNames[i] = w.WorkerType
	}
	return workerNames
}

func applyWorkerConfig(w *WorkerConfig, c config.Worker) *WorkerConfig {
	w = w.Clone()
	if c.Concurrency != nil {
		w.Concurrency = *c.Concurrency
	}
	if c.MaxExecCount != nil {
		w.MaxExecCount = *c.MaxExecCount
	}
	if c.Timeout != nil {
		w.Timeout = *c.Timeout
	}
	return w
}

func findWorkerByType(workerType string) (*WorkerConfig, bool) {
	for _, w := range workersList {
		if w.WorkerType == workerType {
			return w, true
		}
	}
	return nil, false
}

// AddWorker adds a new worker to global list of available workers.
func AddWorker(conf *WorkerConfig) {
	if conf.WorkerType == "" {
		panic("Missing worker type field")
	}
	for _, w := range workersList {
		if w.WorkerType == conf.WorkerType {
			panic(fmt.Errorf("A worker with of type %q is already defined", conf.WorkerType))
		}
	}
	workersList = append(workersList, conf)
}
