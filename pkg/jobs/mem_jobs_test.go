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
	"github.com/stretchr/testify/assert"
)

func randomMicro(min, max int) time.Duration {
	return time.Duration(rand.Intn(max-min)+min) * time.Microsecond
}

func TestInMemoryJobs(t *testing.T) {
	n := 10
	v := 100

	var w sync.WaitGroup

	var workersTestList = WorkersList{
		"test": {
			Concurrency: 4,
			WorkerFunc: func(ctx context.Context, m *Message) error {
				var msg string
				err := m.Unmarshal(&msg)
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

	w.Add(2)

	go func() {
		broker := NewMemBroker(workersTestList)
		for i := 0; i < n; i++ {
			w.Add(1)
			msg, _ := NewMessage(JSONEncoding, "a-"+strconv.Itoa(i+1))
			_, err := broker.PushJob(&JobRequest{
				Domain:     "cozy.local",
				WorkerType: "test",
				Message:    msg,
			})
			assert.NoError(t, err)
			time.Sleep(randomMicro(0, v))
		}
		w.Done()
	}()

	go func() {
		broker := NewMemBroker(workersTestList)
		for i := 0; i < n; i++ {
			w.Add(1)
			msg, _ := NewMessage(JSONEncoding, "b-"+strconv.Itoa(i+1))
			_, err := broker.PushJob(&JobRequest{
				Domain:     "cozy.local",
				WorkerType: "test",
				Message:    msg,
			})
			assert.NoError(t, err)
			time.Sleep(randomMicro(0, v))
		}
		w.Done()
	}()

	w.Wait()
}

func TestUnknownWorkerError(t *testing.T) {
	broker := NewMemBroker(WorkersList{})
	_, err := broker.PushJob(&JobRequest{
		Domain:     "cozy.local",
		WorkerType: "nope",
		Message:    nil,
	})
	assert.Error(t, err)
	assert.Equal(t, ErrUnknownWorker, err)
}

func TestUnknownMessageType(t *testing.T) {
	var w sync.WaitGroup

	broker := NewMemBroker(WorkersList{
		"test": {
			Concurrency: 4,
			WorkerFunc: func(ctx context.Context, m *Message) error {
				var msg string
				err := m.Unmarshal(&msg)
				assert.Error(t, err)
				assert.Equal(t, ErrUnknownMessageType, err)
				w.Done()
				return nil
			},
		},
	})

	w.Add(1)
	_, err := broker.PushJob(&JobRequest{
		WorkerType: "test",
		Domain:     "cozy.local",
		Message: &Message{
			Type: "unknown",
			Data: nil,
		},
	})

	assert.NoError(t, err)
	w.Wait()
}

func TestTimeout(t *testing.T) {
	var w sync.WaitGroup

	broker := NewMemBroker(WorkersList{
		"timeout": {
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, _ *Message) error {
				<-ctx.Done()
				w.Done()
				return ctx.Err()
			},
		},
	})

	w.Add(1)
	_, err := broker.PushJob(&JobRequest{
		WorkerType: "timeout",
		Domain:     "cozy.local",
		Message: &Message{
			Type: "timeout",
			Data: nil,
		},
	})

	assert.NoError(t, err)
	w.Wait()
}

func TestRetry(t *testing.T) {
	var w sync.WaitGroup

	maxExecCount := 4

	var count int
	broker := NewMemBroker(WorkersList{
		"test": {
			Concurrency:  1,
			MaxExecCount: uint(maxExecCount),
			Timeout:      1 * time.Millisecond,
			RetryDelay:   1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, _ *Message) error {
				<-ctx.Done()
				w.Done()
				count++
				if count < maxExecCount {
					return ctx.Err()
				}
				return nil
			},
		},
	})

	w.Add(maxExecCount)
	_, err := broker.PushJob(&JobRequest{
		Domain:     "cozy.local",
		WorkerType: "test",
		Message:    nil,
	})

	assert.NoError(t, err)
	w.Wait()
}

func TestPanicRetried(t *testing.T) {
	var w sync.WaitGroup

	maxExecCount := 4

	broker := NewMemBroker(WorkersList{
		"panic": {
			Concurrency:  1,
			MaxExecCount: uint(maxExecCount),
			RetryDelay:   1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, _ *Message) error {
				w.Done()
				panic("oops")
			},
		},
	})

	w.Add(maxExecCount)
	_, err := broker.PushJob(&JobRequest{
		Domain:     "cozy.local",
		WorkerType: "panic",
		Message:    nil,
	})

	assert.NoError(t, err)
	w.Wait()
}

func TestPanic(t *testing.T) {
	var w sync.WaitGroup

	even, _ := NewMessage("json", 0)
	odd, _ := NewMessage("json", 1)

	broker := NewMemBroker(WorkersList{
		"panic2": {
			Concurrency:  1,
			MaxExecCount: 1,
			RetryDelay:   1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, m *Message) error {
				var i int
				if err := m.Unmarshal(&i); err != nil {
					return err
				}
				if i%2 != 0 {
					panic("oops")
				}
				w.Done()
				return nil
			},
		},
	})
	w.Add(2)
	var err error
	_, err = broker.PushJob(&JobRequest{Domain: "cozy.local", WorkerType: "panic2", Message: odd})
	assert.NoError(t, err)
	_, err = broker.PushJob(&JobRequest{Domain: "cozy.local", WorkerType: "panic2", Message: even})
	assert.NoError(t, err)
	_, err = broker.PushJob(&JobRequest{Domain: "cozy.local", WorkerType: "panic2", Message: odd})
	assert.NoError(t, err)
	_, err = broker.PushJob(&JobRequest{Domain: "cozy.local", WorkerType: "panic2", Message: even})
	assert.NoError(t, err)
	w.Wait()
}

func TestProperSerial(t *testing.T) {
	infos := NewJobInfos(&JobRequest{
		Domain:     "cozy.tools:8080",
		WorkerType: "",
	})

	j := &memJob{
		infos: infos,
	}
	globalStorage.Create(infos)
	err := j.AckConsumed()
	assert.NoError(t, err)
	j2, err := globalStorage.Get("cozy.tools:8080", j.infos.ID())
	assert.NoError(t, err)
	assert.Equal(t, State(Running), j2.State)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	fmt.Println(config.CouchURL())
	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
