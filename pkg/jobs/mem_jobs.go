package jobs

import (
	"container/list"
	"errors"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
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

	// memScheduler is a centralized scheduler of many triggers. It stars all of
	// them and schedules jobs accordingly.
	memScheduler struct {
		broker  Broker
		storage TriggerStorage

		ts map[string]Trigger
		mu sync.RWMutex
	}

	// MemJob struct contains all the parameters of a job.
	MemJob struct {
		infos *JobInfos
		infmu sync.RWMutex
		jobch chan *JobInfos
	}
)

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

// newMemBroker creates a new in-memory broker system.
//
// The in-memory implementation of the job system has the specifity that
// workers are actually launched by the broker at its creation.
func newMemBroker(ws WorkersList) Broker {
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
	j := &MemJob{
		infos: infos,
		jobch: jobch,
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

// Domain returns the associated domain
func (j *MemJob) Domain() string {
	j.infmu.RLock()
	defer j.infmu.RUnlock()
	return j.infos.Domain
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
	return j.asyncSend(&job, false)
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *MemJob) Ack() error {
	j.infmu.Lock()
	job := *j.infos
	job.State = Done
	j.infos = &job
	j.infmu.Unlock()
	return j.asyncSend(&job, true)
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
	return j.asyncSend(&job, true)
}

func (j *MemJob) asyncSend(job *JobInfos, closed bool) error {
	select {
	case j.jobch <- job:
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

// newMemScheduler creates a new in-memory scheduler that will load all
// registered triggers and schedule their work.
func newMemScheduler(storage TriggerStorage) *memScheduler {
	return &memScheduler{
		storage: storage,
		ts:      make(map[string]Trigger),
	}
}

// Start will start the scheduler by actually loading all triggers from the
// scheduler's storage and associate for each of them a go routine in which
// they wait for the trigger send job requests.
func (s *memScheduler) Start(b Broker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, err := s.storage.GetAll()
	if err != nil {
		return err
	}
	s.broker = b
	for _, infos := range ts {
		t, err := NewTrigger(infos)
		if err != nil {
			log.Errorln(
				"[jobs] scheduler: Could not load the trigger %s(%s) at startup: %s",
				infos.Type, infos.ID, err.Error())
			continue
		}
		s.ts[infos.ID] = t
		go s.schedule(t)
	}
	return nil
}

// Add will add a new trigger to the scheduler. The trigger is persisted in
// storage.
func (s *memScheduler) Add(t Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.storage.Add(t); err != nil {
		return err
	}
	s.ts[t.Infos().ID] = t
	go s.schedule(t)
	return nil
}

// Get returns the trigger with the specified ID.
func (s *memScheduler) Get(domain, id string) (Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.ts[id]
	if !ok || t.Infos().Domain != domain {
		return nil, ErrNotFoundTrigger
	}
	return t, nil
}

// Delete removes the trigger with the specified ID. The trigger is unscheduled
// and remove from the storage.
func (s *memScheduler) Delete(domain, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.ts[id]
	if !ok || t.Infos().Domain != domain {
		return ErrNotFoundTrigger
	}
	if err := s.storage.Delete(t); err != nil {
		return err
	}
	delete(s.ts, id)
	t.Unschedule()
	return nil
}

// GetAll returns all the running in-memory triggers.
func (s *memScheduler) GetAll(domain string) ([]Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := make([]Trigger, 0)
	for _, t := range s.ts {
		if t.Infos().Domain == domain {
			v = append(v, t)
		}
	}
	return v, nil
}

func (s *memScheduler) schedule(t Trigger) {
	log.Debugf("[jobs] trigger %s(%s): Starting trigger", t.Type(), t.Infos().ID)
	for req := range t.Schedule() {
		log.Debugf("[jobs] trigger %s(%s): Pushing new job", t.Type(), t.Infos().ID)
		if _, _, err := s.broker.PushJob(req); err != nil {
			log.Errorf("[jobs] trigger %s(%s): Could not schedule a new job: %s", t.Type(), t.Infos().ID, err.Error())
		}
	}
	log.Debugf("[jobs] trigger %s(%s): Closing trigger", t.Type(), t.Infos().ID)
	if err := s.Delete(t.Infos().Domain, t.Infos().ID); err != nil {
		log.Errorf("[jobs] trigger %s(%s): Could not delete trigger: %s", t.Type(), t.Infos().ID, err.Error())
	}
}

var (
	_ Queue     = &memQueue{}
	_ Broker    = &memBroker{}
	_ Job       = &MemJob{}
	_ Scheduler = &memScheduler{}
)
