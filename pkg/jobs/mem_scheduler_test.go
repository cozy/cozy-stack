package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/stretchr/testify/assert"
)

func TestTriggersBadArguments(t *testing.T) {
	var err error
	_, err = NewTrigger(localDB, TriggerInfos{
		Type:      "@at",
		Arguments: "garbage",
	}, nil)
	assert.Error(t, err)

	_, err = NewTrigger(localDB, TriggerInfos{
		Type:      "@in",
		Arguments: "garbage",
	}, nil)
	assert.Error(t, err)

	_, err = NewTrigger(localDB, TriggerInfos{
		Type:      "@unknown",
		Arguments: "",
	}, nil)
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
	ti := TriggerInfos{
		Type:       "@event",
		Arguments:  "io.cozy.testdebounce io.cozy.moredebounce",
		Debounce:   "2s",
		WorkerType: "worker",
		Message:    msg,
	}

	var triggers []Trigger
	triggersInfos := []TriggerInfos{ti}
	sch := newMemScheduler()
	sch.StartScheduler(bro)

	db := prefixer.NewPrefixer("cozy.local.withdebounce", "cozy.local.withdebounce")

	for _, infos := range triggersInfos {
		trigger, err := NewTrigger(db, infos, msg)
		if !assert.NoError(t, err) {
			return
		}
		err = sch.AddTrigger(trigger)
		if !assert.NoError(t, err) {
			return
		}
		triggers = append(triggers, trigger)
	}

	ts, err := sch.GetAllTriggers(db)
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

	for i := 0; i < 24; i++ {
		time.Sleep(200 * time.Millisecond)
		realtime.GetHub().Publish(db, realtime.EventCreate, &doc, nil)
	}

	time.Sleep(3000 * time.Millisecond)
	assert.Equal(t, 3, called)

	realtime.GetHub().Publish(db, realtime.EventCreate, &doc, nil)
	doc.Type = "io.cozy.moredebounce"
	realtime.GetHub().Publish(db, realtime.EventCreate, &doc, nil)
	time.Sleep(3000 * time.Millisecond)
	assert.Equal(t, 4, called)

	for _, trigger := range triggers {
		err = sch.DeleteTrigger(db, trigger.ID())
		assert.NoError(t, err)
	}

	err = sch.ShutdownScheduler(context.Background())
	assert.NoError(t, err)
}
