package jobs

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

var (
	memBrokers   map[string]*MemBroker
	memBrokersMu sync.Mutex
)

type (
	// MemQueue is a queue in-memory implementation of the Queue interface.
	MemQueue struct {
		MaxCapacity int

		jobs *list.List
		run  bool
		jmu  sync.RWMutex

		ch chan Job
		cl chan bool
	}

	// MemBroker is an in-memory broker implementation of the Broker interface.
	MemBroker struct {
		domain string
		queues map[string]*MemQueue
	}

	// MemJob struct contains all the parameters of a job.
	MemJob struct {
		infos *JobInfos
		infmu sync.RWMutex
		jobch chan *JobInfos
	}
)

// NewMemQueue creates and a new in-memory queue.
func NewMemQueue(domain, workerType string) *MemQueue {
	return &MemQueue{
		jobs: list.New(),
		ch:   make(chan Job),
		cl:   make(chan bool),
	}
}

// Enqueue into the queue
func (q *MemQueue) Enqueue(job Job) error {
	q.jmu.Lock()
	defer q.jmu.Unlock()
	q.jobs.PushBack(job)
	if !q.run {
		q.run = true
		go q.send()
	}
	return nil
}

func (q *MemQueue) send() {
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
func (q *MemQueue) Consume() (Job, error) {
	select {
	case job := <-q.ch:
		return job, nil
	case <-q.cl:
		return nil, ErrQueueClosed
	}
}

// Len returns the length of the queue
func (q *MemQueue) Len() int {
	q.jmu.RLock()
	defer q.jmu.RUnlock()
	return q.jobs.Len()
}

// Close closes the queue
func (q *MemQueue) Close() {
	close(q.cl)
}

// NewMemBroker creates a new in-memory broker system.
//
// The in-memory implementation of the job system has the specifity that
// workers are actually launched by the broker at its creation.
func NewMemBroker(domain string, ws WorkersList) Broker {
	memBrokersMu.Lock()
	defer memBrokersMu.Unlock()
	if memBrokers == nil {
		memBrokers = make(map[string]*MemBroker)
	}
	b, ok := memBrokers[domain]
	if ok {
		return b
	}
	queues := make(map[string]*MemQueue)
	for workerType, conf := range ws {
		q := NewMemQueue(domain, workerType)
		queues[workerType] = q
		w := &Worker{
			Domain: domain,
			Type:   workerType,
			Conf:   conf,
		}
		w.Start(q)
	}
	b = &MemBroker{
		domain: domain,
		queues: queues,
	}
	memBrokers[domain] = b
	return b
}

// Domain returns the broker's domain
func (b *MemBroker) Domain() string {
	return b.domain
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *MemBroker) PushJob(req *JobRequest) (*JobInfos, <-chan *JobInfos, error) {
	workerType := req.WorkerType
	q, ok := b.queues[workerType]
	if !ok {
		return nil, nil, ErrUnknownWorker
	}
	jobch := make(chan *JobInfos, 2)
	infos := NewJobInfos(req)
	j := &MemJob{
		infos: infos,
		jobch: jobch,
	}
	if err := q.Enqueue(j); err != nil {
		return nil, nil, err
	}
	return infos, jobch, nil
}

// Infos returns the associated job infos
func (j *MemJob) Infos() *JobInfos {
	j.infmu.RLock()
	defer j.infmu.RUnlock()
	return j.infos
}

// AckConsumed sets the job infos state to Running an sends the new job infos
// on the channel.
func (j *MemJob) AckConsumed() error {
	j.infmu.Lock()
	job := *j.infos
	job.StartedAt = time.Now()
	job.State = Running
	j.infos = &job
	j.infmu.Unlock()
	return j.asyncSend(job, false)
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *MemJob) Ack() error {
	j.infmu.Lock()
	job := *j.infos
	job.State = Done
	j.infos = &job
	j.infmu.Unlock()
	return j.asyncSend(job, true)
}

// Nack sets the job infos state to Errored, set the specified error has the
// error field and sends the new job infos on the channel.
func (j *MemJob) Nack(err error) error {
	j.infmu.Lock()
	job := *j.infos
	job.State = Errored
	job.Error = err
	j.infos = &job
	j.infmu.Unlock()
	return j.asyncSend(job, true)
}

func (j *MemJob) asyncSend(job JobInfos, closed bool) error {
	select {
	case j.jobch <- &job:
	default:
	}
	if closed {
		close(j.jobch)
	}
	return nil
}

// Marshal should not be used for a MemJob
func (j *MemJob) Marshal() ([]byte, error) {
	return nil, errors.New("should not be marshaled")
}

// Unmarshal should not be used for a MemJob
func (j *MemJob) Unmarshal() error {
	return errors.New("should not be unmarshaled")
}

var (
	_ Queue  = &MemQueue{}
	_ Broker = &MemBroker{}
	_ Job    = &MemJob{}
)
