package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/stretchr/testify/assert"
)

type storage struct {
	ts []*TriggerInfos
}

func (s *storage) GetAll() ([]*TriggerInfos, error) { return s.ts, nil }
func (s *storage) Add(trigger Trigger) error        { return nil }
func (s *storage) Delete(trigger Trigger) error     { return nil }

func TestTriggersBadArguments(t *testing.T) {
	var err error
	_, err = NewTrigger(&TriggerInfos{
		TID:       utils.RandomString(10),
		Domain:    "cozy.local",
		Type:      "@at",
		Arguments: "garbage",
	})
	assert.Error(t, err)

	_, err = NewTrigger(&TriggerInfos{
		TID:       utils.RandomString(10),
		Type:      "@in",
		Arguments: "garbage",
	})
	assert.Error(t, err)

	_, err = NewTrigger(&TriggerInfos{
		TID:       utils.RandomString(10),
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

	atID := utils.RandomString(10)
	at := &TriggerInfos{
		TID:        atID,
		Type:       "@at",
		Domain:     "cozy.local",
		Arguments:  time.Now().Add(2 * time.Second).Format(time.RFC3339),
		WorkerType: "worker",
		Message:    msg1,
	}
	inID := utils.RandomString(10)
	in := &TriggerInfos{
		TID:        inID,
		Domain:     "cozy.local",
		Type:       "@in",
		Arguments:  "1s",
		WorkerType: "worker",
		Message:    msg2,
	}

	triggers := []*TriggerInfos{at, in}
	sch := newMemScheduler(&storage{triggers})

	sch.Start(bro)

	ts, err := sch.GetAll("cozy.local")
	assert.NoError(t, err)
	assert.Len(t, ts, len(triggers))

	for _, trigger := range ts {
		switch trigger.Infos().TID {
		case atID:
			assert.Equal(t, at, trigger.Infos())
		case inID:
			assert.Equal(t, in, trigger.Infos())
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

	_, err = sch.Get("cozy.local", atID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotFoundTrigger, err)

	_, err = sch.Get("cozy.local", inID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotFoundTrigger, err)
}
