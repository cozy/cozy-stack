package scheduler_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/15"

var testInstance *instance.Instance
var instanceName string

type testDoc struct {
	id      string
	rev     string
	doctype string
}

func (t *testDoc) ID() string      { return t.id }
func (t *testDoc) Rev() string     { return t.rev }
func (t *testDoc) DocType() string { return t.doctype }

type mockBroker struct {
	jobs []*jobs.JobRequest
}

func (b *mockBroker) Start(workersList jobs.WorkersList) error {
	return nil
}

func (b *mockBroker) Shutdown(ctx context.Context) error {
	return nil
}

func (b *mockBroker) PushJob(request *jobs.JobRequest) (*jobs.JobInfos, error) {
	b.jobs = append(b.jobs, request)
	return nil, nil
}

func (b *mockBroker) QueueLen(workerType string) (int, error) {
	count := 0
	for _, job := range b.jobs {
		if job.WorkerType == workerType {
			count++
		}
	}
	return count, nil
}

func (b *mockBroker) GetJobInfos(domain, id string) (*jobs.JobInfos, error) {
	return nil, errors.New("Not implemented")
}

func TestRedisSchedulerWithTimeTriggers(t *testing.T) {
	var wAt sync.WaitGroup
	var wIn sync.WaitGroup
	bro := jobs.NewMemBroker(1)
	bro.Start(jobs.WorkersList{
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
	sch.Shutdown(context.Background())
	sch.Start(bro)

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

func TestRedisSchedulerWithCronTriggers(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(scheduler.TriggersKey, scheduler.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := stack.GetScheduler().(*scheduler.RedisScheduler)
	sch.Shutdown(context.Background())
	sch.Start(bro)
	sch.Shutdown(context.Background())
	defer sch.Start(bro)

	msg, _ := jobs.NewMessage("json", "@cron")

	infos := &scheduler.TriggerInfos{
		Type:       "@cron",
		Domain:     instanceName,
		Arguments:  "*/3 * * * * *",
		WorkerType: "incr",
		Message:    msg,
	}
	trigger, err := scheduler.NewTrigger(infos)
	assert.NoError(t, err)
	err = sch.Add(trigger)
	assert.NoError(t, err)

	now := time.Now().UTC().Unix()
	for i := int64(0); i < 15; i++ {
		err = sch.Poll(now + i + 4)
		assert.NoError(t, err)
	}
	count, _ := bro.QueueLen("incr")
	assert.Equal(t, 6, count)
}

func TestRedisPollFromSchedKey(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(scheduler.TriggersKey, scheduler.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := stack.GetScheduler().(*scheduler.RedisScheduler)
	sch.Shutdown(context.Background())
	sch.Start(bro)
	sch.Shutdown(context.Background())
	defer sch.Start(bro)

	now := time.Now()
	msg, _ := jobs.NewMessage("json", "@at")

	at := &scheduler.TriggerInfos{
		Type:       "@at",
		Domain:     instanceName,
		Arguments:  now.Format(time.RFC3339),
		WorkerType: "incr",
		Message:    msg,
	}
	db := couchdb.SimpleDatabasePrefix(instanceName)
	err = couchdb.CreateDoc(db, at)
	assert.NoError(t, err)

	ts := now.UTC().Unix()
	key := instanceName + "/" + at.TID
	err = client.ZAdd(scheduler.SchedKey, redis.Z{
		Score:  float64(ts + 1),
		Member: key,
	}).Err()
	assert.NoError(t, err)

	err = sch.Poll(ts + 2)
	assert.NoError(t, err)
	<-time.After(1 * time.Millisecond)
	count, _ := bro.QueueLen("incr")
	assert.Equal(t, 0, count)

	err = sch.Poll(ts + 13)
	assert.NoError(t, err)
	<-time.After(1 * time.Millisecond)
	count, _ = bro.QueueLen("incr")
	assert.Equal(t, 1, count)
}

func TestRedisTriggerEvent(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(scheduler.TriggersKey, scheduler.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := stack.GetScheduler().(*scheduler.RedisScheduler)
	sch.Shutdown(context.Background())
	time.Sleep(1 * time.Second)
	sch.Start(bro)

	evTrigger := &scheduler.TriggerInfos{
		Type:       "@event",
		Domain:     instanceName,
		Arguments:  "io.cozy.event-test:CREATED",
		WorkerType: "incr",
	}
	tri, err := scheduler.NewTrigger(evTrigger)
	assert.NoError(t, err)
	sch.Add(tri)

	realtime.GetHub().Publish(&realtime.Event{
		Domain: instanceName,
		Doc: &testDoc{
			id:      "foo",
			doctype: "io.cozy.event-test",
		},
		Verb: realtime.EventCreate,
	})

	time.Sleep(1 * time.Second)

	count, _ := bro.QueueLen("incr")
	if !assert.Equal(t, 1, count) {
		return
	}

	type eventMessage struct {
		Message string
		Event   map[string]interface{}
	}
	var data eventMessage
	err = bro.jobs[0].Message.Unmarshal(&data)
	assert.NoError(t, err)
	assert.Equal(t, data.Event["domain"].(string), instanceName)
	assert.Equal(t, data.Event["verb"].(string), "CREATED")

	realtime.GetHub().Publish(&realtime.Event{
		Domain: instanceName,
		Doc: &testDoc{
			id:      "foo",
			doctype: "io.cozy.event-test",
		},
		Verb: realtime.EventUpdate,
	})

	realtime.GetHub().Publish(&realtime.Event{
		Domain: instanceName,
		Doc: &testDoc{
			id:      "foo",
			doctype: "io.cozy.event-test.bad",
		},
		Verb: realtime.EventCreate,
	})

	time.Sleep(10 * time.Millisecond)

	count, _ = bro.QueueLen("incr")
	assert.Equal(t, 1, count)
}

// fakeFilePather is used to force a cached value for the fullpath of a FileDoc
type fakeFilePather struct {
	Fullpath string
}

func (d fakeFilePather) FilePath(doc *vfs.FileDoc) (string, error) {
	return d.Fullpath, nil
}

func TestRedisTriggerEventForDirectories(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(scheduler.TriggersKey, scheduler.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := stack.GetScheduler().(*scheduler.RedisScheduler)
	sch.Shutdown(context.Background())
	time.Sleep(1 * time.Second)
	sch.Start(bro)

	dir := &vfs.DirDoc{
		Type:      "directory",
		DocName:   "foo",
		DirID:     consts.RootDirID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Fullpath:  "/foo",
	}
	err = testInstance.VFS().CreateDirDoc(dir)
	assert.NoError(t, err)

	evTrigger := &scheduler.TriggerInfos{
		Type:       "@event",
		Domain:     instanceName,
		Arguments:  "io.cozy.files:CREATED:" + dir.DocID,
		WorkerType: "incr",
	}
	tri, err := scheduler.NewTrigger(evTrigger)
	assert.NoError(t, err)
	sch.Add(tri)

	time.Sleep(1 * time.Second)
	count, _ := bro.QueueLen("incr")
	if !assert.Equal(t, 0, count) {
		return
	}

	barID := utils.RandomString(10)
	realtime.GetHub().Publish(&realtime.Event{
		Domain: instanceName,
		Doc: &vfs.DirDoc{
			Type:      "directory",
			DocID:     barID,
			DocRev:    "1-" + utils.RandomString(10),
			DocName:   "bar",
			DirID:     dir.DocID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Fullpath:  "/foo/bar",
		},
		Verb: realtime.EventCreate,
	})

	time.Sleep(100 * time.Millisecond)
	count, _ = bro.QueueLen("incr")
	assert.Equal(t, 1, count)

	bazID := utils.RandomString(10)
	baz := &vfs.FileDoc{
		Type:      "file",
		DocID:     bazID,
		DocRev:    "1-" + utils.RandomString(10),
		DocName:   "baz",
		DirID:     barID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ByteSize:  42,
		Mime:      "application/json",
		Class:     "application",
		Trashed:   false,
	}
	ffp := fakeFilePather{"/foo/bar/baz"}
	p, err := baz.Path(ffp)
	assert.NoError(t, err)
	assert.Equal(t, "/foo/bar/baz", p)

	realtime.GetHub().Publish(&realtime.Event{
		Domain: instanceName,
		Doc:    baz,
		Verb:   realtime.EventCreate,
	})

	time.Sleep(100 * time.Millisecond)
	count, _ = bro.QueueLen("incr")
	assert.Equal(t, 2, count)

	// Simulate that /foo/bar/baz is moved to /quux
	quux := &vfs.FileDoc{
		Type:      "file",
		DocID:     bazID,
		DocRev:    "2-" + utils.RandomString(10),
		DocName:   "quux",
		DirID:     consts.RootDirID,
		CreatedAt: baz.CreatedAt,
		UpdatedAt: time.Now(),
		ByteSize:  42,
		Mime:      "application/json",
		Class:     "application",
		Trashed:   false,
	}
	ffp = fakeFilePather{"/quux"}
	p, err = quux.Path(ffp)
	assert.NoError(t, err)
	assert.Equal(t, "/quux", p)

	realtime.GetHub().Publish(&realtime.Event{
		Domain: instanceName,
		Doc:    quux,
		OldDoc: baz,
		Verb:   realtime.EventCreate,
	})

	time.Sleep(100 * time.Millisecond)
	count, _ = bro.QueueLen("incr")
	assert.Equal(t, 3, count)
}

func TestRedisSchedulerWithDebounce(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(scheduler.TriggersKey, scheduler.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := stack.GetScheduler().(*scheduler.RedisScheduler)
	sch.Shutdown(context.Background())
	time.Sleep(1 * time.Second)
	sch.Start(bro)

	evTrigger := &scheduler.TriggerInfos{
		Type:       "@event",
		Domain:     instanceName,
		Arguments:  "io.cozy.debounce-test:CREATED",
		WorkerType: "incr",
		Debounce:   "2s",
	}
	tri, err := scheduler.NewTrigger(evTrigger)
	assert.NoError(t, err)
	sch.Add(tri)

	event := &realtime.Event{
		Domain: instanceName,
		Doc: &testDoc{
			id:      "foo",
			doctype: "io.cozy.debounce-test",
		},
		Verb: realtime.EventCreate,
	}

	for i := 0; i < 10; i++ {
		time.Sleep(300 * time.Millisecond)
		realtime.GetHub().Publish(event)
	}

	time.Sleep(2 * time.Second)
	count, _ := bro.QueueLen("incr")
	assert.Equal(t, 2, count)
}

func TestMain(m *testing.M) {
	// prefix = "test:"
	config.UseTestFile()
	cfg := config.GetConfig()
	was := cfg.Jobs.Redis
	cfg.Jobs.Redis = config.NewRedisConfig(redisURL)

	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "test_redis_scheduler")
	testInstance = setup.GetTestInstance()
	instanceName = testInstance.Domain

	setup.AddCleanup(func() error {
		cfg.Jobs.Redis = was
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		return client.Del(scheduler.TriggersKey, scheduler.SchedKey).Err()
	})

	os.Exit(setup.Run())
}
