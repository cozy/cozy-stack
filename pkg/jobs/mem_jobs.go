package jobs

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

type (
	// memQueue is a queue in-memory implementation of the Queue interface.
	memQueue struct {
		MaxCapacity int

		jobs *list.List
		run  bool
		jmu  sync.RWMutex

		ch chan Job
		cl chan bool
	}

	// memBroker is an in-memory broker implementation of the Broker interface.
	memBroker struct {
		queues map[string]*memQueue
	}

	// memJob struct contains all the parameters of a job.
	memJob struct {
		infos *JobInfos
		infmu sync.RWMutex
		jobch chan *JobInfos
	}
)

// globalStorage is the global job persistence layer used thoughout the stack.
var globalStorage = &couchStorage{couchdb.GlobalJobsDB}

type couchStorage struct {
	db couchdb.Database
}

func (c *couchStorage) Get(domain, jobID string) (*JobInfos, error) {
	var job JobInfos
	if err := couchdb.GetDoc(c.db, consts.Jobs, jobID, &job); err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, ErrNotFoundJob
		}
		return nil, err
	}
	if job.Domain != domain {
		return nil, ErrNotFoundJob
	}
	return &job, nil
}

func (c *couchStorage) Create(job *JobInfos) error {
	return couchdb.CreateDoc(c.db, job)
}

func (c *couchStorage) Update(job *JobInfos) error {
	return couchdb.UpdateDoc(c.db, job)
}

// newMemQueue creates and a new in-memory queue.
func newMemQueue(workerType string) *memQueue {
	return &memQueue{
		jobs: list.New(),
		ch:   make(chan Job),
		cl:   make(chan bool),
	}
}

// Enqueue into the queue
func (q *memQueue) Enqueue(job Job) error {
	q.jmu.Lock()
	defer q.jmu.Unlock()
	q.jobs.PushBack(job)
	if !q.run {
		q.run = true
		go q.send()
	}
	return nil
}

func (q *memQueue) send() {
	for {
		q.jmu.Lock()
		e := q.jobs.Front()
		if e == nil {
			q.run = false
			q.jmu.Unlock()
			return
		}
		q.jobs.Remove(e)
		q.jmu.Unlock()
		select {
		case q.ch <- e.Value.(Job):
			continue
		case <-q.cl:
			return
		}
	}
}

// Consume from the queue
func (q *memQueue) Consume() (Job, error) {
	select {
	case job := <-q.ch:
		return job, nil
	case <-q.cl:
		return nil, ErrQueueClosed
	}
}

// Len returns the length of the queue
func (q *memQueue) Len() int {
	q.jmu.RLock()
	defer q.jmu.RUnlock()
	return q.jobs.Len()
}

// Close closes the queue
func (q *memQueue) Close() {
	close(q.cl)
}

// NewMemBroker creates a new in-memory broker system.
//
// The in-memory implementation of the job system has the specifity that
// workers are actually launched by the broker at its creation.
func NewMemBroker(ws WorkersList) Broker {
	queues := make(map[string]*memQueue)
	for workerType, conf := range ws {
		q := newMemQueue(workerType)
		queues[workerType] = q
		w := &Worker{
			Type: workerType,
			Conf: conf,
		}
		w.Start(q)
	}
	return &memBroker{queues: queues}
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *memBroker) PushJob(req *JobRequest) (*JobInfos, <-chan *JobInfos, error) {
	workerType := req.WorkerType
	q, ok := b.queues[workerType]
	if !ok {
		return nil, nil, ErrUnknownWorker
	}
	jobch := make(chan *JobInfos, 2)
	infos := NewJobInfos(req)
	j := &memJob{
		infos: infos,
		jobch: jobch,
	}
	if err := globalStorage.Create(infos); err != nil {
		return nil, nil, err
	}
	if err := q.Enqueue(j); err != nil {
		return nil, nil, err
	}
	return infos, jobch, nil
}

// QueueLen returns the size of the number of elements in queue of the
// specified worker type.
func (b *memBroker) QueueLen(workerType string) (int, error) {
	q, ok := b.queues[workerType]
	if !ok {
		return 0, ErrUnknownWorker
	}
	return q.Len(), nil
}

// GetJobInfos returns the informations about a job.
func (b *memBroker) GetJobInfos(domain, jobID string) (*JobInfos, error) {
	return globalStorage.Get(domain, jobID)
}

// Domain returns the associated domain
func (j *memJob) Domain() string {
	j.infmu.RLock()
	defer j.infmu.RUnlock()
	return j.infos.Domain
}

// Infos returns the associated job infos
func (j *memJob) Infos() *JobInfos {
	j.infmu.RLock()
	defer j.infmu.RUnlock()
	return j.infos
}

// AckConsumed sets the job infos state to Running an sends the new job infos
// on the channel.
func (j *memJob) AckConsumed() error {
	j.infmu.Lock()
	job := *j.infos
	job.StartedAt = time.Now()
	job.State = Running
	j.infos = &job
	if err := globalStorage.Update(j.infos); err != nil {
		return err
	}
	j.infmu.Unlock()
	return j.asyncSend(&job, false)
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *memJob) Ack() error {
	j.infmu.Lock()
	job := *j.infos
	job.State = Done
	j.infos = &job
	if err := globalStorage.Update(j.infos); err != nil {
		return err
	}
	j.infmu.Unlock()
	return j.asyncSend(&job, true)
}

// Nack sets the job infos state to Errored, set the specified error has the
// error field and sends the new job infos on the channel.
func (j *memJob) Nack(err error) error {
	j.infmu.Lock()
	job := *j.infos
	job.State = Errored
	job.Error = err.Error()
	j.infos = &job
	if err := globalStorage.Update(j.infos); err != nil {
		return err
	}
	j.infmu.Unlock()
	return j.asyncSend(&job, true)
}

func (j *memJob) asyncSend(job *JobInfos, closed bool) error {
	select {
	case j.jobch <- job:
	default:
	}
	if closed {
		close(j.jobch)
	}
	return nil
}

// Marshal should not be used for a memJob
func (j *memJob) Marshal() ([]byte, error) {
	return nil, errors.New("should not be marshaled")
}

// Unmarshal should not be used for a memJob
func (j *memJob) Unmarshal() error {
	return errors.New("should not be unmarshaled")
}

var (
	_ Queue  = &memQueue{}
	_ Broker = &memBroker{}
	_ Job    = &memJob{}
)
