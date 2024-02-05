package job_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

func TestMemBroker(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance()

	t.Run("ProperSerial", func(t *testing.T) {
		j := job.NewJob(prefixer.NewPrefixer(0, "cozy.localhost:8080", "cozy.localhost:8080"),
			&job.JobRequest{
				WorkerType: "",
			})
		assert.NoError(t, j.Create())
		assert.NoError(t, j.AckConsumed())
		job2, err := job.Get(j, j.ID())
		assert.NoError(t, err)
		assert.Equal(t, job.Running, job2.State)
	})

	t.Run("MessageMarshalling", func(t *testing.T) {
		data := []byte(`{"Data": "InZhbHVlIgo=", "Type": "json"}`)
		var m job.Message
		assert.NoError(t, json.Unmarshal(data, &m))
		var s string
		assert.NoError(t, m.Unmarshal(&s))
		assert.Equal(t, "value", s)

		data = []byte(`"value2"`)
		assert.NoError(t, json.Unmarshal(data, &m))
		assert.NoError(t, m.Unmarshal(&s))
		assert.Equal(t, "value2", s)

		data = []byte(`{
		"domain": "cozy.local",
		"worker": "foo",
		"message": {"Data": "InZhbHVlIgo=", "Type": "json"}
}`)

		var j job.Job
		assert.NoError(t, json.Unmarshal(data, &j))
		assert.Equal(t, "cozy.local", j.Domain)
		assert.Equal(t, "foo", j.WorkerType)
		assert.EqualValues(t, []byte(`"value"`), j.Message)

		var err error
		var j2 job.Job
		data, err = json.Marshal(j)
		assert.NoError(t, err)
		assert.NoError(t, json.Unmarshal(data, &j2))
		assert.Equal(t, "cozy.local", j2.Domain)
		assert.Equal(t, "foo", j2.WorkerType)
		assert.EqualValues(t, []byte(`"value"`), j2.Message)

		assert.EqualValues(t, &j2, j2.Clone())
	})

	t.Run("InMemoryJobs", func(t *testing.T) {
		n := 10
		v := 100

		var w sync.WaitGroup

		workersTestList := job.WorkersList{
			{
				WorkerType:  "test",
				Concurrency: 4,
				WorkerFunc: func(ctx *job.TaskContext) error {
					var msg string
					err := ctx.UnmarshalMessage(&msg)
					if !assert.NoError(t, err) {
						return err
					}
					if strings.HasPrefix(msg, "a-") {
						_, err := strconv.Atoi(msg[len("a-"):])
						assert.NoError(t, err)
					} else if strings.HasPrefix(msg, "b-") {
						_, err := strconv.Atoi(msg[len("b-"):])
						assert.NoError(t, err)
					} else {
						t.Fatal()
					}
					w.Done()
					return nil
				},
			},
		}

		broker1 := job.NewMemBroker()
		broker2 := job.NewMemBroker()
		assert.NoError(t, broker1.StartWorkers(workersTestList))
		assert.NoError(t, broker2.StartWorkers(workersTestList))
		w.Add(2)

		go func() {
			for i := 0; i < n; i++ {
				w.Add(1)
				msg, _ := job.NewMessage("a-" + strconv.Itoa(i+1))
				_, err := broker1.PushJob(testInstance, &job.JobRequest{
					WorkerType: "test",
					Message:    msg,
				})
				assert.NoError(t, err)
				time.Sleep(randomMicro(0, v))
			}
			w.Done()
		}()

		go func() {
			for i := 0; i < n; i++ {
				w.Add(1)
				msg, _ := job.NewMessage("b-" + strconv.Itoa(i+1))
				_, err := broker2.PushJob(testInstance, &job.JobRequest{
					WorkerType: "test",
					Message:    msg,
				})
				assert.NoError(t, err)
				time.Sleep(randomMicro(0, v))
			}
			w.Done()
		}()

		w.Wait()
	})

	t.Run("UnknownWorkerError", func(t *testing.T) {
		broker := job.NewMemBroker()
		assert.NoError(t, broker.StartWorkers(job.WorkersList{}))
		_, err := broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "nope",
			Message:    nil,
		})
		assert.Error(t, err)
		assert.Equal(t, job.ErrUnknownWorker, err)
	})

	t.Run("UnknownMessageType", func(t *testing.T) {
		var w sync.WaitGroup

		broker := job.NewMemBroker()
		assert.NoError(t, broker.StartWorkers(job.WorkersList{
			{
				WorkerType:  "test",
				Concurrency: 4,
				WorkerFunc: func(ctx *job.TaskContext) error {
					var msg string
					err := ctx.UnmarshalMessage(&msg)
					assert.Error(t, err)
					assert.Equal(t, job.ErrMessageNil, err)
					w.Done()
					return nil
				},
			},
		}))

		w.Add(1)
		_, err := broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "test",
			Message:    nil,
		})

		assert.NoError(t, err)
		w.Wait()
	})

	t.Run("Timeout", func(t *testing.T) {
		var w sync.WaitGroup

		broker := job.NewMemBroker()
		assert.NoError(t, broker.StartWorkers(job.WorkersList{
			{
				WorkerType:   "timeout",
				Concurrency:  1,
				MaxExecCount: 1,
				Timeout:      1 * time.Millisecond,
				WorkerFunc: func(ctx *job.TaskContext) error {
					<-ctx.Done()
					w.Done()
					return ctx.Err()
				},
			},
		}))

		w.Add(1)
		_, err := broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "timeout",
			Message:    nil,
		})

		assert.NoError(t, err)
		w.Wait()
	})

	t.Run("Retry", func(t *testing.T) {
		var w sync.WaitGroup

		maxExecCount := 4

		var count int
		broker := job.NewMemBroker()
		assert.NoError(t, broker.StartWorkers(job.WorkersList{
			{
				WorkerType:   "test",
				Concurrency:  1,
				MaxExecCount: maxExecCount,
				Timeout:      1 * time.Millisecond,
				RetryDelay:   1 * time.Millisecond,
				WorkerFunc: func(ctx *job.TaskContext) error {
					<-ctx.Done()
					w.Done()
					count++
					if count < maxExecCount {
						return ctx.Err()
					}
					return nil
				},
			},
		}))

		w.Add(maxExecCount)
		_, err := broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "test",
			Message:    nil,
		})

		assert.NoError(t, err)
		w.Wait()
	})

	t.Run("PanicRetried", func(t *testing.T) {
		var w sync.WaitGroup

		maxExecCount := 4

		broker := job.NewMemBroker()
		assert.NoError(t, broker.StartWorkers(job.WorkersList{
			{
				WorkerType:   "panic",
				Concurrency:  1,
				MaxExecCount: maxExecCount,
				RetryDelay:   1 * time.Millisecond,
				WorkerFunc: func(ctx *job.TaskContext) error {
					w.Done()
					panic("oops")
				},
			},
		}))

		w.Add(maxExecCount)
		_, err := broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "panic",
			Message:    nil,
		})

		assert.NoError(t, err)
		w.Wait()
	})

	t.Run("Panic", func(t *testing.T) {
		var w sync.WaitGroup

		even, _ := job.NewMessage(0)
		odd, _ := job.NewMessage(1)

		broker := job.NewMemBroker()
		assert.NoError(t, broker.StartWorkers(job.WorkersList{
			{
				WorkerType:   "panic2",
				Concurrency:  1,
				MaxExecCount: 1,
				RetryDelay:   1 * time.Millisecond,
				WorkerFunc: func(ctx *job.TaskContext) error {
					var i int
					if err := ctx.UnmarshalMessage(&i); err != nil {
						return err
					}
					if i%2 != 0 {
						panic("oops")
					}
					w.Done()
					return nil
				},
			},
		}))
		w.Add(2)
		var err error
		_, err = broker.PushJob(testInstance, &job.JobRequest{WorkerType: "panic2", Message: odd})
		assert.NoError(t, err)
		_, err = broker.PushJob(testInstance, &job.JobRequest{WorkerType: "panic2", Message: even})
		assert.NoError(t, err)
		_, err = broker.PushJob(testInstance, &job.JobRequest{WorkerType: "panic2", Message: odd})
		assert.NoError(t, err)
		_, err = broker.PushJob(testInstance, &job.JobRequest{WorkerType: "panic2", Message: even})
		assert.NoError(t, err)
		w.Wait()
	})

	t.Run("MemAddJobRateLimitExceeded", func(t *testing.T) {
		workersTestList := job.WorkersList{
			{
				WorkerType:  "thumbnail",
				Concurrency: 4,
				WorkerFunc: func(ctx *job.TaskContext) error {
					return nil
				},
			},
		}
		broker := job.NewMemBroker()
		err := broker.StartWorkers(workersTestList)
		assert.NoError(t, err)

		msg, _ := job.NewMessage("z-0")
		j, err := broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "thumbnail",
			Message:    msg,
		})

		assert.NoError(t, err)
		assert.NotNil(t, j)

		ct := limits.JobThumbnailType
		limits.SetMaximumLimit(ct, 10)
		maxLimit := limits.GetMaximumLimit(ct)
		// Blocking the job push
		for i := int64(0); i < maxLimit-1; i++ {
			j, err := broker.PushJob(testInstance, &job.JobRequest{
				WorkerType: "thumbnail",
				Message:    msg,
			})
			assert.NoError(t, err)
			assert.NotNil(t, j)
		}

		j, err = broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "thumbnail",
			Message:    msg,
		})
		assert.Error(t, err)
		assert.Nil(t, j)
		assert.ErrorIs(t, err, limits.ErrRateLimitReached)

		j, err = broker.PushJob(testInstance, &job.JobRequest{
			WorkerType: "thumbnail",
			Message:    msg,
		})
		assert.Error(t, err)
		assert.Nil(t, j)
	})
}
