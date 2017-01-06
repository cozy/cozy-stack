package jobs

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
)

var (
	defaultConcurrency  = 1
	defaultMaxExecCount = 3
	defaultMaxExecTime  = 60 * time.Second
	defaultRetryDelay   = 60 * time.Millisecond
	defaultTimeout      = 10 * time.Second
)

type (
	// WorkerFunc represent the work function that a worker should implement.
	WorkerFunc func(msg *Message, timeout <-chan time.Time) error

	// Worker is a unit of work that will consume from a queue and execute the do
	// method for each jobs it pulls.
	Worker struct {
		Domain string
		Type   string
		Conf   *WorkerConfig

		q       Queue
		started int32
	}
)

// Start is used to start the worker consumption of messages from its queue.
func (w *Worker) Start(q Queue) {
	if !atomic.CompareAndSwapInt32(&w.started, 0, 1) {
		return
	}
	w.q = q
	c := &(*w.Conf)
	if c.Concurrency == 0 {
		c.Concurrency = uint(defaultConcurrency)
	}
	if c.MaxExecCount == 0 {
		c.MaxExecCount = uint(defaultMaxExecCount)
	}
	if c.MaxExecTime == 0 {
		c.MaxExecTime = defaultMaxExecTime
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = defaultRetryDelay
	}
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
	w.Conf = c
	for i := 0; i < int(c.Concurrency); i++ {
		name := fmt.Sprintf("%s/%s/%d", w.Domain, w.Type, i)
		go w.work(name)
	}
}

func (w *Worker) work(workerID string) {
	// TODO: err handling and persistence
	for {
		job, err := w.q.Consume()
		if err != nil {
			if err != ErrQueueClosed {
				log.Errorf("[job] %s: error while consuming queue (%s)",
					workerID, err.Error())
			}
			return
		}
		t := &task{
			job:  job,
			conf: w.Conf,
		}
		if err = t.run(); err != nil {
			log.Errorf("[job] %s: error while performing job %s (%s)",
				workerID, job.ID, err.Error())
		}
	}
}

// Stop will stop the worker's consumption of its queue. It will also close the
// associated queue.
func (w *Worker) Stop() {
	if !atomic.CompareAndSwapInt32(&w.started, 1, 0) {
		return
	}
	w.q.Close()
}

type task struct {
	job  *Job
	conf *WorkerConfig

	startTime time.Time
	execCount uint
}

func (t *task) run() (err error) {
	t.startTime = time.Now()
	t.execCount = 0
	for {
		retry, delay, timeout := t.nextDelay()
		if !retry {
			return err
		}
		if err != nil {
			log.Warnf("[job] %s: %s (retry in %s)", t.job.ID, err.Error(), delay)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		log.Debugf("[job] %s: run %d (timeout %s)", t.job.ID, t.execCount, timeout)
		err = t.conf.WorkerFunc(t.job.Message, time.After(timeout))
		if err == nil {
			break
		}
		t.execCount++
	}
	return nil
}

func (t *task) nextDelay() (bool, time.Duration, time.Duration) {
	c := t.conf
	execTime := time.Since(t.startTime)

	if t.execCount >= c.MaxExecCount || execTime > c.MaxExecTime {
		return false, 0, 0
	}

	// the worker timeout should take into account the maximum execution time
	// allowed to the task
	timeout := c.Timeout
	if execTime+timeout > c.MaxExecTime {
		timeout = execTime - c.MaxExecTime
	}

	var nextDelay time.Duration
	if t.execCount == 0 {
		// on first execution, execute immediatly
		nextDelay = 0
	} else {
		nextDelay = c.RetryDelay << (t.execCount - 1)

		// fuzzDelay number between delay * (1 +/- 0.1)
		fuzzDelay := int(0.1 * float64(nextDelay))
		nextDelay = nextDelay + time.Duration((rand.Intn(2*fuzzDelay) - fuzzDelay))
	}

	if execTime+nextDelay > c.MaxExecTime {
		return false, 0, 0
	}

	return true, nextDelay, timeout
}
