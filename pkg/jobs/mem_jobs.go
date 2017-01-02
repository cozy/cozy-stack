package jobs

import (
	"container/list"
	"sync"
	"time"
)

var (
	memQueues   map[string]*MemQueue
	memQueuesMu sync.Mutex

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

		ch chan *Job
		cl chan bool
	}

	// MemBroker is an in-memory broker implementation of the Broker interface.
	MemBroker struct {
		domain string
		queues map[string]*MemQueue
	}
)

// NewMemQueue creates and a new in-memory queue.
func NewMemQueue(domain, workerType string) *MemQueue {
	name := makeQueueName(domain, workerType)
	memQueuesMu.Lock()
	defer memQueuesMu.Unlock()
	if memQueues == nil {
		memQueues = make(map[string]*MemQueue)
	}
	q, ok := memQueues[name]
	if ok {
		return q
	}
	q = &MemQueue{
		jobs: list.New(),
		ch:   make(chan *Job),
		cl:   make(chan bool),
	}
	memQueues[name] = q
	return q
}

// Enqueue into the queue
func (q *MemQueue) Enqueue(job *Job) error {
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
		case q.ch <- e.Value.(*Job):
			continue
		case <-q.cl:
			return
		}
	}
}

// Consume from the queue
func (q *MemQueue) Consume() (*Job, error) {
	var job *Job
	select {
	case job = <-q.ch:
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
	for workerType, wconf := range ws {
		q := NewMemQueue(domain, workerType)
		queues[workerType] = q
		w := &Worker{
			Domain:      domain,
			Type:        workerType,
			Concurrency: wconf.Concurrency,
			Func:        wconf.WorkerFunc,

			q: q,
		}
		w.Start()
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
func (b *MemBroker) PushJob(req *JobRequest) (*Job, error) {
	q, ok := b.queues[req.WorkerType]
	if !ok {
		return nil, ErrUnknownWorker
	}
	j := &Job{
		ID:         makeID(),
		WorkerType: req.WorkerType,
		Message:    req.Message,
		State:      Queued,
		QueuedAt:   time.Now(),
	}
	if err := q.Enqueue(j); err != nil {
		return nil, err
	}
	return j, nil
}

var (
	_ Queue  = &MemQueue{}
	_ Broker = &MemBroker{}
)
