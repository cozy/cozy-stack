package jobs

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL1 = "redis://localhost:6379/0"
const redisURL2 = "redis://localhost:6379/1"

var client1 *redis.Client
var client2 *redis.Client

func randomMicro(min, max int) time.Duration {
	return time.Duration(rand.Intn(max-min)+min) * time.Microsecond
}

func TestRedisJobs(t *testing.T) {
	n := 10
	v := 100

	var w sync.WaitGroup
	w.Add(2*n + 1)

	var workersTestList = WorkersList{
		{
			WorkerType:  "test",
			Concurrency: 4,
			WorkerFunc: func(ctx *WorkerContext) error {
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

	broker1 := NewRedisBroker(client1)
	err := broker1.Start(workersTestList)
	assert.NoError(t, err)

	broker2 := NewRedisBroker(client2)
	err = broker2.Start(workersTestList)
	assert.NoError(t, err)

	msg, _ := NewMessage("z-0")
	_, err = broker1.PushJob(&JobRequest{
		Domain:     "cozy.local.redisjobs",
		WorkerType: "test",
		Message:    msg,
	})
	assert.NoError(t, err)

	go func() {
		for i := 0; i < n; i++ {
			msg, _ := NewMessage("a-" + strconv.Itoa(i+1))
			_, err = broker1.PushJob(&JobRequest{
				Domain:     "cozy.local.redisjobs",
				WorkerType: "test",
				Message:    msg,
			})
			assert.NoError(t, err)
			time.Sleep(randomMicro(0, v))
		}
	}()

	go func() {
		for i := 0; i < n; i++ {
			msg, _ := NewMessage("b-" + strconv.Itoa(i+1))
			_, err = broker2.PushJob(&JobRequest{
				Domain:     "cozy.local.redisjobs",
				WorkerType: "test",
				Message:    msg,
				Manual:     true,
			})
			assert.NoError(t, err)
			time.Sleep(randomMicro(0, v))
		}
	}()

	w.Wait()

	err = broker1.Shutdown(context.Background())
	assert.NoError(t, err)
	err = broker2.Shutdown(context.Background())
	assert.NoError(t, err)
	time.Sleep(1 * time.Second)
}

func TestMain(m *testing.M) {
	redisBRPopTimeout = 1 * time.Second
	config.UseTestFile()
	db, err := checkup.HTTPChecker{URL: config.CouchURL().String()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	opts1, _ := redis.ParseURL(redisURL1)
	opts2, _ := redis.ParseURL(redisURL2)
	client1 = redis.NewClient(opts1)
	client2 = redis.NewClient(opts2)
	os.Exit(m.Run())
}
