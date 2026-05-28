package sharing

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// UpdateSharingDescriptionIfNeeded checks if the given references contain
// sharing references and triggers a job to update the sharing metadata.
func UpdateSharingDescriptionIfNeeded(inst *instance.Instance, referencedBy []couchdb.DocReference, newDescription string) {
	// Check if this file/directory is referenced by any sharings
	for _, ref := range referencedBy {
		if ref.Type == consts.Sharings {
			// This file/directory is a sharing root, trigger an update job
			msg, err := job.NewMessage(&UpdateMsg{
				SharingID:      ref.ID,
				NewDescription: newDescription,
			})
			if err != nil {
				inst.Logger().WithNamespace("sharing").
					Warnf("Failed to create share-update message: %s", err)
				continue
			}

			_, err = job.System().PushJob(inst, &job.JobRequest{
				WorkerType: "share-update",
				Message:    msg,
			})
			if err != nil {
				inst.Logger().WithNamespace("sharing").
					Warnf("Failed to push share-update job for sharing %s: %s", ref.ID, err)
			}
		}
	}
}
