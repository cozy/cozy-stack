package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
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

// redisScheduler is a centralized scheduler of many triggers. It starts all of
// them and schedules jobs accordingly.
type redisScheduler struct {
	broker  Broker
	client  redis.UniversalClient
	closed  chan struct{}
	stopped chan struct{}
	log     *logrus.Entry
}

// NewRedisScheduler creates a new scheduler that use redis to synchronize with
// other cozy-stack processes to schedule jobs.
func NewRedisScheduler(client redis.UniversalClient) Scheduler {
	return &redisScheduler{
		client:  client,
		log:     logger.WithNamespace("scheduler-redis"),
		stopped: make(chan struct{}),
	}
}

func redisKey(infos *TriggerInfos) string {
	return infos.Domain + "/" + infos.TID
}

func eventsKey(domain string) string {
	return "events-" + domain
}

// StartScheduler a goroutine that will fetch triggers in redis to schedule
// their jobs.
func (s *redisScheduler) StartScheduler(b Broker) error {
	s.broker = b
	s.closed = make(chan struct{})
	s.startEventDispatcher()
	go s.pollLoop()
	return nil
}

func (s *redisScheduler) pollLoop() {
	ticker := time.NewTicker(pollInterval)
	for {
		select {
		case <-s.closed:
			ticker.Stop()
			s.stopped <- struct{}{}
			return
		case <-ticker.C:
			now := time.Now().UTC().Unix()
			if err := s.PollScheduler(now); err != nil {
				s.log.Warnf("Failed to poll redis: %s", err)
			}
		}
	}
}

func (s *redisScheduler) startEventDispatcher() {
	eventsCh := make(chan *realtime.Event, 100)
	go func() {
		c := realtime.GetHub().SubscribeLocalAll()
		defer func() {
			c.Close()
			close(eventsCh)
		}()
		for {
			select {
			case <-s.closed:
				return
			case event := <-c.Channel:
				eventsCh <- event
			}
		}
	}()
	for i := 0; i < eventLoopSize; i++ {
		go s.eventLoop(eventsCh)
	}
}

func (s *redisScheduler) eventLoop(eventsCh <-chan *realtime.Event) {
	for event := range eventsCh {
		key := eventsKey(event.Domain)
		m, err := s.client.HGetAll(key).Result()
		if err != nil {
			s.log.Errorf("Could not fetch redis set %s: %s",
				key, err.Error())
			continue
		}
		for triggerID, arguments := range m {
			found := false
			for _, args := range strings.Split(arguments, " ") {
				rule, err := permissions.UnmarshalRuleString(args)
				if err != nil {
					s.log.Warnf("Coud not unmarshal rule %s: %s",
						key, err.Error())
					continue
				}
				if eventMatchPermission(event, &rule) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
			t, err := s.GetTrigger(event.Domain, triggerID)
			if err != nil {
				s.log.Warnf("Could not fetch @event trigger %s %s: %s",
					event.Domain, triggerID, err.Error())
				continue
			}
			et := t.(*EventTrigger)
			if et.infos.Debounce != "" {
				var d time.Duration
				if d, err = time.ParseDuration(et.infos.Debounce); err == nil {
					timestamp := time.Now().Add(d)
					s.client.ZAddNX(TriggersKey, redis.Z{
						Score:  float64(timestamp.UTC().Unix()),
						Member: redisKey(t.Infos()),
					})
					continue
				} else {
					s.log.Warnf("Trigger %s %s has an invalid debounce: %s",
						et.infos.Domain, et.infos.TID, et.infos.Debounce)
					continue
				}
			}
			jobRequest, err := et.Infos().JobRequestWithEvent(event)
			if err != nil {
				s.log.Warnf("Could not encode realtime event %s %s: %s",
					event.Domain, triggerID, err.Error())
				continue
			}
			_, err = s.broker.PushJob(jobRequest)
			if err != nil {
				s.log.Warnf("Could not push job trigger by event %s %s: %s",
					event.Domain, triggerID, err.Error())
				continue
			}
		}
	}
}

// ShutdownScheduler shuts down the the scheduling of triggers
func (s *redisScheduler) ShutdownScheduler(ctx context.Context) error {
	if s.closed == nil {
		return nil
	}
	fmt.Print("  shutting down redis scheduler...")
	close(s.closed)
	select {
	case <-ctx.Done():
		fmt.Println("failed: ", ctx.Err())
		return ctx.Err()
	case <-s.stopped:
		fmt.Println("ok.")
	}
	return nil
}

// PollScheduler polls redis to see if there are some triggers ready.
func (s *redisScheduler) PollScheduler(now int64) error {
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
		t, err := s.GetTrigger(parts[0], parts[1])
		if err != nil {
			if err == ErrNotFoundTrigger {
				s.client.ZRem(SchedKey, results[0])
			}
			return err
		}
		switch t := t.(type) {
		case *EventTrigger: // Debounced
			job := t.Infos().JobRequest()
			job.Debounced = true
			if err = s.client.ZRem(SchedKey, results[0]).Err(); err != nil {
				return err
			}
			if _, err = s.broker.PushJob(job); err != nil {
				return err
			}
		case *AtTrigger:
			job := t.Infos().JobRequest()
			if _, err = s.broker.PushJob(job); err != nil {
				return err
			}
			if err = s.deleteTrigger(t); err != nil {
				return err
			}
		case *CronTrigger:
			job := t.Infos().JobRequest()
			if _, err = s.broker.PushJob(job); err != nil {
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

// AddTrigger a trigger to the system, by persisting it and using redis for
// scheduling its jobs
func (s *redisScheduler) AddTrigger(t Trigger) error {
	infos := t.Infos()
	db := couchdb.SimpleDatabasePrefix(infos.Domain)
	if err := couchdb.CreateDoc(db, infos); err != nil {
		return err
	}
	return s.addToRedis(t, time.Now())
}

func (s *redisScheduler) addToRedis(t Trigger, prev time.Time) error {
	var timestamp time.Time
	switch t := t.(type) {
	case *EventTrigger:
		hKey := eventsKey(t.Infos().Domain)
		return s.client.HSet(hKey, t.ID(), t.Infos().Arguments).Err()
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
	pipe := s.client.Pipeline()
	pipe.ZAdd(TriggersKey, redis.Z{
		Score:  float64(timestamp.UTC().Unix()),
		Member: redisKey(t.Infos()),
	}).Err()
	pipe.ZRem(SchedKey, redisKey(t.Infos()))
	_, err := pipe.Exec()
	return err
}

// GetTrigger returns the trigger with the specified ID.
func (s *redisScheduler) GetTrigger(domain, id string) (Trigger, error) {
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

// DeleteTrigger removes the trigger with the specified ID. The trigger is
// unscheduled and remove from the storage.
func (s *redisScheduler) DeleteTrigger(domain, id string) error {
	t, err := s.GetTrigger(domain, id)
	if err != nil {
		return err
	}
	return s.deleteTrigger(t)
}

func (s *redisScheduler) deleteTrigger(t Trigger) error {
	db := couchdb.SimpleDatabasePrefix(t.Infos().Domain)
	if err := couchdb.DeleteDoc(db, t.Infos()); err != nil {
		return err
	}
	switch t.(type) {
	case *EventTrigger:
		return s.client.HDel(eventsKey(t.Infos().Domain), t.ID()).Err()
	case *AtTrigger, *CronTrigger:
		pipe := s.client.Pipeline()
		pipe.ZRem(TriggersKey, redisKey(t.Infos()))
		pipe.ZRem(SchedKey, redisKey(t.Infos()))
		_, err := pipe.Exec()
		return err
	}
	return nil
}

// GetAllTriggers returns all the triggers for a domain, from couch.
func (s *redisScheduler) GetAllTriggers(domain string) ([]Trigger, error) {
	var infos []*TriggerInfos
	db := couchdb.SimpleDatabasePrefix(domain)
	err := couchdb.ForeachDocs(db, consts.Triggers, func(_ string, data json.RawMessage) error {
		var t *TriggerInfos
		if err := json.Unmarshal(data, &t); err != nil {
			return err
		}
		infos = append(infos, t)
		return nil
	})
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

// CleanRedis removes clean redis by removing the two sets holding the triggers
// states.
func (s *redisScheduler) CleanRedis() error {
	return s.client.Del(TriggersKey, SchedKey).Err()
}

// RebuildRedis puts all the triggers in redis (idempotent)
func (s *redisScheduler) RebuildRedis(domain string) error {
	triggers, err := s.GetAllTriggers(domain)
	if err != nil {
		return err
	}
	for _, t := range triggers {
		if err = s.addToRedis(t, time.Now()); err != nil {
			return err
		}
	}
	joblog.Infof("Redis rebuilt for domain %q with %d triggers created",
		domain, len(triggers))
	return nil
}

var _ Scheduler = &redisScheduler{}
