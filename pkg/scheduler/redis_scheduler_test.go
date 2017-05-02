package scheduler_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/"

var instanceName string

func TestRedisSchedulerWithTimeTriggers(t *testing.T) {
	var wAt sync.WaitGroup
	var wIn sync.WaitGroup
	bro := jobs.NewMemBroker(jobs.WorkersList{
		"worker": {
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, m *jobs.Message) error {
				var msg string
				if err := m.Unmarshal(&msg); err != nil {
					return err
				}
				switch msg {
				case "@at":
					wAt.Done()
				case "@in":
					wIn.Done()
				}
				return nil
			},
		},
	})

	msg1, _ := jobs.NewMessage("json", "@at")
	msg2, _ := jobs.NewMessage("json", "@in")

	wAt.Add(1) // 1 time in @at
	wIn.Add(1) // 1 time in @in

	at := &scheduler.TriggerInfos{
		Type:       "@at",
		Domain:     instanceName,
		Arguments:  time.Now().Add(2 * time.Second).Format(time.RFC3339),
		WorkerType: "worker",
		Message:    msg1,
	}
	in := &scheduler.TriggerInfos{
		Domain:     instanceName,
		Type:       "@in",
		Arguments:  "1s",
		WorkerType: "worker",
		Message:    msg2,
	}

	sch := stack.GetScheduler().(*scheduler.RedisScheduler)
	// TODO Don't export Broker
	// sch.Stop()
	// sch.Start(bro)
	sch.Broker = bro

	tat, err := scheduler.NewTrigger(at)
	assert.NoError(t, err)
	err = sch.Add(tat)
	assert.NoError(t, err)
	atID := tat.Infos().TID

	tin, err := scheduler.NewTrigger(in)
	assert.NoError(t, err)
	err = sch.Add(tin)
	assert.NoError(t, err)
	inID := tin.Infos().TID

	ts, err := sch.GetAll(instanceName)
	assert.NoError(t, err)
	assert.Len(t, ts, 3) // 1 @event for thumbnails + 1 @at + 1 @in

	for _, trigger := range ts {
		switch trigger.Infos().TID {
		case atID:
			assert.Equal(t, at, trigger.Infos())
		case inID:
			assert.Equal(t, in, trigger.Infos())
		default:
			// Just ignore the @event trigger for generating thumbnails
			infos := trigger.Infos()
			if infos.Type != "@event" || infos.WorkerType != "thumbnail" {
				t.Fatalf("unknown trigger ID %s", trigger.Infos().TID)
			}
		}
	}

	done := make(chan bool)
	go func() {
		wAt.Wait()
		done <- true
	}()

	go func() {
		wIn.Wait()
		done <- true
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout")
		}
	}

	time.Sleep(50 * time.Millisecond)

	_, err = sch.Get(instanceName, atID)
	assert.Error(t, err)
	assert.Equal(t, scheduler.ErrNotFoundTrigger, err)

	_, err = sch.Get(instanceName, inID)
	assert.Error(t, err)
	assert.Equal(t, scheduler.ErrNotFoundTrigger, err)
}

func TestMain(m *testing.M) {
	// prefix = "test:"
	config.UseTestFile()
	cfg := config.GetConfig()
	was := cfg.Jobs.URL
	cfg.Jobs.URL = redisURL

	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "test_redis_scheduler")
	instanceName = setup.GetTestInstance().Domain

	setup.AddCleanup(func() error {
		cfg.Jobs.URL = was
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		return client.Del(scheduler.TriggersKey, scheduler.SchedKey).Err()
	})

	os.Exit(setup.Run())
}
