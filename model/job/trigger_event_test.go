package job_test

import (
	"context"
	"sync"
	"testing"
	"time"

	jobs "github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/stretchr/testify/assert"
)

func makeMessage(t *testing.T, msg string) jobs.Message {
	out, err := jobs.NewMessage(msg)
	assert.NoError(t, err)
	return out
}

func TestTriggerEvent(t *testing.T) {
	var wg sync.WaitGroup
	var called = make(map[string]bool)

	bro := jobs.NewMemBroker()
	assert.NoError(t, bro.StartWorkers(jobs.WorkersList{
		{
			WorkerType:   "worker_event",
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx *jobs.WorkerContext) error {
				defer wg.Done()
				var msg string
				if err := ctx.UnmarshalMessage(&msg); err != nil {
					assert.NoError(t, err)
					return err
				}
				var evt struct {
					Domain string `json:"domain"`
					Verb   string `json:"verb"`
					Doc    couchdb.JSONDoc
				}
				if err := ctx.UnmarshalEvent(&evt); err != nil {
					assert.NoError(t, err)
					return nil
				}
				assert.Equal(t, testInstance.Domain, evt.Domain)
				assert.Equal(t, "CREATED", evt.Verb)
				assert.Equal(t, "test-id", evt.Doc.ID())
				called[msg] = true
				return nil
			},
		},
	}))

	var triggers []jobs.Trigger
	triggersInfos := []jobs.TriggerInfos{
		{
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:DELETED",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-bad-verb"),
		},
		{
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:CREATED:value:test",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb-correct-value"),
		},
		{
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:CREATED",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb"),
		},
		{
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:CREATED:notvalue:test",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb-bad-value"),
		},
		{
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-wholetype"),
		},
	}

	sch := jobs.NewMemScheduler()
	assert.NoError(t, sch.StartScheduler(bro))

	for _, infos := range triggersInfos {
		trigger, err := jobs.NewTrigger(testInstance, infos, infos.Message)
		if !assert.NoError(t, err) {
			return
		}
		err = sch.AddTrigger(trigger)
		if !assert.NoError(t, err) {
			return
		}
		triggers = append(triggers, trigger)
	}

	wg.Add(3)

	time.AfterFunc(1*time.Millisecond, func() {
		doc := couchdb.JSONDoc{
			Type: "io.cozy.testeventobject",
			M: map[string]interface{}{
				"_id":  "test-id",
				"_rev": "1-xxabxx",
				"test": "value",
			},
		}
		realtime.GetHub().Publish(testInstance, realtime.EventCreate, &doc, nil)
	})

	wg.Wait()

	assert.True(t, called["message-wholetype"])
	assert.True(t, called["message-correct-verb"])
	assert.True(t, called["message-correct-verb-correct-value"])
	assert.False(t, called["message-bad-verb"])
	assert.False(t, called["message-correct-verb-bad-value"])

	for _, trigger := range triggers {
		err := sch.DeleteTrigger(testInstance, trigger.ID())
		assert.NoError(t, err)
	}

	err := sch.ShutdownScheduler(context.Background())
	assert.NoError(t, err)
}
