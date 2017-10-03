package jobs

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// contextKey are the keys used in the worker context
type contextKey int

const (
	// ContextDomainKey is used to store the domain string name
	ContextDomainKey contextKey = iota
	// ContextWorkerKey is used to store the workerID string
	ContextWorkerKey
)

var (
	defaultConcurrency  = runtime.NumCPU()
	defaultMaxExecCount = 3
	defaultMaxExecTime  = 60 * time.Second
	defaultRetryDelay   = 60 * time.Millisecond
	defaultTimeout      = 10 * time.Second
)

type (
	// WorkerInitFunc is optionally called at the beginning of the process and
	// can produce a cookie value that will be passed into the given
	// WorkerThreaderFunc.
	WorkerInitFunc func() (interface{}, error)

	// WorkerFunc represent the work function that a worker should implement.
	WorkerFunc func(context context.Context, msg Message) error

	// WorkerThreadedFunc represent the work function that a worker should
	// implement. In addition to a simple WorkerFunc, a threaded one can thread
	// its context on each call and to the commit method.
	WorkerThreadedFunc func(context context.Context, cookie interface{}, msg Message) error

	// WorkerCommit is an optional method that is always called once after the
	// execution of the WorkerFunc.
	WorkerCommit func(context context.Context, cookie interface{}, msg Message, errjob error) error

	// WorkerConfig is the configuration parameter of a worker defined by the job
	// system. It contains parameters of the worker along with the worker main
	// function that perform the work against a job's message.
	WorkerConfig struct {
		WorkerInit         WorkerInitFunc
		WorkerFunc         WorkerFunc
		WorkerThreadedFunc WorkerThreadedFunc
		WorkerCommit       WorkerCommit
		Concurrency        int           `json:"concurrency"`
		MaxExecCount       int           `json:"max_exec_count"`
		MaxExecTime        time.Duration `json:"max_exec_time"`
		Timeout            time.Duration `json:"timeout"`
		RetryDelay         time.Duration `json:"retry_delay"`
	}

	// Worker is a unit of work that will consume from a queue and execute the do
	// method for each jobs it pulls.
	Worker struct {
		Type    string
		Conf    *WorkerConfig
		jobs    chan *Job
		running uint32
		closed  chan struct{}
	}
)

var slots chan struct{}

func setNbSlots(nb int) {
	slots = make(chan struct{}, nb)
	for i := 0; i < nb; i++ {
		slots <- struct{}{}
	}
}

// NewWorkerContext returns a context.Context usable by a worker.
func NewWorkerContext(domain, workerID string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ContextDomainKey, domain)
	ctx = context.WithValue(ctx, ContextWorkerKey, workerID)
	return ctx
}

// Start is used to start the worker consumption of messages from its queue.
func (w *Worker) Start(jobs chan *Job) error {
	if !atomic.CompareAndSwapUint32(&w.running, 0, 1) {
		return ErrClosed
	}
	w.jobs = jobs
	w.closed = make(chan struct{})
	for i := 0; i < w.Conf.Concurrency; i++ {
		name := fmt.Sprintf("%s/%d", w.Type, i)
		joblog.Debugf("Start worker %s", name)
		go w.work(name, w.closed)
	}
	return nil
}

// Shutdown is used to close the worker, waiting for all tasks to end
func (w *Worker) Shutdown(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32(&w.running, 1, 0) {
		return ErrClosed
	}
	close(w.jobs)
	for i := 0; i < w.Conf.Concurrency; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.closed:
		}
	}
	return nil
}

func (w *Worker) work(workerID string, closed chan<- struct{}) {
	for job := range w.jobs {
		domain := job.Domain
		if domain == "" {
			joblog.Errorf("[job] %s: missing domain from job request", workerID)
			continue
		}
		parentCtx := NewWorkerContext(domain, workerID)
		if err := job.AckConsumed(); err != nil {
			joblog.Errorf("[job] %s: error acking consume job %s: %s",
				workerID, job.ID(), err.Error())
			continue
		}
		t := &task{
			ctx:      parentCtx,
			job:      job,
			conf:     w.defaultedConf(job.Options),
			workerID: workerID,
		}
		var err error
		if err = t.run(); err != nil {
			joblog.Errorf("[job] %s: error while performing job %s: %s",
				workerID, job.ID(), err.Error())
			err = job.Nack(err)
		} else {
			err = job.Ack()
		}
		if err != nil {
			joblog.Errorf("[job] %s: error while acking job done %s: %s",
				workerID, job.ID(), err.Error())
		}
	}
	joblog.Debugf("[job] %s: worker shut down", workerID)
	closed <- struct{}{}
}

func (w *Worker) defaultedConf(opts *JobOptions) *WorkerConfig {
	c := w.Conf.Clone()
	if c.Concurrency == 0 {
		c.Concurrency = defaultConcurrency
	}
	if c.MaxExecCount == 0 {
		c.MaxExecCount = defaultMaxExecCount
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

type task struct {
	ctx  context.Context
	job  *Job
	conf *WorkerConfig

	workerID  string
	startTime time.Time
	execCount int
}

func (t *task) run() (err error) {
	var cookie interface{}
	t.startTime = time.Now()
	t.execCount = 0
	if t.conf.WorkerInit != nil {
		cookie, err = t.conf.WorkerInit()
		if err != nil {
			return err
		}
	}
	defer func() {
		if t.conf.WorkerCommit != nil {
			if errc := t.conf.WorkerCommit(t.ctx, cookie, t.job.Message, err); errc != nil {
				joblog.Warnf("[job] %s: error while commiting job %s: %s",
					t.workerID, t.job.ID(), errc.Error())
			}
		}
	}()
	for {
		retry, delay, timeout := t.nextDelay()
		if !retry {
			return err
		}
		if err != nil {
			joblog.Warnf("[job] %s: error while performing job %s: %s (retry in %s)",
				t.workerID, t.job.ID(), err.Error(), delay)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		joblog.Debugf("[job] %s: executing job %s(%d) (timeout %s)",
			t.workerID, t.job.ID(), t.execCount, timeout)
		ctx, cancel := context.WithTimeout(t.ctx, timeout)
		if err = t.exec(ctx, cookie); err == nil {
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

func (t *task) exec(ctx context.Context, cookie interface{}) (err error) {
	slot := <-slots
	defer func() {
		slots <- slot
		if r := recover(); r != nil {
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
			joblog.Errorf("%s: %s", r, debug.Stack())
		}
	}()
	if t.conf.WorkerThreadedFunc != nil {
		return t.conf.WorkerThreadedFunc(ctx, cookie, t.job.Message)
	}
	return t.conf.WorkerFunc(ctx, t.job.Message)
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
		nextDelay = c.RetryDelay << uint(t.execCount-1)

		// fuzzDelay number between delay * (1 +/- 0.1)
		fuzzDelay := int(0.1 * float64(nextDelay))
		nextDelay = nextDelay + time.Duration((rand.Intn(2*fuzzDelay) - fuzzDelay))
	}

	if execTime+nextDelay > c.MaxExecTime {
		return false, 0, 0
	}

	return true, nextDelay, timeout
}
