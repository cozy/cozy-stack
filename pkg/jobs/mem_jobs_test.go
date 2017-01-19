package jobs

import (
	"context"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func randomMicro(min, max int) time.Duration {
	return time.Duration(rand.Intn(max-min)+min) * time.Microsecond
}

func TestInMemoryJobs(t *testing.T) {
	n := 1000
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
		broker := NewMemBroker("cozy.local", workersTestList)
		for i := 0; i < n; i++ {
			w.Add(1)
			msg, _ := NewMessage(JSONEncoding, "a-"+strconv.Itoa(i+1))
			_, _, err := broker.PushJob(&JobRequest{
				WorkerType: "test",
				Message:    msg,
			})
			assert.NoError(t, err)
			time.Sleep(randomMicro(0, v))
		}
		w.Done()
	}()

	go func() {
		broker := NewMemBroker("cozy.local", workersTestList)
		for i := 0; i < n; i++ {
			w.Add(1)
			msg, _ := NewMessage(JSONEncoding, "b-"+strconv.Itoa(i+1))
			_, _, err := broker.PushJob(&JobRequest{
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
	broker := NewMemBroker("baz.quz", WorkersList{})
	_, _, err := broker.PushJob(&JobRequest{
		WorkerType: "nope",
		Message:    nil,
	})
	assert.Error(t, err)
	assert.Equal(t, ErrUnknownWorker, err)
}

func TestUnknownMessageType(t *testing.T) {
	var w sync.WaitGroup

	broker := NewMemBroker("foo.bar", WorkersList{
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
	_, _, err := broker.PushJob(&JobRequest{
		WorkerType: "test",
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

	broker := NewMemBroker("timeout.cozy", WorkersList{
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
	_, _, err := broker.PushJob(&JobRequest{
		WorkerType: "timeout",
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
	broker := NewMemBroker("retry", WorkersList{
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
	_, _, err := broker.PushJob(&JobRequest{
		WorkerType: "test",
		Message:    nil,
	})

	assert.NoError(t, err)
	w.Wait()
}

func TestPanic(t *testing.T) {
	var w sync.WaitGroup

	maxExecCount := 4

	broker := NewMemBroker("panic", WorkersList{
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
	_, _, err := broker.PushJob(&JobRequest{
		WorkerType: "panic",
		Message:    nil,
	})

	assert.NoError(t, err)
	w.Wait()
}

func TestInfoChan(t *testing.T) {
	var w sync.WaitGroup

	broker := NewMemBroker("chan.cozy", WorkersList{
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
	job, done, err := broker.PushJob(&JobRequest{
		WorkerType: "timeout",
		Message: &Message{
			Type: "timeout",
			Data: nil,
		},
	})

	assert.Equal(t, Queued, job.State)

	job = <-done
	assert.Equal(t, string(Running), string(job.State))

	job = <-done
	assert.Equal(t, string(Errored), string(job.State))

	job = <-done
	assert.Nil(t, job)

	assert.NoError(t, err)
	w.Wait()
}
