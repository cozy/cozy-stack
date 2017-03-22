package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func makeMessage(t *testing.T, msg string) *Message {
	out, err := NewMessage("json", msg)
	assert.NoError(t, err)
	return out
}

func TestTriggerEvent(t *testing.T) {

	var wg sync.WaitGroup
	var called = make(map[string]bool)

	NewMemBroker("test2.scheduler.io", WorkersList{
		"worker_event": {
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, m *Message) error {
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

	storage := &storage{[]*TriggerInfos{
		{
			ID:         utils.RandomString(10),
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:DELETED",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-bad-verb"),
		},
		{
			ID:         utils.RandomString(10),
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:CREATED:value:test",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb-correct-value"),
		},
		{
			ID:         utils.RandomString(10),
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:CREATED",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb"),
		},
		{
			ID:         utils.RandomString(10),
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject:CREATED:notvalue:test",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-correct-verb-bad-value"),
		},
		{
			ID:         utils.RandomString(10),
			Type:       "@event",
			Arguments:  "io.cozy.testeventobject",
			WorkerType: "worker_event",
			Message:    makeMessage(t, "message-wholetype"),
		},
	}}
	wg.Add(3)
	NewMemScheduler("test2.scheduler.io", storage)
	bro := GetMemBroker("test2.scheduler.io")
	sch := GetMemScheduler("test2.scheduler.io")
	sch.Start(bro)

	time.AfterFunc(1*time.Millisecond, func() {
		doc := couchdb.JSONDoc{
			Type: "io.cozy.testeventobject",
			M: map[string]interface{}{
				"_id":  "test-id",
				"_rev": "1-xxabxx",
				"test": "value",
			},
		}
		realtime.InstanceHub("test2.scheduler.io").Publish(&realtime.Event{
			Type: realtime.EventCreate,
			Doc:  &doc,
		})
	})

	wg.Wait()

	assert.True(t, called["message-wholetype"])
	assert.True(t, called["message-correct-verb"])
	assert.True(t, called["message-correct-verb-correct-value"])
	assert.False(t, called["message-bad-verb"])
	assert.False(t, called["message-correct-verb-bad-value"])

	for _, t := range storage.ts {
		sch.Delete(t.ID)
	}

}
