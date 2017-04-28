package scheduler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/go-redis/redis"
)

// TriggersKey is the the key of the sorted set in redis used for triggers
// waiting to be activated
const TriggersKey = "triggers"

// SchedKey is the the key of the sorted set in redis used for triggers
// currently being executed
const SchedKey = "scheduling"

// luaPoll returns the lua script used for polling triggers in redis
// TODO we should poll sched too
const luaPoll = `
local res = redis.call("ZRANGEBYSCORE", "` + TriggersKey + `", 0, KEYS[1], "WITHSCORES", "LIMIT", 0, 1)
if #res > 0 then
  redis.call("ZREM", "` + TriggersKey + `", res[1])
  redis.call("ZADD", "` + SchedKey + `", res[2], res[1])
end
return res`

// local res = redis.call("ZRANGEBYSCORE", "` + TriggersKey + `", 0, KEYS[1], "WITHSCORES", "LIMIT", 1)
// if #res > 0 then
//   redis.call("ZREM", "` + TriggersKey + `", res[1])
//   redis.call("ZADD", "` + SchedKey + `", res[1], res[2])
// end
// return res

// pollInterval is the time interval between 2 redis polling
const pollInterval = 1 * time.Second

// RedisScheduler is a centralized scheduler of many triggers. It starts all of
// them and schedules jobs accordingly.
type RedisScheduler struct {
	Broker jobs.Broker
	client *redis.Client
}

// NewRedisScheduler creates a new scheduler that use redis to synchronize with
// other cozy-stack processes to schedule jobs.
func NewRedisScheduler(client *redis.Client) *RedisScheduler {
	return &RedisScheduler{
		client: client,
	}
}

func redisKey(infos *TriggerInfos) string {
	return infos.Domain + ":" + infos.TID
}

// Start a goroutine that will fetch triggers in redis to schedule their jobs
func (s *RedisScheduler) Start(b jobs.Broker) error {
	s.Broker = b
	go func() {
		for _ = range time.Tick(pollInterval) {
			if err := s.Poll(); err != nil {
				log.Warnf("[Scheduler] Failed to poll redis: %s", err)
			}
		}
	}()
	return nil
}

// Poll redis to see if there are some triggers ready
func (s *RedisScheduler) Poll() error {
	now := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	// TODO it should be a loop
	res, err := s.client.Eval(luaPoll, []string{now}).Result()
	if err != nil || res == nil {
		return err
	}
	results, ok := res.([]interface{})
	if !ok {
		return errors.New("Unexpected response from redis")
	}
	if len(results) < 2 {
		return nil
	}
	parts := strings.SplitN(results[0].(string), ":", 2)
	if len(parts) != 2 {
		// TODO remove the trigger from redis
		return fmt.Errorf("Invalid key %s", res)
	}
	t, err := s.Get(parts[0], parts[1])
	if err != nil {
		// TODO if not found, remove the trigger from redis
		return err
	}
	switch t := t.(type) {
	case *AtTrigger:
		job := t.Trigger()
		if _, _, err = s.Broker.PushJob(job); err != nil {
			return err
		}
		return s.deleteTrigger(t)
	case *CronTrigger:
		job := t.Trigger()
		if _, _, err = s.Broker.PushJob(job); err != nil {
			return err
		}
		return s.addToRedis(t)
	}
	return errors.New("Not implemented yet")
}

// Add a trigger to the system, by persisting it and using redis for scheduling
// its jobs
func (s *RedisScheduler) Add(t Trigger) error {
	infos := t.Infos()
	db := couchdb.SimpleDatabasePrefix(infos.Domain)
	if err := couchdb.CreateDoc(db, infos); err != nil {
		return err
	}
	return s.addToRedis(t)
}

func (s *RedisScheduler) addToRedis(t Trigger) error {
	var timestamp time.Time
	switch t := t.(type) {
	case *AtTrigger:
		timestamp = t.at
	case *CronTrigger:
		timestamp = t.NextExecution(time.Now())
	case *EventTrigger:
		// TODO implement this (we ignore it because of the thumbnails trigger)
		return nil
	default:
		return errors.New("Not implemented yet")
	}
	return s.client.ZAddNX(TriggersKey, redis.Z{
		Score:  float64(timestamp.UTC().Unix()),
		Member: redisKey(t.Infos()),
	}).Err()
}

// Get returns the trigger with the specified ID.
func (s *RedisScheduler) Get(domain, id string) (Trigger, error) {
	var infos TriggerInfos
	db := couchdb.SimpleDatabasePrefix(domain)
	if err := couchdb.GetDoc(db, consts.Triggers, id, &infos); err != nil {
		return nil, err
	}
	return NewTrigger(&infos)
}

// Delete removes the trigger with the specified ID. The trigger is unscheduled
// and remove from the storage.
func (s *RedisScheduler) Delete(domain, id string) error {
	t, err := s.Get(domain, id)
	if err != nil {
		return err
	}
	return s.deleteTrigger(t)
}

func (s *RedisScheduler) deleteTrigger(t Trigger) error {
	db := couchdb.SimpleDatabasePrefix(t.Infos().Domain)
	if err := couchdb.DeleteDoc(db, t.Infos()); err != nil {
		return err
	}
	pipe := s.client.Pipeline()
	pipe.ZRem(TriggersKey, t.ID())
	pipe.ZRem(SchedKey, t.ID())
	_, err := pipe.Exec()
	return err
}

// GetAll returns all the triggers for a domain, from couch.
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
	v := make([]Trigger, 0, len(infos))
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
