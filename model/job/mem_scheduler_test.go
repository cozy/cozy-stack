package job_test

import (
	"context"
	"testing"
	"time"

	jobs "github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/stretchr/testify/assert"
)

func TestTriggersBadArguments(t *testing.T) {
	var err error
	_, err = jobs.NewTrigger(testInstance, jobs.TriggerInfos{
		Type:      "@at",
		Arguments: "garbage",
	}, nil)
	assert.Error(t, err)

	_, err = jobs.NewTrigger(testInstance, jobs.TriggerInfos{
		Type:      "@in",
		Arguments: "garbage",
	}, nil)
	assert.Error(t, err)

	_, err = jobs.NewTrigger(testInstance, jobs.TriggerInfos{
		Type:      "@unknown",
		Arguments: "",
	}, nil)
	if assert.Error(t, err) {
		assert.Equal(t, jobs.ErrUnknownTrigger, err)
	}
}

func TestMemSchedulerWithDebounce(t *testing.T) {
	called := 0
	bro := jobs.NewMemBroker()
	assert.NoError(t, bro.StartWorkers(jobs.WorkersList{
		{
			WorkerType:   "worker",
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx *jobs.WorkerContext) error {
				called++
				return nil
			},
		},
	}))

	msg, _ := jobs.NewMessage("@event")
	ti := jobs.TriggerInfos{
		Type:       "@event",
		Arguments:  "io.cozy.testdebounce io.cozy.moredebounce",
		Debounce:   "2s",
		WorkerType: "worker",
		Message:    msg,
	}

	var triggers []jobs.Trigger
	triggersInfos := []jobs.TriggerInfos{ti}
	sch := jobs.NewMemScheduler()
	assert.NoError(t, sch.StartScheduler(bro))

	// Clear the existing triggers before testing with our triggers
	ts, err := sch.GetAllTriggers(testInstance)
	assert.NoError(t, err)
	for _, trigger := range ts {
		err = sch.DeleteTrigger(testInstance, trigger.ID())
		assert.NoError(t, err)
	}

	for _, infos := range triggersInfos {
		trigger, err := jobs.NewTrigger(testInstance, infos, msg)
		if !assert.NoError(t, err) {
			return
		}
		err = sch.AddTrigger(trigger)
		if !assert.NoError(t, err) {
			return
		}
		triggers = append(triggers, trigger)
	}

	ts, err = sch.GetAllTriggers(testInstance)
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
		realtime.GetHub().Publish(testInstance, realtime.EventCreate, &doc, nil)
	}

	time.Sleep(3000 * time.Millisecond)
	assert.Equal(t, 3, called)

	realtime.GetHub().Publish(testInstance, realtime.EventCreate, &doc, nil)
	doc.Type = "io.cozy.moredebounce"
	realtime.GetHub().Publish(testInstance, realtime.EventCreate, &doc, nil)
	time.Sleep(3000 * time.Millisecond)
	assert.Equal(t, 4, called)

	for _, trigger := range triggers {
		err = sch.DeleteTrigger(testInstance, trigger.ID())
		assert.NoError(t, err)
	}

	err = sch.ShutdownScheduler(context.Background())
	assert.NoError(t, err)
}
