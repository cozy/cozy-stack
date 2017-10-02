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

const redisURL = "redis://localhost:6379/15"

var client *redis.Client

func randomMicro(min, max int) time.Duration {
	return time.Duration(rand.Intn(max-min)+min) * time.Microsecond
}

func TestRedisJobs(t *testing.T) {
	n := 10
	v := 100

	var w sync.WaitGroup
	w.Add(2*n + 1)

	var workersTestList = WorkersList{
		"test": {
			Concurrency: 4,
			WorkerFunc: func(ctx context.Context, m Message) error {
				var msg string
				err := m.Unmarshal(&msg)
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

	broker1 := NewRedisBroker(4, client)
	err := broker1.Start(workersTestList)
	assert.NoError(t, err)

	broker2 := NewRedisBroker(4, client)
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

	opts, _ := redis.ParseURL(redisURL)
	client = redis.NewClient(opts)
	os.Exit(m.Run())
}
