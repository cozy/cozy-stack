package jobs

import (
	"container/list"
	"errors"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
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

	// memJob struct contains all the parameters of a job.
	memJob struct {
		infos *JobInfos
		// No mutex, a memJob is expected to be used from only one goroutine at a time
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
func NewMemBroker(ws WorkersList) Broker {
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
	j := &memJob{
		infos: infos,
	}
	if err := globalStorage.Create(infos); err != nil {
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
	return globalStorage.Get(domain, jobID)
}

// Domain returns the associated domain
func (j *memJob) Domain() string {
	return j.infos.Domain
}

// Infos returns the associated job infos
func (j *memJob) Infos() *JobInfos {
	return j.infos
}

// AckConsumed sets the job infos state to Running an sends the new job infos
// on the channel.
func (j *memJob) AckConsumed() error {
	job := *j.infos
	log.Debugf("[jobs] ack_consume %s ", job.ID())
	job.StartedAt = time.Now()
	job.State = Running
	j.infos = &job
	return j.persist()
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *memJob) Ack() error {
	job := *j.infos
	log.Debugf("[jobs] ack %s ", job.ID())
	job.State = Done
	j.infos = &job
	return j.persist()
}

// Nack sets the job infos state to Errored, set the specified error has the
// error field and sends the new job infos on the channel.
func (j *memJob) Nack(err error) error {
	job := *j.infos
	log.Debugf("[jobs] nack %s ", job.ID())
	job.State = Errored
	job.Error = err.Error()
	j.infos = &job
	return j.persist()
}

func (j *memJob) persist() error {
	return globalStorage.Update(j.infos)
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
	_ Broker = &memBroker{}
	_ Job    = &memJob{}
)
