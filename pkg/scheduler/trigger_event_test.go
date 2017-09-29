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

func makeMessage(t *testing.T, msg string) *jobs.Message {
	out, err := jobs.NewMessage("json", msg)
	assert.NoError(t, err)
	return out
}

func TestTriggerEvent(t *testing.T) {
	var wg sync.WaitGroup
	var called = make(map[string]bool)

	bro := jobs.NewMemBroker(1)
	bro.Start(jobs.WorkersList{
		"worker_event": {
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, m *jobs.Message) error {
				defer wg.Done()
				var msg struct {
					Message string
					Event   struct {
						Type string
						Doc  *couchdb.JSONDoc
					}
				}
				if err := m.Unmarshal(&msg); err != nil {
					assert.NoError(t, err)
					return err
				}
				assert.Equal(t, "test-id", msg.Event.Doc.ID())
				called[msg.Message] = true
				return nil
			},
		},
	})

	triggers := []*TriggerInfos{
		{
			Type:       "@event",
			Domain:     "cozy.local.triggerevent",
			Arguments:  "io.cozy.testeventobject:DELETED",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-bad-verb"),
		},
		{
			Type:       "@event",
			Domain:     "cozy.local.triggerevent",
			Arguments:  "io.cozy.testeventobject:CREATED:value:test",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb-correct-value"),
		},
		{
			Type:       "@event",
			Domain:     "cozy.local.triggerevent",
			Arguments:  "io.cozy.testeventobject:CREATED",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb"),
		},
		{
			Type:       "@event",
			Domain:     "cozy.local.triggerevent",
			Arguments:  "io.cozy.testeventobject:CREATED:notvalue:test",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb-bad-value"),
		},
		{
			Type:       "@event",
			Domain:     "cozy.local.triggerevent",
			Arguments:  "io.cozy.testeventobject",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-wholetype"),
		},
	}

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
		realtime.GetHub().Publish(&realtime.Event{
			Verb:   realtime.EventCreate,
			Doc:    &doc,
			Domain: "cozy.local.triggerevent",
		})
	})

	wg.Wait()

	assert.True(t, called["message-wholetype"])
	assert.True(t, called["message-correct-verb"])
	assert.True(t, called["message-correct-verb-correct-value"])
	assert.False(t, called["message-bad-verb"])
	assert.False(t, called["message-correct-verb-bad-value"])

	for _, trigger := range triggers {
		err := sch.Delete(trigger.Domain, trigger.TID)
		assert.NoError(t, err)
	}

	err := sch.Shutdown(context.Background())
	assert.NoError(t, err)
}
