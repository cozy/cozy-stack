package job_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	jobs "github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/15"

var testInstance *instance.Instance

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

func (b *mockBroker) PushJob(db prefixer.Prefixer, request *jobs.JobRequest) (*jobs.Job, error) {
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
	assert.NoError(t, bro.StartWorkers(jobs.WorkersList{
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
	}))

	msg1, _ := jobs.NewMessage("@at")
	msg2, _ := jobs.NewMessage("@in")

	wAt.Add(1) // 1 time in @at
	wIn.Add(1) // 1 time in @in

	at := jobs.TriggerInfos{
		Type:       "@at",
		Arguments:  time.Now().Add(2 * time.Second).Format(time.RFC3339),
		WorkerType: "worker",
	}
	in := jobs.TriggerInfos{
		Type:       "@in",
		Arguments:  "1s",
		WorkerType: "worker",
	}

	sch := jobs.System().(jobs.Scheduler)
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	assert.NoError(t, sch.StartScheduler(bro))
	time.Sleep(50 * time.Millisecond)

	// Clear the existing triggers before testing with our triggers
	ts, err := sch.GetAllTriggers(testInstance)
	assert.NoError(t, err)
	for _, trigger := range ts {
		err = sch.DeleteTrigger(testInstance, trigger.ID())
		assert.NoError(t, err)
	}

	tat, err := jobs.NewTrigger(testInstance, at, msg1)
	assert.NoError(t, err)
	err = sch.AddTrigger(tat)
	assert.NoError(t, err)
	atID := tat.Infos().TID

	tin, err := jobs.NewTrigger(testInstance, in, msg2)
	assert.NoError(t, err)
	err = sch.AddTrigger(tin)
	assert.NoError(t, err)
	inID := tin.Infos().TID

	ts, err = sch.GetAllTriggers(testInstance)
	assert.NoError(t, err)
	assert.Len(t, ts, 2)

	for _, trigger := range ts {
		switch trigger.Infos().TID {
		case atID:
			assert.Equal(t, tat.Infos(), trigger.Infos())
		case inID:
			assert.Equal(t, tin.Infos(), trigger.Infos())
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

	_, err = sch.GetTrigger(testInstance, atID)
	assert.Error(t, err)
	assert.Equal(t, jobs.ErrNotFoundTrigger, err)

	_, err = sch.GetTrigger(testInstance, inID)
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
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	assert.NoError(t, sch.StartScheduler(bro))
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	defer func() { _ = sch.StartScheduler(bro) }()

	msg, _ := jobs.NewMessage("@cron")

	infos := jobs.TriggerInfos{
		Type:       "@cron",
		Arguments:  "*/3 * * * * *",
		WorkerType: "incr",
	}
	trigger, err := jobs.NewTrigger(testInstance, infos, msg)
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
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	assert.NoError(t, sch.StartScheduler(bro))
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	defer func() { _ = sch.StartScheduler(bro) }()

	now := time.Now()
	msg, _ := jobs.NewMessage("@at")

	at := jobs.TriggerInfos{
		Type:       "@at",
		Arguments:  now.Format(time.RFC3339),
		WorkerType: "incr",
	}

	tat, err := jobs.NewTrigger(testInstance, at, msg)
	assert.NoError(t, err)

	err = couchdb.CreateDoc(testInstance, tat.Infos())
	assert.NoError(t, err)

	ts := now.UTC().Unix()
	key := testInstance.DBPrefix() + "/" + tat.ID()
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
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	time.Sleep(1 * time.Second)
	assert.NoError(t, sch.StartScheduler(bro))

	evTrigger := jobs.TriggerInfos{
		Type:       "@event",
		Arguments:  "io.cozy.event-test:CREATED",
		WorkerType: "incr",
	}

	tri, err := jobs.NewTrigger(testInstance, evTrigger, nil)
	assert.NoError(t, err)
	assert.NoError(t, sch.AddTrigger(tri))

	realtime.GetHub().Publish(testInstance, realtime.EventCreate,
		&testDoc{id: "foo", doctype: "io.cozy.event-test"}, nil)

	time.Sleep(1 * time.Second)

	count, _ := bro.WorkerQueueLen("incr")
	if !assert.Equal(t, 1, count) {
		return
	}

	var evt struct {
		Domain string `json:"domain"`
		Prefix string `json:"prefix"`
		Verb   string `json:"verb"`
	}
	var data string
	err = bro.jobs[0].Event.Unmarshal(&evt)
	assert.NoError(t, err)
	err = bro.jobs[0].Message.Unmarshal(&data)
	assert.NoError(t, err)

	assert.Equal(t, evt.Domain, testInstance.Domain)
	assert.Equal(t, evt.Verb, "CREATED")

	realtime.GetHub().Publish(testInstance, realtime.EventUpdate,
		&testDoc{id: "foo", doctype: "io.cozy.event-test"}, nil)

	realtime.GetHub().Publish(testInstance, realtime.EventCreate,
		&testDoc{id: "foo", doctype: "io.cozy.event-test.bad"}, nil)

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
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	time.Sleep(1 * time.Second)
	assert.NoError(t, sch.StartScheduler(bro))

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

	evTrigger := jobs.TriggerInfos{
		Type:       "@event",
		Arguments:  "io.cozy.files:CREATED:" + dir.DocID,
		WorkerType: "incr",
	}
	tri, err := jobs.NewTrigger(testInstance, evTrigger, nil)
	assert.NoError(t, err)
	assert.NoError(t, sch.AddTrigger(tri))

	time.Sleep(1 * time.Second)
	count, _ := bro.WorkerQueueLen("incr")
	if !assert.Equal(t, 0, count) {
		return
	}

	barID := utils.RandomString(10)
	realtime.GetHub().Publish(testInstance, realtime.EventCreate,
		&vfs.DirDoc{
			Type:      "directory",
			DocID:     barID,
			DocRev:    "1-" + utils.RandomString(10),
			DocName:   "bar",
			DirID:     dir.DocID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Fullpath:  "/foo/bar",
		}, nil)

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

	realtime.GetHub().Publish(testInstance, realtime.EventCreate, baz, nil)

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

	realtime.GetHub().Publish(testInstance, realtime.EventCreate, quux, baz)

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
	assert.NoError(t, sch.ShutdownScheduler(context.Background()))
	time.Sleep(1 * time.Second)
	assert.NoError(t, sch.StartScheduler(bro))

	evTrigger := jobs.TriggerInfos{
		Type:       "@event",
		Arguments:  "io.cozy.debounce-test:CREATED io.cozy.debounce-more:CREATED",
		WorkerType: "incr",
		Debounce:   "4s",
	}
	tri, err := jobs.NewTrigger(testInstance, evTrigger, nil)
	assert.NoError(t, err)
	assert.NoError(t, sch.AddTrigger(tri))

	doc := testDoc{
		id:      "foo",
		doctype: "io.cozy.debounce-test",
	}

	for i := 0; i < 10; i++ {
		time.Sleep(600 * time.Millisecond)
		realtime.GetHub().Publish(testInstance, realtime.EventCreate, &doc, nil)
	}

	time.Sleep(5000 * time.Millisecond)
	count, _ := bro.WorkerQueueLen("incr")
	assert.Equal(t, 2, count)

	realtime.GetHub().Publish(testInstance, realtime.EventCreate, &doc, nil)
	doc.doctype = "io.cozy.debounce-more"
	realtime.GetHub().Publish(testInstance, realtime.EventCreate, &doc, nil)
	time.Sleep(5000 * time.Millisecond)
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
	setup := testutils.NewSetup(m, "test_jobs")
	testInstance = setup.GetTestInstance()

	setup.AddCleanup(func() error {
		cfg.Jobs.RedisConfig = was
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		return client.Del(jobs.TriggersKey, jobs.SchedKey).Err()
	})

	os.Exit(setup.Run())
}
