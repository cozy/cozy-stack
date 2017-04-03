package jobs

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
)

// contextKey are the keys used in the worker context
type contextKey int

const (
	// ContextDomainKey is the used to store the domain string name
	ContextDomainKey contextKey = iota
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
	WorkerFunc func(context context.Context, msg *Message) error

	// Worker is a unit of work that will consume from a queue and execute the do
	// method for each jobs it pulls.
	Worker struct {
		Domain string
		Type   string
		Conf   *WorkerConfig

		jobs    Queue
		started int32
	}
)

// NewWorkerContext returns a context.Context usable by a worker.
func NewWorkerContext(domain string) context.Context {
	return context.WithValue(context.Background(), ContextDomainKey, domain)
}

// Start is used to start the worker consumption of messages from its queue.
func (w *Worker) Start(q Queue) {
	if !atomic.CompareAndSwapInt32(&w.started, 0, 1) {
		return
	}
	w.jobs = q
	for i := 0; i < int(w.Conf.Concurrency); i++ {
		name := fmt.Sprintf("%s/%s/%d", w.Domain, w.Type, i)
		go w.work(name)
	}
}

func (w *Worker) work(workerID string) {
	// TODO: err handling and persistence
	parentCtx := NewWorkerContext(w.Domain)
	for {
		job, err := w.jobs.Consume()
		if err != nil {
			if err != ErrQueueClosed {
				log.Errorf("[job] %s: error while consuming queue (%s)",
					workerID, err.Error())
			}
			return
		}
		infos := job.Infos()
		if err = job.AckConsumed(); err != nil {
			log.Errorf("[job] %s: error acking consume job %s (%s)",
				workerID, infos.ID, err.Error())
			continue
		}
		t := &task{
			ctx:   parentCtx,
			infos: infos,
			conf:  w.defaultedConf(infos.Options),
		}
		if err = t.run(); err != nil {
			log.Errorf("[job] %s: error while performing job %s (%s)",
				workerID, infos.ID, err.Error())
			err = job.Nack(err)
		} else {
			err = job.Ack()
		}
		if err != nil {
			log.Errorf("[job] %s: error while acking job done %s (%s)",
				workerID, infos.ID, err.Error())
		}
	}
}

func (w *Worker) defaultedConf(opts *JobOptions) *WorkerConfig {
	c := w.Conf.clone()
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
	if opts == nil {
		return c
	}
	if opts.MaxExecCount != 0 && opts.MaxExecCount < c.MaxExecCount {
		c.MaxExecCount = opts.MaxExecCount
	}
	if opts.MaxExecTime > 0 && opts.MaxExecTime < c.MaxExecTime {
		c.MaxExecTime = opts.MaxExecTime
	}
	if opts.Timeout > 0 && opts.Timeout < c.Timeout {
		c.Timeout = opts.Timeout
	}
	return c
}

// Stop will stop the worker's consumption of its queue. It will also close the
// associated queue.
func (w *Worker) Stop() {
	if !atomic.CompareAndSwapInt32(&w.started, 1, 0) {
		return
	}
	w.jobs.Close()
}

type task struct {
	ctx   context.Context
	infos *JobInfos
	conf  *WorkerConfig

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
			log.Warnf("[job] %s: %s (retry in %s)", t.infos.ID, err.Error(), delay)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		log.Debugf("[job] %s: run %d (timeout %s)", t.infos.ID, t.execCount, timeout)
		ctx, cancel := context.WithTimeout(t.ctx, timeout)
		if err = t.exec(ctx); err == nil {
			cancel()
			break
		}
		// Even though ctx should have expired already, it is good practice to call
		// its cancelation function in any case. Failure to do so may keep the
		// context and its parent alive longer than necessary.
		cancel()
		t.execCount++
	}
	return nil
}

func (t *task) exec(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	return t.conf.WorkerFunc(ctx, t.infos.Message)
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
		timeout = c.MaxExecTime - execTime
	}

	var nextDelay time.Duration
	if t.execCount == 0 {
		// on first execution, execute immediately
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
