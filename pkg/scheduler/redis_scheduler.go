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
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/go-redis/redis"
)

// TriggersKey is the the key of the sorted set in redis used for triggers
// waiting to be activated
const TriggersKey = "triggers"

// SchedKey is the the key of the sorted set in redis used for triggers
// currently being executed
const SchedKey = "scheduling"

// pollInterval is the time interval between 2 redis polling
const pollInterval = 1 * time.Second

// eventLoopSize is the number of goroutines handling @events and triggering
// jobs.
const eventLoopSize = 50

// luaPoll returns the lua script used for polling triggers in redis.
// If a trigger is in the scheduling key for more than 10 seconds, it is
// an error and we can try again to schedule it.
const luaPoll = `
local w = KEYS[1] - 10
local s = redis.call("ZRANGEBYSCORE", "` + SchedKey + `", 0, w, "WITHSCORES", "LIMIT", 0, 1)
if #s > 0 then
  redis.call("ZADD", "` + SchedKey + `", KEYS[1], s[1])
  return s
end
local t = redis.call("ZRANGEBYSCORE", "` + TriggersKey + `", 0, KEYS[1], "WITHSCORES", "LIMIT", 0, 1)
if #t > 0 then
  redis.call("ZREM", "` + TriggersKey + `", t[1])
  redis.call("ZADD", "` + SchedKey + `", t[2], t[1])
end
return t`

// RedisScheduler is a centralized scheduler of many triggers. It starts all of
// them and schedules jobs accordingly.
type RedisScheduler struct {
	broker  jobs.Broker
	client  *redis.Client
	stopped chan struct{}
}

// NewRedisScheduler creates a new scheduler that use redis to synchronize with
// other cozy-stack processes to schedule jobs.
func NewRedisScheduler(client *redis.Client) *RedisScheduler {
	return &RedisScheduler{
		client: client,
	}
}

func redisKey(infos *TriggerInfos) string {
	return infos.Domain + "/" + infos.TID
}

func eventsKey(domain string) string {
	return "events-" + domain
}

// Start a goroutine that will fetch triggers in redis to schedule their jobs
func (s *RedisScheduler) Start(b jobs.Broker) error {
	s.broker = b
	s.stopped = make(chan struct{})
	s.startEventDispatcher()
	go s.pollLoop()
	return nil
}

func (s *RedisScheduler) pollLoop() {
	ticker := time.NewTicker(pollInterval)
	for {
		select {
		case <-s.stopped:
			ticker.Stop()
			return
		case <-ticker.C:
			now := time.Now().UTC().Unix()
			if err := s.Poll(now); err != nil {
				log.Warnf("[scheduler] Failed to poll redis: %s", err)
			}
		}
	}
}

func (s *RedisScheduler) startEventDispatcher() {
	eventsCh := make(chan *realtime.Event, 100)
	go func() {
		c := realtime.GetHub().SubscribeAll()
		for {
			select {
			case <-s.stopped:
				c.Close()
				close(eventsCh)
				return
			case event := <-c.Read():
				eventsCh <- event
			}
		}
	}()
	for i := 0; i < eventLoopSize; i++ {
		go s.eventLoop(eventsCh)
	}
}

func (s *RedisScheduler) eventLoop(ch <-chan *realtime.Event) {
	for event := range ch {
		key := eventsKey(event.Domain)
		m, err := s.client.HGetAll(key).Result()
		if err != nil {
			log.Errorf("[scheduler] Could not fetch redis set %s: %s",
				key, err.Error())
			continue
		}
		for triggerID, args := range m {
			rule, err := permissions.UnmarshalRuleString(args)
			if err != nil {
				log.Warnf("[scheduler] Coud not unmarshal rule %s: %s",
					key, err.Error())
				continue
			}
			if !eventMatchPermission(event, &rule) {
				continue
			}
			t, err := s.Get(event.Domain, triggerID)
			if err != nil {
				log.Warnf("[scheduler] Could not fetch @event trigger %s %s: %s",
					event.Domain, triggerID, err.Error())
				continue
			}
			_, _, err = s.broker.PushJob(t.(*EventTrigger).Trigger(event))
			if err != nil {
				log.Warnf("[scheduler] Could not push job trigger by event %s %s: %s",
					event.Domain, triggerID, err.Error())
				continue
			}
		}
	}
}

// Stop the scheduling of triggers
func (s *RedisScheduler) Stop() {
	if s.stopped != nil {
		close(s.stopped)
	}
}

// Poll redis to see if there are some triggers ready
func (s *RedisScheduler) Poll(now int64) error {
	keys := []string{strconv.FormatInt(now, 10)}
	for {
		res, err := s.client.Eval(luaPoll, keys).Result()
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
		parts := strings.SplitN(results[0].(string), "/", 2)
		if len(parts) != 2 {
			s.client.ZRem(SchedKey, results[0])
			return fmt.Errorf("Invalid key %s", res)
		}
		t, err := s.Get(parts[0], parts[1])
		if err != nil {
			s.client.ZRem(SchedKey, results[0])
			return err
		}
		switch t := t.(type) {
		case *AtTrigger:
			job := t.Trigger()
			if _, _, err = s.broker.PushJob(job); err != nil {
				return err
			}
			if err = s.deleteTrigger(t); err != nil {
				return err
			}
		case *CronTrigger:
			job := t.Trigger()
			if _, _, err = s.broker.PushJob(job); err != nil {
				return err
			}
			score, err := strconv.ParseInt(results[1].(string), 10, 64)
			var prev time.Time
			if err != nil {
				prev = time.Now()
			} else {
				prev = time.Unix(score, 0)
			}
			if err := s.addToRedis(t, prev); err != nil {
				return err
			}
		default:
			return errors.New("Not implemented yet")
		}
	}
}

// Add a trigger to the system, by persisting it and using redis for scheduling
// its jobs
func (s *RedisScheduler) Add(t Trigger) error {
	infos := t.Infos()
	db := couchdb.SimpleDatabasePrefix(infos.Domain)
	if err := couchdb.CreateDoc(db, infos); err != nil {
		return err
	}
	return s.addToRedis(t, time.Now())
}

func (s *RedisScheduler) addToRedis(t Trigger, prev time.Time) error {
	var timestamp time.Time
	switch t := t.(type) {
	case *EventTrigger:
		hKey := eventsKey(t.Infos().Domain)
		hMap := make(map[string]interface{})
		hMap[t.ID()] = t.Infos().Arguments
		return s.client.HMSet(hKey, hMap).Err()
	case *AtTrigger:
		timestamp = t.at
	case *CronTrigger:
		timestamp = t.NextExecution(prev)
		now := time.Now()
		if timestamp.Before(now) {
			timestamp = t.NextExecution(now)
		}
	default:
		return errors.New("Not implemented yet")
	}
	return s.client.ZAdd(TriggersKey, redis.Z{
		Score:  float64(timestamp.UTC().Unix()),
		Member: redisKey(t.Infos()),
	}).Err()
}

// Get returns the trigger with the specified ID.
func (s *RedisScheduler) Get(domain, id string) (Trigger, error) {
	var infos TriggerInfos
	db := couchdb.SimpleDatabasePrefix(domain)
	if err := couchdb.GetDoc(db, consts.Triggers, id, &infos); err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, ErrNotFoundTrigger
		}
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
	switch t.(type) {
	case *EventTrigger:
		return s.client.HDel(eventsKey(t.Infos().Domain), t.ID()).Err()
	case *AtTrigger, *CronTrigger:
		pipe := s.client.Pipeline()
		pipe.ZRem(TriggersKey, t.ID())
		pipe.ZRem(SchedKey, t.ID())
		_, err := pipe.Exec()
		return err
	}
	return nil
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
