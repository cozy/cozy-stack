package jobs

import (
	"container/list"
	"sync"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

type (
	// memQueue is a queue in-memory implementation of the Queue interface.
	memQueue struct {
		MaxCapacity int
		Jobs        chan Job

		list *list.List
		run  bool
		jmu  sync.RWMutex
	}

	// memBroker is an in-memory broker implementation of the Broker interface.
	memBroker struct {
		queues map[string]*memQueue
	}
)

// globalStorage is the global job persistence layer used thoughout the stack.
var globalStorage = &couchStorage{couchdb.GlobalJobsDB}

// newMemQueue creates and a new in-memory queue.
func newMemQueue(workerType string) *memQueue {
	return &memQueue{
		list: list.New(),
		Jobs: make(chan Job),
	}
}

// Enqueue into the queue
func (q *memQueue) Enqueue(job Job) error {
	q.jmu.Lock()
	defer q.jmu.Unlock()
	q.list.PushBack(job)
	if !q.run {
		q.run = true
		go q.send()
	}
	return nil
}

func (q *memQueue) send() {
	for {
		q.jmu.Lock()
		e := q.list.Front()
		if e == nil {
			q.run = false
			q.jmu.Unlock()
			return
		}
		q.list.Remove(e)
		q.jmu.Unlock()
		q.Jobs <- e.Value.(Job)
	}
}

// Len returns the length of the queue
func (q *memQueue) Len() int {
	q.jmu.RLock()
	defer q.jmu.RUnlock()
	return q.list.Len()
}

// NewMemBroker creates a new in-memory broker system.
//
// The in-memory implementation of the job system has the specifity that
// workers are actually launched by the broker at its creation.
func NewMemBroker(nbWorkers int, ws WorkersList) Broker {
	setNbSlots(nbWorkers)
	queues := make(map[string]*memQueue)
	for workerType, conf := range ws {
		q := newMemQueue(workerType)
		queues[workerType] = q
		w := &Worker{
			Type: workerType,
			Conf: conf,
		}
		w.Start(q.Jobs)
	}
	return &memBroker{queues: queues}
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *memBroker) PushJob(req *JobRequest) (*JobInfos, error) {
	workerType := req.WorkerType
	q, ok := b.queues[workerType]
	if !ok {
		return nil, ErrUnknownWorker
	}
	infos := NewJobInfos(req)
	j := Job{
		infos:   infos,
		storage: globalStorage,
	}
	if err := globalStorage.Create(infos); err != nil {
		return nil, err
	}
	if err := q.Enqueue(j); err != nil {
		return nil, err
	}
	// Writing in couchdb should be enough to publish this event,
	// but it is not published on right domain, so we publish it again.
	realtime.GetHub().Publish(&realtime.Event{
		Verb:   realtime.EventCreate,
		Doc:    infos.Clone(),
		Domain: infos.Domain,
	})
	return infos, nil
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

var (
	_ Broker = &memBroker{}
)
