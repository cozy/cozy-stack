package jobs

import (
	"context"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/utils"
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

func TestPanicRetried(t *testing.T) {
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

func TestPanic(t *testing.T) {
	var w sync.WaitGroup

	even, _ := NewMessage("json", 0)
	odd, _ := NewMessage("json", 1)

	broker := NewMemBroker("panic2", WorkersList{
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
	_, _, err = broker.PushJob(&JobRequest{WorkerType: "panic2", Message: odd})
	assert.NoError(t, err)
	_, _, err = broker.PushJob(&JobRequest{WorkerType: "panic2", Message: even})
	assert.NoError(t, err)
	_, _, err = broker.PushJob(&JobRequest{WorkerType: "panic2", Message: odd})
	assert.NoError(t, err)
	_, _, err = broker.PushJob(&JobRequest{WorkerType: "panic2", Message: even})
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

type storage struct {
	ts []Trigger
}

func (s *storage) GetAll() ([]Trigger, error)   { return s.ts, nil }
func (s *storage) Add(trigger Trigger) error    { return nil }
func (s *storage) Delete(trigger Trigger) error { return nil }

func TestTriggersBadArguments(t *testing.T) {
	var err error
	_, err = NewTrigger(&TriggerInfos{
		ID:        utils.RandomString(10),
		Type:      "@at",
		Arguments: "garbage",
	})
	assert.Error(t, err)

	_, err = NewTrigger(&TriggerInfos{
		ID:        utils.RandomString(10),
		Type:      "@in",
		Arguments: "garbage",
	})
	assert.Error(t, err)

	_, err = NewTrigger(&TriggerInfos{
		ID:        utils.RandomString(10),
		Type:      "@interval",
		Arguments: "garbage",
	})
	assert.Error(t, err)

	_, err = NewTrigger(&TriggerInfos{
		ID:        utils.RandomString(10),
		Type:      "@unknown",
		Arguments: "",
	})
	if assert.Error(t, err) {
		assert.Equal(t, ErrUnknownTrigger, err)
	}
}

func TestMemSchedulerWithTimeTriggers(t *testing.T) {
	var wAt sync.WaitGroup
	var wIn sync.WaitGroup
	var wInterval sync.WaitGroup
	NewMemBroker("test.scheduler.io", WorkersList{
		"worker": {
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, m *Message) error {
				var msg string
				if err := m.Unmarshal(&msg); err != nil {
					return err
				}
				switch msg {
				case "@at":
					wAt.Done()
				case "@in":
					wIn.Done()
				case "@interval":
					wInterval.Done()
				}
				return nil
			},
		},
	})

	msg1, _ := NewMessage("json", "@at")
	msg2, _ := NewMessage("json", "@in")
	msg3, _ := NewMessage("json", "@interval")

	wAt.Add(1)       // 1 time in @at
	wIn.Add(1)       // 1 time in @in
	wInterval.Add(2) // 2 times in @interval

	atID := utils.RandomString(10)
	at, err := NewTrigger(&TriggerInfos{
		ID:         atID,
		Type:       "@at",
		Arguments:  time.Now().Add(2 * time.Second).Format(time.RFC3339),
		WorkerType: "worker",
		Message:    msg1,
	})
	if !assert.NoError(t, err) {
		return
	}
	inID := utils.RandomString(10)
	in, err := NewTrigger(&TriggerInfos{
		ID:         inID,
		Type:       "@in",
		Arguments:  "1s",
		WorkerType: "worker",
		Message:    msg2,
	})
	if !assert.NoError(t, err) {
		return
	}
	intervalID := utils.RandomString(10)
	interval, err := NewTrigger(&TriggerInfos{
		ID:         intervalID,
		Type:       "@interval",
		Arguments:  "1s",
		WorkerType: "worker",
		Message:    msg3,
	})
	if !assert.NoError(t, err) {
		return
	}

	triggers := []Trigger{at, in, interval}
	NewMemScheduler("test.scheduler.io", &storage{triggers})

	bro := GetMemBroker("test.scheduler.io")
	sch := GetMemScheduler("test.scheduler.io")
	sch.Start(bro)

	ts, err := sch.GetAll()
	assert.NoError(t, err)
	assert.Len(t, ts, len(triggers))

	for _, trigger := range ts {
		switch trigger.Infos().ID {
		case atID:
			assert.Equal(t, at.Infos(), trigger.Infos())
		case inID:
			assert.Equal(t, in.Infos(), trigger.Infos())
		case intervalID:
			assert.Equal(t, interval.Infos(), trigger.Infos())
		default:
			t.Fatalf("unknown trigger ID %s", trigger.Infos().ID)
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

	go func() {
		wInterval.Wait()
		done <- true
	}()

	for i := 0; i < len(ts); i++ {
		<-done
	}

	_, err = sch.Get(atID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotFoundTrigger, err)

	_, err = sch.Get(inID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotFoundTrigger, err)

	interval2, err := sch.Get(intervalID)
	assert.NoError(t, err)
	assert.Equal(t, interval, interval2)

	err = sch.Delete(intervalID)
	assert.NoError(t, err)

	_, err = sch.Get(intervalID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotFoundTrigger, err)
}
