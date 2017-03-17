package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestTriggerEvent(t *testing.T) {

	var wg sync.WaitGroup

	wg.Add(1)
	NewMemBroker("test2.scheduler.io", WorkersList{
		"worker_event": {
			Concurrency:  1,
			MaxExecCount: 1,
			Timeout:      1 * time.Millisecond,
			WorkerFunc: func(ctx context.Context, m *Message) error {
				var msg struct {
					Message *string
					Event   realtime.Event
				}
				if err := m.Unmarshal(&msg); err != nil {
					assert.Equal(t, "test-id", msg.Event.DocID)
					assert.Equal(t, "message-for-worker-event", msg.Message)
					return err
				}
				wg.Done()
				return nil
			},
		},
	})

	msg, _ := NewMessage("json", "message-for-worker-event")

	id := utils.RandomString(10)
	trigger := &TriggerInfos{
		ID:         id,
		Type:       "@event",
		Arguments:  "io.cozy.testeventobject",
		WorkerType: "worker_event",
		Message:    msg,
	}
	NewMemScheduler("test2.scheduler.io", &storage{[]*TriggerInfos{trigger}})
	bro := GetMemBroker("test2.scheduler.io")
	sch := GetMemScheduler("test2.scheduler.io")
	sch.Start(bro)

	ts, err := sch.GetAll()
	assert.NoError(t, err)
	assert.Len(t, ts, 1)

	time.AfterFunc(1*time.Millisecond, func() {
		realtime.InstanceHub("test2.scheduler.io").Publish(&realtime.Event{
			Type:    realtime.EventCreate,
			DocType: "io.cozy.testeventobject",
			DocID:   "test-id",
			DocRev:  "1-xxabxx",
		})
	})

	wg.Wait()
}
