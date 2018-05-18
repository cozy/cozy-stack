package jobs_test

import (
	"context"
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

func (b *mockBroker) StartWorkers(workersList jobs.WorkersList) error {
	return nil
}

func (b *mockBroker) ShutdownWorkers(ctx context.Context) error {
	return nil
}

func (b *mockBroker) PushJob(request *jobs.JobRequest) (*jobs.Job, error) {
	b.jobs = append(b.jobs, request)
	return nil, nil
}

func (b *mockBroker) WorkerQueueLen(workerType string) (int, error) {
	count := 0
	for _, job := range b.jobs {
		if job.WorkerType == workerType {
			count++
		}
	}
	return count, nil
}

func (b *mockBroker) WorkersTypes() []string {
	return []string{}
}

func TestRedisSchedulerWithTimeTriggers(t *testing.T) {
	var wAt sync.WaitGroup
	var wIn sync.WaitGroup
	bro := jobs.NewMemBroker()
	bro.StartWorkers(jobs.WorkersList{
		{
			WorkerType:   "worker",
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx *jobs.WorkerContext) error {
				var msg string
				if err := ctx.UnmarshalMessage(&msg); err != nil {
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

	msg1, _ := jobs.NewMessage("@at")
	msg2, _ := jobs.NewMessage("@in")

	wAt.Add(1) // 1 time in @at
	wIn.Add(1) // 1 time in @in

	at := &jobs.TriggerInfos{
		Type:       "@at",
		Domain:     instanceName,
		Arguments:  time.Now().Add(2 * time.Second).Format(time.RFC3339),
		WorkerType: "worker",
		Message:    msg1,
	}
	in := &jobs.TriggerInfos{
		Domain:     instanceName,
		Type:       "@in",
		Arguments:  "1s",
		WorkerType: "worker",
		Message:    msg2,
	}

	sch := jobs.System().(jobs.Scheduler)
	sch.ShutdownScheduler(context.Background())
	sch.StartScheduler(bro)

	tat, err := jobs.NewTrigger(at)
	assert.NoError(t, err)
	err = sch.AddTrigger(tat)
	assert.NoError(t, err)
	atID := tat.Infos().TID

	tin, err := jobs.NewTrigger(in)
	assert.NoError(t, err)
	err = sch.AddTrigger(tin)
	assert.NoError(t, err)
	inID := tin.Infos().TID

	ts, err := sch.GetAllTriggers(instanceName)
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

	_, err = sch.GetTrigger(instanceName, atID)
	assert.Error(t, err)
	assert.Equal(t, jobs.ErrNotFoundTrigger, err)

	_, err = sch.GetTrigger(instanceName, inID)
	assert.Error(t, err)
	assert.Equal(t, jobs.ErrNotFoundTrigger, err)
}

func TestRedisSchedulerWithCronTriggers(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(jobs.TriggersKey, jobs.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := jobs.System().(jobs.Scheduler)
	sch.ShutdownScheduler(context.Background())
	sch.StartScheduler(bro)
	sch.ShutdownScheduler(context.Background())
	defer sch.StartScheduler(bro)

	msg, _ := jobs.NewMessage("@cron")

	infos := &jobs.TriggerInfos{
		Type:       "@cron",
		Domain:     instanceName,
		Arguments:  "*/3 * * * * *",
		WorkerType: "incr",
		Message:    msg,
	}
	trigger, err := jobs.NewTrigger(infos)
	assert.NoError(t, err)
	err = sch.AddTrigger(trigger)
	assert.NoError(t, err)

	now := time.Now().UTC().Unix()
	for i := int64(0); i < 15; i++ {
		err = sch.PollScheduler(now + i + 4)
		assert.NoError(t, err)
	}
	count, _ := bro.WorkerQueueLen("incr")
	assert.Equal(t, 6, count)
}

func TestRedisPollFromSchedKey(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(jobs.TriggersKey, jobs.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := jobs.System().(jobs.Scheduler)
	sch.ShutdownScheduler(context.Background())
	sch.StartScheduler(bro)
	sch.ShutdownScheduler(context.Background())
	defer sch.StartScheduler(bro)

	now := time.Now()
	msg, _ := jobs.NewMessage("@at")

	at := &jobs.TriggerInfos{
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
	err = client.ZAdd(jobs.SchedKey, redis.Z{
		Score:  float64(ts + 1),
		Member: key,
	}).Err()
	assert.NoError(t, err)

	err = sch.PollScheduler(ts + 2)
	assert.NoError(t, err)
	<-time.After(1 * time.Millisecond)
	count, _ := bro.WorkerQueueLen("incr")
	assert.Equal(t, 0, count)

	err = sch.PollScheduler(ts + 13)
	assert.NoError(t, err)
	<-time.After(1 * time.Millisecond)
	count, _ = bro.WorkerQueueLen("incr")
	assert.Equal(t, 1, count)
}

func TestRedisTriggerEvent(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(jobs.TriggersKey, jobs.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := jobs.System().(jobs.Scheduler)
	sch.ShutdownScheduler(context.Background())
	time.Sleep(1 * time.Second)
	sch.StartScheduler(bro)

	evTrigger := &jobs.TriggerInfos{
		Type:       "@event",
		Domain:     instanceName,
		Arguments:  "io.cozy.event-test:CREATED",
		WorkerType: "incr",
	}
	tri, err := jobs.NewTrigger(evTrigger)
	assert.NoError(t, err)
	sch.AddTrigger(tri)

	realtime.GetHub().Publish(&realtime.Event{
		Domain: instanceName,
		Doc: &testDoc{
			id:      "foo",
			doctype: "io.cozy.event-test",
		},
		Verb: realtime.EventCreate,
	})

	time.Sleep(1 * time.Second)

	count, _ := bro.WorkerQueueLen("incr")
	if !assert.Equal(t, 1, count) {
		return
	}

	var evt struct {
		Domain string `json:"domain"`
		Verb   string `json:"verb"`
	}
	var data string
	err = bro.jobs[0].Event.Unmarshal(&evt)
	assert.NoError(t, err)
	err = bro.jobs[0].Message.Unmarshal(&data)
	assert.NoError(t, err)

	assert.Equal(t, evt.Domain, instanceName)
	assert.Equal(t, evt.Verb, "CREATED")

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

	count, _ = bro.WorkerQueueLen("incr")
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
	err := client.Del(jobs.TriggersKey, jobs.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := jobs.System().(jobs.Scheduler)
	sch.ShutdownScheduler(context.Background())
	time.Sleep(1 * time.Second)
	sch.StartScheduler(bro)

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

	evTrigger := &jobs.TriggerInfos{
		Type:       "@event",
		Domain:     instanceName,
		Arguments:  "io.cozy.files:CREATED:" + dir.DocID,
		WorkerType: "incr",
	}
	tri, err := jobs.NewTrigger(evTrigger)
	assert.NoError(t, err)
	sch.AddTrigger(tri)

	time.Sleep(1 * time.Second)
	count, _ := bro.WorkerQueueLen("incr")
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
	count, _ = bro.WorkerQueueLen("incr")
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
	count, _ = bro.WorkerQueueLen("incr")
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
	count, _ = bro.WorkerQueueLen("incr")
	assert.Equal(t, 3, count)
}

func TestRedisSchedulerWithDebounce(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	err := client.Del(jobs.TriggersKey, jobs.SchedKey).Err()
	assert.NoError(t, err)

	bro := &mockBroker{}
	sch := jobs.System().(jobs.Scheduler)
	sch.ShutdownScheduler(context.Background())
	time.Sleep(1 * time.Second)
	sch.StartScheduler(bro)

	evTrigger := &jobs.TriggerInfos{
		Type:       "@event",
		Domain:     instanceName,
		Arguments:  "io.cozy.debounce-test:CREATED io.cozy.debounce-more:CREATED",
		WorkerType: "incr",
		Debounce:   "2s",
	}
	tri, err := jobs.NewTrigger(evTrigger)
	assert.NoError(t, err)
	sch.AddTrigger(tri)

	doc := testDoc{
		id:      "foo",
		doctype: "io.cozy.debounce-test",
	}
	event := &realtime.Event{
		Domain: instanceName,
		Doc:    &doc,
		Verb:   realtime.EventCreate,
	}

	for i := 0; i < 10; i++ {
		time.Sleep(300 * time.Millisecond)
		realtime.GetHub().Publish(event)
	}

	time.Sleep(2500 * time.Millisecond)
	count, _ := bro.WorkerQueueLen("incr")
	assert.Equal(t, 2, count)

	realtime.GetHub().Publish(event)
	doc.doctype = "io.cozy.debounce-more"
	realtime.GetHub().Publish(event)
	time.Sleep(2500 * time.Millisecond)
	count, _ = bro.WorkerQueueLen("incr")
	assert.Equal(t, 3, count)
}

func TestMain(m *testing.M) {
	// prefix = "test:"
	config.UseTestFile()
	cfg := config.GetConfig()
	was := cfg.Jobs.RedisConfig
	var err error
	cfg.Jobs.RedisConfig, err = config.NewRedisConfig(redisURL)
	if err != nil {
		panic(err)
	}

	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "test_redis_scheduler")
	testInstance = setup.GetTestInstance()
	instanceName = testInstance.Domain

	setup.AddCleanup(func() error {
		cfg.Jobs.RedisConfig = was
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		return client.Del(jobs.TriggersKey, jobs.SchedKey).Err()
	})

	os.Exit(setup.Run())
}
