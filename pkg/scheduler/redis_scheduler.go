package scheduler

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/go-redis/redis"
)

// SchedKey is the the key of the sorted set in redis used for scheduling triggers
const SchedKey = "sched"

// RedisScheduler is a centralized scheduler of many triggers. It starts all of
// them and schedules jobs accordingly.
type RedisScheduler struct {
	broker jobs.Broker
	client *redis.Client
}

// NewRedisScheduler creates a new scheduler that use redis to synchronize with
// other cozy-stack processes to schedule jobs.
func NewRedisScheduler(client *redis.Client) *RedisScheduler {
	return &RedisScheduler{
		client: client,
	}
}

// Start ...
func (s *RedisScheduler) Start(b jobs.Broker) error {
	s.broker = b
	return nil
}

// Add ...
func (s *RedisScheduler) Add(t Trigger) error {
	infos := t.Infos()
	db := couchdb.SimpleDatabasePrefix(infos.Domain)
	if err := couchdb.CreateDoc(db, infos); err != nil {
		return err
	}
	return s.client.ZAddNX(SchedKey, redis.Z{
		Score:  1, // FIXME
		Member: t.ID(),
	}).Err()
}

// Get ...
func (s *RedisScheduler) Get(domain, id string) (Trigger, error) {
	var infos *TriggerInfos
	db := couchdb.SimpleDatabasePrefix(domain)
	if err := couchdb.GetDoc(db, consts.Triggers, id, infos); err != nil {
		return nil, err
	}
	return NewTrigger(infos)
}

// Delete ...
func (s *RedisScheduler) Delete(domain, id string) error {
	t, err := s.Get(domain, id)
	if err != nil {
		return err
	}
	db := couchdb.SimpleDatabasePrefix(domain)
	if err = couchdb.DeleteDoc(db, t.Infos()); err != nil {
		return err
	}
	return s.client.ZRem(SchedKey, id).Err()
}

// GetAll ...
func (s *RedisScheduler) GetAll(domain string) ([]Trigger, error) {
	var infos []*TriggerInfos
	db := couchdb.SimpleDatabasePrefix(domain)
	// TODO(pagination): use a sort of couchdb.WalkDocs function when available.
	req := &couchdb.AllDocsRequest{Limit: 1000}
	err := couchdb.GetAllDocs(db, consts.Triggers, req, &infos)
	if err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return nil, nil
		}
		return nil, err
	}
	v := make([]Trigger, len(infos))
	for _, info := range infos {
		t, err := NewTrigger(info)
		if err != nil {
			return nil, err
		}
		v = append(v, t)
	}
	return v, nil
}

var _ Scheduler = &RedisScheduler{}
