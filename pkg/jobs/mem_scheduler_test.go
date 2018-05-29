package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
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

func TestMemSchedulerWithDebounce(t *testing.T) {
	called := 0
	bro := NewMemBroker()
	bro.StartWorkers(WorkersList{
		{
			WorkerType:   "worker",
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx *WorkerContext) error {
				called++
				return nil
			},
		},
	})

	msg, _ := NewMessage("@event")
	ti := &TriggerInfos{
		Type:       "@event",
		Domain:     "cozy.local.withdebounce",
		Arguments:  "io.cozy.testdebounce io.cozy.moredebounce",
		Debounce:   "2s",
		WorkerType: "worker",
		Message:    msg,
	}

	triggers := []*TriggerInfos{ti}
	sch := newMemScheduler()
	sch.StartScheduler(bro)

	for _, infos := range triggers {
		trigger, err := NewTrigger(infos)
		if !assert.NoError(t, err) {
			return
		}
		err = sch.AddTrigger(trigger)
		if !assert.NoError(t, err) {
			return
		}
	}

	ts, err := sch.GetAllTriggers("cozy.local.withdebounce")
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
		time.Sleep(200 * time.Millisecond)
		realtime.GetHub().Publish(event)
	}

	time.Sleep(3000 * time.Millisecond)
	assert.Equal(t, 3, called)

	realtime.GetHub().Publish(event)
	doc.Type = "io.cozy.moredebounce"
	realtime.GetHub().Publish(event)
	time.Sleep(3000 * time.Millisecond)
	assert.Equal(t, 4, called)

	for _, trigger := range triggers {
		err = sch.DeleteTrigger(trigger.Domain, trigger.TID)
		assert.NoError(t, err)
	}

	err = sch.ShutdownScheduler(context.Background())
	assert.NoError(t, err)
}
