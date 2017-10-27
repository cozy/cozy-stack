package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/sirupsen/logrus"
)

// MemScheduler is a centralized scheduler of many triggers. It starts all of
// them and schedules jobs accordingly.
type MemScheduler struct {
	broker jobs.Broker

	ts  map[string]Trigger
	mu  sync.RWMutex
	log *logrus.Entry
}

// NewMemScheduler creates a new in-memory scheduler that will load all
// registered triggers and schedule their work.
func NewMemScheduler() Scheduler {
	return newMemScheduler()
}

func newMemScheduler() *MemScheduler {
	return &MemScheduler{
		ts:  make(map[string]Trigger),
		log: logger.WithNamespace("mem-scheduler"),
	}
}

// Start will start the scheduler by actually loading all triggers from the
// scheduler's storage and associate for each of them a go routine in which
// they wait for the trigger send job requests.
func (s *MemScheduler) Start(b jobs.Broker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ts []*TriggerInfos
	err := couchdb.ForeachDocs(couchdb.GlobalDB, consts.Instances, func(data []byte) error {
		var d struct {
			Domain string `json:"domain"`
		}
		if err := json.Unmarshal(data, &d); err != nil {
			return err
		}
		db := couchdb.SimpleDatabasePrefix(d.Domain)
		err := couchdb.ForeachDocs(db, consts.Triggers, func(data []byte) error {
			var t *TriggerInfos
			if err := json.Unmarshal(data, &t); err != nil {
				return err
			}
			ts = append(ts, t)
			return nil
		})
		if err != nil && !couchdb.IsNoDatabaseError(err) {
			return err
		}
		return nil
	})
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return err
	}

	s.broker = b

	for _, infos := range ts {
		t, err := NewTrigger(infos)
		if err != nil {
			s.log.Errorf("[scheduler] scheduler: Could not load the trigger %s(%s) at startup: %s",
				infos.Type, infos.TID, err.Error())
			continue
		}
		s.ts[infos.TID] = t
		go s.schedule(t)
	}

	return nil
}

// Shutdown the scheduling of triggers
func (s *MemScheduler) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Print("  shutting down in-memory scheduler...")
	for _, t := range s.ts {
		t.Unschedule()
	}
	fmt.Println("ok.")
	return nil
}

// Add will add a new trigger to the scheduler. The trigger is persisted in
// storage.
func (s *MemScheduler) Add(t Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	db := couchdb.SimpleDatabasePrefix(t.Infos().Domain)
	if err := couchdb.CreateDoc(db, t.Infos()); err != nil {
		return err
	}
	s.ts[t.Infos().TID] = t
	go s.schedule(t)
	return nil
}

// Get returns the trigger with the specified ID.
func (s *MemScheduler) Get(domain, id string) (Trigger, error) {
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
func (s *MemScheduler) Delete(domain, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.ts[id]
	if !ok || t.Infos().Domain != domain {
		return ErrNotFoundTrigger
	}
	delete(s.ts, id)
	t.Unschedule()
	db := couchdb.SimpleDatabasePrefix(t.Infos().Domain)
	return couchdb.DeleteDoc(db, t.Infos())
}

// GetAll returns all the running in-memory triggers.
func (s *MemScheduler) GetAll(domain string) ([]Trigger, error) {
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

func (s *MemScheduler) schedule(t Trigger) {
	s.log.Debugf("[scheduler] trigger %s(%s): Starting trigger",
		t.Type(), t.Infos().TID)
	ch := t.Schedule()
	var debounced <-chan time.Time
	var originalReq *jobs.JobRequest
	var d time.Duration
	infos := t.Infos()
	if infos.Debounce != "" {
		var err error
		if d, err = time.ParseDuration(infos.Debounce); err != nil {
			s.log.Infof("[scheduler] trigger %s has an invalid debounce: %s",
				infos.TID, infos.Debounce)
		}
	}
	for {
		select {
		case req, ok := <-ch:
			if !ok {
				return
			}
			if d == 0 {
				s.pushJob(t, req)
			} else if debounced == nil {
				debounced = time.After(d)
				originalReq = req
			}
		case <-debounced:
			s.pushJob(t, originalReq)
			debounced = nil
			originalReq = nil
		}
	}
}

func (s *MemScheduler) pushJob(t Trigger, req *jobs.JobRequest) {
	log := s.log.WithField("domain", req.Domain)
	log.Infof(
		"[scheduler] trigger %s(%s): Pushing new job %s",
		t.Type(), t.Infos().TID, req.WorkerType)
	if _, err := s.broker.PushJob(req); err != nil {
		log.Errorf("[scheduler] trigger %s(%s): Could not schedule a new job: %s",
			t.Type(), t.Infos().TID, err.Error())
	}
}

// RebuildRedis does nothing for the in memory scheduler. It's just here to
// implement the Scheduler interface.
func (s *MemScheduler) RebuildRedis(domain string) error {
	return errors.New("MemScheduler does not use redis")
}

var _ Scheduler = &MemScheduler{}
