package job_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrigger(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance()

	t.Run("TriggerEvent", func(t *testing.T) {
		var wg sync.WaitGroup
		called := make(map[string]bool)
		verb := "CREATED"

		bro := job.NewMemBroker()
		assert.NoError(t, bro.StartWorkers(job.WorkersList{
			{
				WorkerType:   "worker_event",
				Concurrency:  1,
				MaxExecCount: 1,
				Timeout:      1 * time.Millisecond,
				WorkerFunc: func(ctx *job.TaskContext) error {
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
					assert.Equal(t, verb, evt.Verb)
					assert.Equal(t, "test-id", evt.Doc.ID())
					called[msg] = true
					return nil
				},
			},
		}))

		var triggers []job.Trigger
		triggersInfos := []job.TriggerInfos{
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
				Arguments:  "io.cozy.testeventobject:UPDATED:!=:test",
				WorkerType: "worker_event",
				Message:    makeMessage(t, "message-change"),
			},
			{
				Type:       "@event",
				Arguments:  "io.cozy.testeventobject",
				WorkerType: "worker_event",
				Message:    makeMessage(t, "message-wholetype"),
			},
		}

		sch := job.NewMemScheduler()
		assert.NoError(t, sch.StartScheduler(bro))

		for _, infos := range triggersInfos {
			trigger, err := job.NewTrigger(testInstance, infos, infos.Message)
			require.NoError(t, err)

			err = sch.AddTrigger(trigger)
			require.NoError(t, err)

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

		assert.True(t, called["message-correct-verb"])
		assert.True(t, called["message-correct-verb-correct-value"])
		assert.True(t, called["message-wholetype"])
		assert.False(t, called["message-bad-verb"])
		assert.False(t, called["message-correct-verb-bad-value"])
		assert.False(t, called["message-change"])

		delete(called, "message-correct-verb")
		delete(called, "message-correct-verb-correct-value")
		delete(called, "message-wholetype")

		wg.Add(1)
		verb = "UPDATED"

		time.AfterFunc(1*time.Millisecond, func() {
			doc := couchdb.JSONDoc{
				Type: "io.cozy.testeventobject",
				M: map[string]interface{}{
					"_id":  "test-id",
					"_rev": "2-xxcdxx",
					"test": "value",
				},
			}
			olddoc := couchdb.JSONDoc{
				Type: "io.cozy.testeventobject",
				M: map[string]interface{}{
					"_id":  "test-id",
					"_rev": "1-xxabxx",
					"test": "value",
				},
			}
			realtime.GetHub().Publish(testInstance, realtime.EventUpdate, &doc, &olddoc)
		})

		wg.Wait()

		assert.True(t, called["message-wholetype"])
		assert.False(t, called["message-correct-verb"])
		assert.False(t, called["message-correct-verb-correct-value"])
		assert.False(t, called["message-bad-verb"])
		assert.False(t, called["message-correct-verb-bad-value"])
		assert.False(t, called["message-change"])

		delete(called, "message-wholetype")

		wg.Add(2)

		time.AfterFunc(1*time.Millisecond, func() {
			doc := couchdb.JSONDoc{
				Type: "io.cozy.testeventobject",
				M: map[string]interface{}{
					"_id":  "test-id",
					"_rev": "3-xxefxx",
					"test": "changed",
				},
			}
			olddoc := couchdb.JSONDoc{
				Type: "io.cozy.testeventobject",
				M: map[string]interface{}{
					"_id":  "test-id",
					"_rev": "2-xxcdxx",
					"test": "value",
				},
			}
			realtime.GetHub().Publish(testInstance, realtime.EventUpdate, &doc, &olddoc)
		})

		wg.Wait()

		assert.False(t, called["message-correct-verb"])
		assert.False(t, called["message-correct-verb-correct-value"])
		assert.False(t, called["message-bad-verb"])
		assert.False(t, called["message-correct-verb-bad-value"])
		assert.True(t, called["message-change"])
		assert.True(t, called["message-wholetype"])

		for _, trigger := range triggers {
			err := sch.DeleteTrigger(testInstance, trigger.ID())
			assert.NoError(t, err)
		}

		err := sch.ShutdownScheduler(context.Background())
		assert.NoError(t, err)
	})
}

func makeMessage(t *testing.T, msg string) job.Message {
	out, err := job.NewMessage(msg)
	assert.NoError(t, err)
	return out
}
