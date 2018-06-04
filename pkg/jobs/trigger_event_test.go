package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/stretchr/testify/assert"
)

func makeMessage(t *testing.T, msg string) Message {
	out, err := NewMessage(msg)
	assert.NoError(t, err)
	return out
}

func TestTriggerEvent(t *testing.T) {
	var wg sync.WaitGroup
	var called = make(map[string]bool)

	bro := NewMemBroker()
	bro.StartWorkers(WorkersList{
		{
			WorkerType:   "worker_event",
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx *WorkerContext) error {
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
				assert.Equal(t, "cozy.local.triggerevent", evt.Domain)
				assert.Equal(t, "CREATED", evt.Verb)
				assert.Equal(t, "test-id", evt.Doc.ID())
				called[msg] = true
				return nil
			},
		},
	})

	db := prefixer.NewPrefixer("cozy.local.triggerevent", "cozy.local.triggerevent")

	var triggers []Trigger
	triggersInfos := []TriggerInfos{
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

	sch := newMemScheduler()
	sch.StartScheduler(bro)

	for _, infos := range triggersInfos {
		trigger, err := NewTrigger(db, infos, infos.Message)
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
		realtime.GetHub().Publish(db, realtime.EventCreate, &doc, nil)
	})

	wg.Wait()

	assert.True(t, called["message-wholetype"])
	assert.True(t, called["message-correct-verb"])
	assert.True(t, called["message-correct-verb-correct-value"])
	assert.False(t, called["message-bad-verb"])
	assert.False(t, called["message-correct-verb-bad-value"])

	for _, trigger := range triggers {
		err := sch.DeleteTrigger(db, trigger.ID())
		assert.NoError(t, err)
	}

	err := sch.ShutdownScheduler(context.Background())
	assert.NoError(t, err)
}
