package rag

import (
	"errors"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
)

// ragStatusTriggerID is the fixed ID of the per-instance @webhook trigger that
// feeds the "rag-index-status" worker, so it can be looked up in a single query.
const ragStatusTriggerID = "rag-index-status"

// EnsureRAGWebhook returns the URL of the per-instance @webhook trigger feeding
// the "rag-index-status" worker, creating it on the first call.
func EnsureRAGWebhook(inst *instance.Instance) (string, error) {
	sched := job.System()

	t, err := sched.GetTrigger(inst, ragStatusTriggerID)
	if err != nil {
		if !errors.Is(err, job.ErrNotFoundTrigger) {
			return "", err
		}
		t, err = job.NewTrigger(inst, job.TriggerInfos{
			TID:        ragStatusTriggerID,
			Type:       "@webhook",
			WorkerType: "rag-index-status",
		}, nil)
		if err != nil {
			return "", err
		}
		if err = sched.AddTrigger(t); err != nil {
			return "", err
		}
		inst.Logger().WithNamespace("rag").Infof("RAG webhook trigger created: %s", t.ID())
	}
	return inst.PageURL("/jobs/webhooks/"+t.ID(), nil), nil
}
