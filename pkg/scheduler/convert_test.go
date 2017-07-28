package scheduler_test

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/stretchr/testify/assert"
)

func TestTriggersFromMonoStackAreImportedInRedit(t *testing.T) {

	oldsched := scheduler.NewMemScheduler()
	trig, err := scheduler.NewTrigger(&scheduler.TriggerInfos{
		Domain:    "triggers-convert.cozy.tools",
		Type:      "@in",
		Arguments: "1h",
	})
	if !assert.NoError(t, err) {
		return
	}

	err = oldsched.Add(trig)
	if !assert.NoError(t, err) {
		return
	}

	newsched := stack.GetScheduler().(*scheduler.RedisScheduler)
	err = newsched.ImportFromMemStorage()
	if !assert.NoError(t, err) {
		return
	}

	tt, err := newsched.GetAll("triggers-convert.cozy.tools")
	if !assert.NoError(t, err) {
		return
	}

	found := false
	for _, t := range tt {
		if t.ID() == trig.ID() {
			found = true
		}
	}
	assert.True(t, found, "the trigger has been copied")
	defer newsched.Delete("triggers-convert.cozy.tools", trig.ID())

	memsched2 := scheduler.NewMemScheduler()
	bro := jobs.NewMemBroker(1)
	bro.Start(jobs.WorkersList{})
	memsched2.Start(bro)
	oldtrigkept, err := memsched2.Get("triggers-convert.cozy.tools", trig.ID())
	assert.Nil(t, oldtrigkept, "old trigger has been deleted")
	assert.Error(t, err, "old trigger has been deleted")
}
