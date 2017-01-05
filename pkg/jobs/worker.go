package jobs

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
)

type (
	// WorkerFunc represent the work function that a worker should implement.
	WorkerFunc func(msg *Message) error

	// Worker is a unit of work that will consume from a queue and execute the do
	// method for each jobs it pulls.
	Worker struct {
		Domain      string
		Type        string
		Concurrency int
		Func        WorkerFunc

		q Queue
	}
)

// Start is used to start the worker consumption of messages from its queue.
func (w *Worker) Start() {
	for i := 0; i < w.Concurrency; i++ {
		go func(workerID string) {
			// TODO: err handling and persistence
			for {
				job, err := w.q.Consume()
				if err != nil {
					if err != ErrQueueClosed {
						log.Errorf("[job] %s: error while consuming queue (%s)", workerID, err.Error())
					}
					return
				}
				if err = w.Func(job.Message); err != nil {
					log.Errorf("[job] %s: error while performing job (%s)", workerID, err.Error())
				}
			}
		}(fmt.Sprintf("%s/%s/%d", w.Domain, w.Type, i))
	}
}

// Stop will stop the worker's consumption of its queue. It will also close the
// associated queue.
func (w *Worker) Stop() {
	w.q.Close()
}
