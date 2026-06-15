package rag

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
)

// EnsureRAGWebhook creates — once per instance — a @webhook trigger pointing
// to the "rag-index-status" worker and returns its URL. If the trigger already
// exists it is not recreated; its URL is returned directly.
//
// TODO: call this function when the RAG partition is provisioned and pass the
// returned URL to the ragondin indexer so it can POST back when indexing completes.
func EnsureRAGWebhook(inst *instance.Instance) (string, error) {
	log := inst.Logger().WithNamespace("rag")
	sched := job.System()
	triggers, err := sched.GetAllTriggers(inst)
	if err != nil {
		return "", err
	}
	for _, t := range triggers {
		info := t.Infos()
		if info.Type == "@webhook" && info.WorkerType == "rag-index-status" {
			webhookURL := inst.PageURL("/jobs/webhooks/"+t.ID(), nil)
			log.Debugf("RAG webhook trigger already exists: %s", t.ID())
			return webhookURL, nil
		}
	}

	t, err := job.NewTrigger(inst, job.TriggerInfos{
		Type:       "@webhook",
		WorkerType: "rag-index-status",
	}, nil)
	if err != nil {
		return "", err
	}
	if err = sched.AddTrigger(t); err != nil {
		return "", err
	}
	webhookURL := inst.PageURL("/jobs/webhooks/"+t.ID(), nil)
	log.Infof("RAG webhook trigger created: %s", t.ID())
	return webhookURL, nil
}
