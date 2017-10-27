package jobs

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/sirupsen/logrus"
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
	// can produce a context value.
	WorkerInitFunc func(ctx *WorkerContext) (*WorkerContext, error)

	// WorkerFunc represent the work function that a worker should implement.
	WorkerFunc func(ctx *WorkerContext) error

	// WorkerCommit is an optional method that is always called once after the
	// execution of the WorkerFunc.
	WorkerCommit func(ctx *WorkerContext, errjob error) error

	// WorkerConfig is the configuration parameter of a worker defined by the job
	// system. It contains parameters of the worker along with the worker main
	// function that perform the work against a job's message.
	WorkerConfig struct {
		WorkerInit   WorkerInitFunc
		WorkerFunc   WorkerFunc
		WorkerCommit WorkerCommit
		Concurrency  int           `json:"concurrency"`
		MaxExecCount int           `json:"max_exec_count"`
		MaxExecTime  time.Duration `json:"max_exec_time"`
		Timeout      time.Duration `json:"timeout"`
		RetryDelay   time.Duration `json:"retry_delay"`
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

	// WorkerContext is a context.Context passed to the worker for each job
	// execution and contains specific values from the job.
	WorkerContext struct {
		context.Context
		job    *Job
		evt    Event
		log    *logrus.Entry
		cookie interface{}
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
func NewWorkerContext(workerID string, job *Job) *WorkerContext {
	ctx := context.Background()
	return &WorkerContext{
		Context: ctx,
		job:     job,
		log:     logger.WithDomain(job.Domain).WithField("worker_id", workerID),
	}
}

// NewWorkerContextWithEvent returns a context.Context usable by a worker. It
// returns the same context as NewWorkerContext except that it also includes
// the event responsible for the job, from a @event trigger for instance.
func NewWorkerContextWithEvent(workerID string, job *Job, event Event) *WorkerContext {
	ctx := NewWorkerContext(workerID, job)
	ctx.evt = event
	return ctx
}

// WithTimeout returns a clone of the context with a different deadline.
func (c *WorkerContext) WithTimeout(timeout time.Duration) (*WorkerContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.Context, timeout)
	newCtx := c.clone()
	newCtx.Context = ctx
	return newCtx, cancel
}

// WithCookie returns a clone of the context with a new cookie value.
func (c *WorkerContext) WithCookie(cookie interface{}) *WorkerContext {
	newCtx := c.clone()
	newCtx.cookie = cookie
	return newCtx
}

func (c *WorkerContext) clone() *WorkerContext {
	return &WorkerContext{
		Context: c.Context,
		job:     c.job,
		evt:     c.evt,
		log:     c.log,
		cookie:  c.cookie,
	}
}

// Logger return the logger associated with the worker context.
func (c *WorkerContext) Logger() *logrus.Entry {
	return c.log
}

// UnmarshalMessage unmarshals the message contained in the worker context.
func (c *WorkerContext) UnmarshalMessage(v interface{}) error {
	return c.job.Message.Unmarshal(v)
}

// UnmarshalEvent unmarshals the event contained in the worker context.
func (c *WorkerContext) UnmarshalEvent(v interface{}) error {
	if c.evt == nil {
		return errors.New("jobs: does not have an event associated")
	}
	return c.evt.Unmarshal(v)
}

// Domain returns the domain associated with the worker context.
func (c *WorkerContext) Domain() string {
	return c.job.Domain
}

// TriggerID returns the possible trigger identifier responsible for launching
// the job.
func (c *WorkerContext) TriggerID() (string, bool) {
	triggerID := c.job.TriggerID
	return triggerID, triggerID != ""
}

// Cookie returns the cookie associated with the worker context.
func (c *WorkerContext) Cookie() interface{} {
	return c.cookie
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
		var parentCtx *WorkerContext
		if event := job.Event; event != nil {
			parentCtx = NewWorkerContextWithEvent(workerID, job, event)
		} else {
			parentCtx = NewWorkerContext(workerID, job)
		}
		if err := job.AckConsumed(); err != nil {
			parentCtx.Logger().Errorf("[job] error acking consume job: %s",
				err.Error())
			continue
		}
		t := &task{
			ctx:  parentCtx,
			job:  job,
			conf: w.defaultedConf(job.Options),
		}
		var err error
		if err = t.run(); err != nil {
			parentCtx.Logger().Errorf("[job] error while performing job: %s",
				err.Error())
			err = job.Nack(err)
		} else {
			err = job.Ack()
		}
		if err != nil {
			parentCtx.Logger().Errorf("[job] error while acking job done: %s",
				err.Error())
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
	ctx  *WorkerContext
	conf *WorkerConfig
	job  *Job

	startTime time.Time
	execCount int
}

func (t *task) run() (err error) {
	t.startTime = time.Now()
	t.execCount = 0
	if t.conf.WorkerInit != nil {
		t.ctx, err = t.conf.WorkerInit(t.ctx)
		if err != nil {
			return err
		}
	}
	defer func() {
		if t.conf.WorkerCommit != nil {
			if errc := t.conf.WorkerCommit(t.ctx, err); errc != nil {
				t.ctx.Logger().Warnf("[job] error while commiting job: %s",
					errc.Error())
			}
		}
	}()
	for {
		retry, delay, timeout := t.nextDelay()
		if !retry {
			return err
		}
		if err != nil {
			t.ctx.Logger().Warnf("[job] error while performing job: %s (retry in %s)",
				err.Error(), delay)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		t.ctx.Logger().Debugf("[job] executing job (%d) (timeout %s)",
			t.execCount, timeout)
		ctx, cancel := t.ctx.WithTimeout(timeout)
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

func (t *task) exec(ctx *WorkerContext) (err error) {
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
	return t.conf.WorkerFunc(ctx)
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
