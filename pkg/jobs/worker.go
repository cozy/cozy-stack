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

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/metrics"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	defaultConcurrency  = runtime.NumCPU()
	defaultMaxExecCount = 1
	defaultRetryDelay   = 60 * time.Millisecond
	defaultTimeout      = 10 * time.Second
)

type (
	// WorkerInitFunc is called at the start of the worker system, only once. It
	// is not called before every job process. It can be useful to initialize a
	// global variable used by the worker.
	WorkerInitFunc func() error

	// WorkerStartFunc is optionally called at the beginning of the each job
	// process and can produce a context value.
	WorkerStartFunc func(ctx *WorkerContext) (*WorkerContext, error)

	// WorkerFunc represent the work function that a worker should implement.
	WorkerFunc func(ctx *WorkerContext) error

	// WorkerCommit is an optional method that is always called once after the
	// execution of the WorkerFunc.
	WorkerCommit func(ctx *WorkerContext, errjob error) error

	// WorkerBeforeHook is an optional method that is always called before the
	// job is being pushed into the queue. It can be useful to skip the job
	// beforehand.
	WorkerBeforeHook func(job *Job) (bool, error)

	// WorkerConfig is the configuration parameter of a worker defined by the job
	// system. It contains parameters of the worker along with the worker main
	// function that perform the work against a job's message.
	WorkerConfig struct {
		WorkerInit   WorkerInitFunc
		WorkerStart  WorkerStartFunc
		WorkerFunc   WorkerFunc
		WorkerCommit WorkerCommit
		WorkerType   string
		BeforeHook   WorkerBeforeHook
		Concurrency  int
		MaxExecCount int
		AdminOnly    bool
		Timeout      time.Duration
		RetryDelay   time.Duration
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
		job     *Job
		log     *logrus.Entry
		id      string
		cookie  interface{}
		noRetry bool
	}
)

var slots chan struct{}

func setNbSlots(nb int) {
	slots = make(chan struct{}, nb)
	for i := 0; i < nb; i++ {
		slots <- struct{}{}
	}
}

// Clone clones the worker config
func (w *WorkerConfig) Clone() *WorkerConfig {
	cloned := *w
	return &cloned
}

// NewWorkerContext returns a context.Context usable by a worker.
func NewWorkerContext(workerID string, job *Job) *WorkerContext {
	ctx := context.Background()
	id := fmt.Sprintf("%s/%s", workerID, job.ID())
	log := logger.WithDomain(job.Domain).
		WithField("job_id", job.ID()).
		WithField("worker_id", workerID).
		WithField("nspace", "jobs")

	if job.ForwardLogs {
		// we need to clone the underlying logger in order to add a specific hook
		// only on this logger.
		loggerClone := logger.Clone(log.Logger)
		loggerClone.AddHook(realtime.LogHook(job, realtime.GetHub(),
			consts.Jobs, job.ID()))
		log.Logger = loggerClone
	}

	return &WorkerContext{
		Context: ctx,
		job:     job,
		log:     log,
		id:      id,
	}
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

// SetNoRetry set the no-retry flag to prevent a retry on the next execution.
func (c *WorkerContext) SetNoRetry() {
	c.noRetry = true
}

// NoRetry returns the no-retry flag.
func (c *WorkerContext) NoRetry() bool {
	return c.noRetry
}

func (c *WorkerContext) clone() *WorkerContext {
	return &WorkerContext{
		Context: c.Context,
		job:     c.job,
		log:     c.log,
		id:      c.id,
		cookie:  c.cookie,
	}
}

// ID returns a unique identifier for the worker context.
func (c *WorkerContext) ID() string {
	return c.id
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
	if c.job == nil || c.job.Event == nil {
		return errors.New("jobs: does not have an event associated")
	}
	return c.job.Event.Unmarshal(v)
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

// NewWorker creates a new instance of Worker with the given configuration.
func NewWorker(conf *WorkerConfig) *Worker {
	return &Worker{
		Type: conf.WorkerType,
		Conf: conf,
	}
}

// Manual returns if the job was started manually
func (c *WorkerContext) Manual() bool {
	return c.job.Manual
}

// Start is used to start the worker consumption of messages from its queue.
func (w *Worker) Start(jobs chan *Job) error {
	if !atomic.CompareAndSwapUint32(&w.running, 0, 1) {
		return ErrClosed
	}
	w.jobs = jobs
	w.closed = make(chan struct{})
	if w.Conf.WorkerInit != nil {
		if err := w.Conf.WorkerInit(); err != nil {
			return fmt.Errorf("Could not start worker %s: %s", w.Type, err)
		}
	}
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
			joblog.Errorf("%s: missing domain from job request", workerID)
			continue
		}
		parentCtx := NewWorkerContext(workerID, job)
		if err := job.AckConsumed(); err != nil {
			parentCtx.Logger().Errorf("error acking consume job: %s",
				err.Error())
			continue
		}
		t := &task{
			w:    w,
			ctx:  parentCtx,
			job:  job,
			conf: w.defaultedConf(job.Options),
		}
		var runResultLabel string
		var errAck error
		errRun := t.run()
		if errRun == ErrAbort {
			errRun = nil
		}
		if errRun != nil {
			parentCtx.Logger().Errorf("error while performing job: %s",
				errRun.Error())
			runResultLabel = metrics.WorkerExecResultErrored
			errAck = job.Nack(errRun)
		} else {
			runResultLabel = metrics.WorkerExecResultSuccess
			errAck = job.Ack()
		}
		metrics.WorkerExecCounter.WithLabelValues(w.Type, runResultLabel).Inc()
		if errAck != nil {
			parentCtx.Logger().Errorf("error while acking job done: %s",
				errAck.Error())
		}

		// Delete the trigger associated with the job (if any) when we receive a
		// ErrBadTrigger.
		//
		// XXX: better retro-action between broker and scheduler to avoid going
		// through the global job-system.
		if job.TriggerID != "" && globalJobSystem != nil {
			if _, ok := errRun.(ErrBadTrigger); ok {
				onBadTriggerError(job)
			}
		}
	}
	joblog.Debugf("%s: worker shut down", workerID)
	closed <- struct{}{}
}

// onBadTriggerError is the handler executed when we receive a specific
// ErrBadTrigger error message:
//   - delete the associated trigger
//   - delete the account document associated with this trigger if any (not activated)
func onBadTriggerError(job *Job) {
	// XXX: the account deletion is not activated for now
	// t, err := globalJobSystem.GetTrigger(job, job.TriggerID)
	// if err != nil {
	// 	return
	// }

	globalJobSystem.DeleteTrigger(job, job.TriggerID)

	// if job.WorkerType != "konnector" {
	// 	return
	// }
	// var msg struct {
	// 	Account string `json:"account"`
	// }
	// if err = t.Infos().Message.Unmarshal(&msg); err == nil && msg.Account != "" {
	// 	doc := couchdb.JSONDoc{Type: consts.Accounts}
	// 	if err = couchdb.GetDoc(job, consts.Accounts, msg.Account, &doc); err == nil {
	// 		couchdb.DeleteDoc(job, &doc)
	// 	}
	// }
}

func (w *Worker) defaultedConf(opts *JobOptions) *WorkerConfig {
	c := w.Conf.Clone()
	if c.Concurrency == 0 {
		c.Concurrency = defaultConcurrency
	}
	if c.MaxExecCount == 0 {
		c.MaxExecCount = defaultMaxExecCount
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
	if opts.Timeout > 0 && opts.Timeout < c.Timeout {
		c.Timeout = opts.Timeout
	}
	return c
}

type task struct {
	w    *Worker
	ctx  *WorkerContext
	conf *WorkerConfig
	job  *Job

	startTime time.Time
	endTime   time.Time
	execCount int
}

func (t *task) run() (err error) {
	t.startTime = time.Now()
	t.execCount = 0

	if t.conf.WorkerStart != nil {
		t.ctx, err = t.conf.WorkerStart(t.ctx)
		if err != nil {
			return err
		}
	}
	defer func() {
		if t.conf.WorkerCommit != nil {
			t.ctx.log = t.ctx.Logger().WithField("exec_time", t.endTime.Sub(t.startTime))
			if errc := t.conf.WorkerCommit(t.ctx, err); errc != nil {
				t.ctx.Logger().Warnf("Error while committing job: %s",
					errc.Error())
			}
		}
	}()
	for {
		retry, delay, timeout := t.nextDelay(err)
		if !retry {
			break
		}
		if err != nil {
			t.ctx.Logger().Warnf("Error while performing job: %s (retry in %s)",
				err.Error(), delay)
		}

		if delay > 0 {
			time.Sleep(delay)
		}

		t.ctx.Logger().Debugf("Executing job (%d) (timeout set to %s)",
			t.execCount, timeout)

		var execResultLabel string
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			metrics.WorkerExecDurations.WithLabelValues(t.w.Type, execResultLabel).Observe(v)
		}))

		ctx, cancel := t.ctx.WithTimeout(timeout)
		err = t.exec(ctx)
		if err == nil {
			execResultLabel = metrics.WorkerExecResultSuccess
			timer.ObserveDuration()
			t.endTime = time.Now()
			cancel()
			break
		}
		execResultLabel = metrics.WorkerExecResultErrored
		timer.ObserveDuration()
		t.endTime = time.Now()

		// Even though ctx should have expired already, it is good practice to call
		// its cancelation function in any case. Failure to do so may keep the
		// context and its parent alive longer than necessary.
		cancel()
		t.execCount++

		if ctx.NoRetry() {
			break
		}
	}

	metrics.WorkerExecRetries.WithLabelValues(t.w.Type).Observe(float64(t.execCount))
	return
}

func (t *task) exec(ctx *WorkerContext) (err error) {
	var slot struct{}
	if slots != nil {
		slot = <-slots
	}
	defer func() {
		if slots != nil {
			slots <- slot
		}
		if r := recover(); r != nil {
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
			ctx.Logger().Errorf("[panic] %s: %s", r, debug.Stack())
		}
	}()
	return t.conf.WorkerFunc(ctx)
}

func (t *task) nextDelay(prevError error) (bool, time.Duration, time.Duration) {
	// for certain kinds of errors, we do not have a retry since these error
	// cannot be recovered from
	{
		if _, ok := prevError.(ErrBadTrigger); ok {
			return false, 0, 0
		}
		switch prevError {
		case ErrAbort, ErrMessageUnmarshal, ErrMessageNil:
			return false, 0, 0
		}
	}

	c := t.conf

	if t.execCount >= c.MaxExecCount {
		return false, 0, 0
	}

	// the worker timeout should take into account the maximum execution time
	// allowed to the task
	timeout := c.Timeout

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

	return true, nextDelay, timeout
}
