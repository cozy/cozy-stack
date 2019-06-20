package archive

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "zip",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      30 * time.Second,
		WorkerFunc:   WorkerZip,
	})

	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "unzip",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      30 * time.Second,
		WorkerFunc:   WorkerUnzip,
	})
}
