package job

import (
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

// EnsureCleanOldTrashedTrigger schedules cleanup using the instance's retention policy.
func EnsureCleanOldTrashedTrigger(inst *instance.Instance) {
	// 1. Check if we need a trigger for clean-old-trashed worker
	cfg := config.GetConfig().Fs.AutoCleanTrashedAfter
	after, ok := cfg[inst.ContextName]
	if !ok || after == "" {
		return
	}

	// 2. Check if the trigger already exists
	sched := System()
	infos := TriggerInfos{
		Type:       "@cron",
		WorkerType: "clean-old-trashed",
	}
	if sched.HasTrigger(inst, infos) {
		return
	}

	// 3. Create the trigger
	now := time.Now()
	hours := (now.Hour() + 12) % 24
	infos.Arguments = fmt.Sprintf("0 %d %d * * *", now.Minute(), hours)
	trigger, err := NewTrigger(inst, infos, nil)
	if err != nil {
		inst.Logger().Errorf("Cannot create clean-old-trashed trigger: %s", err)
		return
	}
	if err = sched.AddTrigger(trigger); err != nil {
		inst.Logger().Errorf("Cannot create clean-old-trashed trigger: %s", err)
	}
}
