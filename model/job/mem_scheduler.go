package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/sirupsen/logrus"
)

// memScheduler is a centralized scheduler of many triggers. It starts all of
// them and schedules jobs accordingly.
type memScheduler struct {
	broker Broker

	ts  map[string]Trigger
	mu  sync.RWMutex
	log *logrus.Entry
}

// NewMemScheduler creates a new in-memory scheduler that will load all
// registered triggers and schedule their work.
func NewMemScheduler() Scheduler {
	return newMemScheduler()
}

func newMemScheduler() *memScheduler {
	return &memScheduler{
		ts:  make(map[string]Trigger),
		log: logger.WithNamespace("mem-scheduler"),
	}
}

type inst struct {
	Domain string `json:"domain"`
	Prefix string `json:"prefix"`
}

func (i inst) DBPrefix() string {
	if i.Prefix != "" {
		return i.Prefix
	}
	return i.Domain
}

func (i inst) DomainName() string {
	return i.Domain
}

// StartScheduler will start the scheduler by actually loading all triggers
// from the scheduler's storage and associate for each of them a go routine in
// which they wait for the trigger send job requests.
func (s *memScheduler) StartScheduler(b Broker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ts []*TriggerInfos
	err := couchdb.ForeachDocs(couchdb.GlobalDB, consts.Instances, func(_ string, data json.RawMessage) error {
		var db inst
		if err := json.Unmarshal(data, &db); err != nil {
			return err
		}
		err := couchdb.ForeachDocs(db, consts.Triggers, func(_ string, data json.RawMessage) error {
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
		t, err := fromTriggerInfos(infos)
		if err != nil {
			joblog.Errorf("Could not start trigger with ID %s: %s",
				infos.ID(), err.Error())
			continue
		}
		s.ts[t.DBPrefix()+"/"+infos.TID] = t
		go s.schedule(t)
	}

	return nil
}

// ShutdownScheduler shuts down the scheduling of triggers
func (s *memScheduler) ShutdownScheduler(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Print("  shutting down in-memory scheduler...")
	for _, t := range s.ts {
		t.Unschedule()
	}
	fmt.Println("ok.")
	return nil
}

// AddTrigger will add a new trigger to the scheduler. The trigger is persisted
// in storage.
func (s *memScheduler) AddTrigger(t Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := couchdb.CreateDoc(t, t.Infos()); err != nil {
		return err
	}
	s.ts[t.DBPrefix()+"/"+t.Infos().TID] = t
	go s.schedule(t)
	return nil
}

// GetTrigger returns the trigger with the specified ID.
func (s *memScheduler) GetTrigger(db prefixer.Prefixer, id string) (Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.ts[db.DBPrefix()+"/"+id]
	if !ok {
		return nil, ErrNotFoundTrigger
	}
	return t, nil
}

// DeleteTrigger removes the trigger with the specified ID. The trigger is unscheduled
// and remove from the storage.
func (s *memScheduler) DeleteTrigger(db prefixer.Prefixer, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.ts[db.DBPrefix()+"/"+id]
	if !ok {
		return ErrNotFoundTrigger
	}
	delete(s.ts, db.DBPrefix()+"/"+id)
	t.Unschedule()
	return couchdb.DeleteDoc(db, t.Infos())
}

// GetAllTriggers returns all the running in-memory triggers.
func (s *memScheduler) GetAllTriggers(db prefixer.Prefixer) ([]Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := db.DBPrefix() + "/"
	v := make([]Trigger, 0)
	for n, t := range s.ts {
		if strings.HasPrefix(n, prefix) {
			v = append(v, t)
		}
	}
	return v, nil
}

func (s *memScheduler) schedule(t Trigger) {
	s.log.Debugf("trigger %s(%s): Starting trigger",
		t.Type(), t.Infos().TID)
	ch := t.Schedule()
	var debounced <-chan time.Time
	var originalReq *JobRequest
	var d time.Duration
	infos := t.Infos()
	if infos.Debounce != "" {
		var err error
		if d, err = time.ParseDuration(infos.Debounce); err != nil {
			s.log.Errorf("trigger %s has an invalid debounce: %s",
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

func (s *memScheduler) pushJob(t Trigger, req *JobRequest) {
	log := s.log.WithField("domain", t.DomainName())
	log.Infof("trigger %s(%s): Pushing new job %s",
		t.Type(), t.Infos().TID, req.WorkerType)
	if _, err := s.broker.PushJob(t, req); err != nil {
		log.Errorf("trigger %s(%s): Could not schedule a new job: %s",
			t.Type(), t.Infos().TID, err.Error())
	}
}

func (s *memScheduler) PollScheduler(now int64) error {
	return errors.New("memScheduler cannot be polled")
}

// CleanRedis does nothing for the in memory scheduler. It's just
// here to implement the Scheduler interface.
func (s *memScheduler) CleanRedis() error {
	return errors.New("memScheduler does not use redis")
}

// RebuildRedis does nothing for the in memory scheduler. It's just
// here to implement the Scheduler interface.
func (s *memScheduler) RebuildRedis(db prefixer.Prefixer) error {
	return errors.New("memScheduler does not use redis")
}

var _ Scheduler = &memScheduler{}
