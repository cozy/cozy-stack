package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/stretchr/testify/assert"
)

func TestTriggersBadArguments(t *testing.T) {
	var err error
	_, err = NewTrigger(&TriggerInfos{
		Domain:    "cozy.local",
		Type:      "@at",
		Arguments: "garbage",
	})
	assert.Error(t, err)

	_, err = NewTrigger(&TriggerInfos{
		Type:      "@in",
		Arguments: "garbage",
	})
	assert.Error(t, err)

	_, err = NewTrigger(&TriggerInfos{
		Domain:    "cozy.local",
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

	at := &TriggerInfos{
		Type:       "@at",
		Domain:     "cozy.local.memsched_timetriggers",
		Arguments:  time.Now().Add(2 * time.Second).Format(time.RFC3339),
		WorkerType: "worker",
		Message:    msg1,
	}
	in := &TriggerInfos{
		Domain:     "cozy.local.memsched_timetriggers",
		Type:       "@in",
		Arguments:  "1s",
		WorkerType: "worker",
		Message:    msg2,
	}

	triggers := []*TriggerInfos{at, in}
	sch := newMemScheduler()
	sch.Start(bro)

	for _, infos := range triggers {
		trigger, err := NewTrigger(infos)
		if !assert.NoError(t, err) {
			return
		}
		err = sch.Add(trigger)
		if !assert.NoError(t, err) {
			return
		}
	}

	ts, err := sch.GetAll("cozy.local.memsched_timetriggers")
	assert.NoError(t, err)
	assert.Len(t, ts, len(triggers))

	for _, trigger := range ts {
		switch trigger.Infos().Type {
		case "@at":
			assert.EqualValues(t, at, trigger.Infos())
		case "@in":
			assert.EqualValues(t, in, trigger.Infos())
		default:
			t.Fatalf("unknown trigger ID %s", trigger.Infos().TID)
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

	for i := 0; i < len(ts); i++ {
		<-done
	}

	for _, trigger := range triggers {
		err = sch.Delete(trigger.Domain, trigger.TID)
		assert.NoError(t, err)
	}

	err = sch.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestMemSchedulerWithDebounce(t *testing.T) {
	called := 0
	bro := jobs.NewMemBroker(1)
	bro.Start(jobs.WorkersList{
		"worker": {
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, m *jobs.Message) error {
				called++
				return nil
			},
		},
	})

	msg, _ := jobs.NewMessage("json", "@event")
	ti := &TriggerInfos{
		Type:       "@event",
		Domain:     "cozy.local.withdebounce",
		Arguments:  "io.cozy.testdebounce",
		Debounce:   "100ms",
		WorkerType: "worker",
		Message:    msg,
	}

	triggers := []*TriggerInfos{ti}
	sch := newMemScheduler()
	sch.Start(bro)

	for _, infos := range triggers {
		trigger, err := NewTrigger(infos)
		if !assert.NoError(t, err) {
			return
		}
		err = sch.Add(trigger)
		if !assert.NoError(t, err) {
			return
		}
	}

	ts, err := sch.GetAll("cozy.local.withdebounce")
	assert.NoError(t, err)
	assert.Len(t, ts, len(triggers))

	doc := couchdb.JSONDoc{
		Type: "io.cozy.testdebounce",
		M: map[string]interface{}{
			"_id":  "test-id",
			"_rev": "1-xxabxx",
			"test": "value",
		},
	}
	event := &realtime.Event{
		Verb:   realtime.EventCreate,
		Doc:    &doc,
		Domain: "cozy.local.withdebounce",
	}

	for i := 0; i < 24; i++ {
		time.Sleep(10 * time.Millisecond)
		realtime.GetHub().Publish(event)
	}

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 3, called)

	for _, trigger := range triggers {
		err = sch.Delete(trigger.Domain, trigger.TID)
		assert.NoError(t, err)
	}

	err = sch.Shutdown(context.Background())
	assert.NoError(t, err)
}
