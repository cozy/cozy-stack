package jobs

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	multierror "github.com/hashicorp/go-multierror"
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
		nbWorkers int
		queues    map[string]*memQueue
		workers   []*Worker
		running   uint32
		closed    chan struct{}
	}
)

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
func NewMemBroker(nbWorkers int) Broker {
	return &memBroker{
		nbWorkers: nbWorkers,
		queues:    make(map[string]*memQueue),
		closed:    make(chan struct{}),
	}
}

func (b *memBroker) Start(ws WorkersList) error {
	if !atomic.CompareAndSwapUint32(&b.running, 0, 1) {
		return ErrClosed
	}
	if b.nbWorkers <= 0 {
		return nil
	}
	joblog.Infof("Starting in-memory broker with %d workers", b.nbWorkers)
	setNbSlots(b.nbWorkers)
	for workerType, conf := range ws {
		q := newMemQueue(workerType)
		w := &Worker{
			Type: workerType,
			Conf: conf,
		}
		b.queues[workerType] = q
		b.workers = append(b.workers, w)
		if err := w.Start(q.Jobs); err != nil {
			return err
		}
	}
	return nil
}

func (b *memBroker) Shutdown(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32(&b.running, 1, 0) {
		return ErrClosed
	}
	fmt.Print("  shutting down in-memory broker...")
	errs := make(chan error)
	for _, w := range b.workers {
		go func(w *Worker) { errs <- w.Shutdown(ctx) }(w)
	}
	var errm error
	for i := 0; i < len(b.workers); i++ {
		if err := <-errs; err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	if errm != nil {
		fmt.Println("failed:", errm)
	} else {
		fmt.Println("ok.")
	}
	return errm
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *memBroker) PushJob(req *JobRequest) (*JobInfos, error) {
	if atomic.LoadUint32(&b.running) == 0 {
		return nil, ErrClosed
	}
	workerType := req.WorkerType
	q, ok := b.queues[workerType]
	if !ok {
		return nil, ErrUnknownWorker
	}
	infos := NewJobInfos(req)
	storage := newCouchStorage(req.Domain)
	j := Job{
		infos:   infos,
		storage: storage,
	}
	if err := storage.Create(infos); err != nil {
		return nil, err
	}
	if err := q.Enqueue(j); err != nil {
		return nil, err
	}
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
	return newCouchStorage(domain).Get(jobID)
}

var (
	_ Broker = &memBroker{}
)
