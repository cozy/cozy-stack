package job_test

import (
	"context"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	jobs "github.com/cozy/cozy-stack/model/job"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL1 = "redis://localhost:6379/0"
const redisURL2 = "redis://localhost:6379/1"

func randomMicro(min, max int) time.Duration {
	return time.Duration(rand.Intn(max-min)+min) * time.Microsecond
}

func TestRedisJobs(t *testing.T) {
	// redisBRPopTimeout = 1 * time.Second
	opts1, _ := redis.ParseURL(redisURL1)
	opts2, _ := redis.ParseURL(redisURL2)
	client1 := redis.NewClient(opts1)
	client2 := redis.NewClient(opts2)

	n := 10
	v := 100

	var w sync.WaitGroup
	w.Add(2*n + 1)

	var workersTestList = jobs.WorkersList{
		{
			WorkerType:  "test",
			Concurrency: 4,
			WorkerFunc: func(ctx *jobs.WorkerContext) error {
				var msg string
				err := ctx.UnmarshalMessage(&msg)
				if !assert.NoError(t, err) {
					return err
				}
				if strings.HasPrefix(msg, "z-") {
					_, err := strconv.Atoi(msg[len("z-"):])
					assert.NoError(t, err)
				} else if strings.HasPrefix(msg, "a-") {
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

	broker1 := jobs.NewRedisBroker(client1)
	err := broker1.StartWorkers(workersTestList)
	assert.NoError(t, err)

	broker2 := jobs.NewRedisBroker(client2)
	err = broker2.StartWorkers(workersTestList)
	assert.NoError(t, err)

	msg, _ := jobs.NewMessage("z-0")
	_, err = broker1.PushJob(testInstance, &jobs.JobRequest{
		WorkerType: "test",
		Message:    msg,
	})
	assert.NoError(t, err)

	go func() {
		for i := 0; i < n; i++ {
			msg, _ := jobs.NewMessage("a-" + strconv.Itoa(i+1))
			_, err = broker1.PushJob(testInstance, &jobs.JobRequest{
				WorkerType: "test",
				Message:    msg,
			})
			assert.NoError(t, err)
			time.Sleep(randomMicro(0, v))
		}
	}()

	go func() {
		for i := 0; i < n; i++ {
			msg, _ := jobs.NewMessage("b-" + strconv.Itoa(i+1))
			_, err = broker2.PushJob(testInstance, &jobs.JobRequest{
				WorkerType: "test",
				Message:    msg,
				Manual:     true,
			})
			assert.NoError(t, err)
			time.Sleep(randomMicro(0, v))
		}
	}()

	w.Wait()

	err = broker1.ShutdownWorkers(context.Background())
	assert.NoError(t, err)
	err = broker2.ShutdownWorkers(context.Background())
	assert.NoError(t, err)
	time.Sleep(1 * time.Second)
}
