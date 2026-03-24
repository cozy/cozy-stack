package sharing

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
)

// UpdateSharingDescriptionIfNeeded checks if the given directory is a sharing
// root and triggers a job to update the sharing description if needed.
func UpdateSharingDescriptionIfNeeded(inst *instance.Instance, dir *vfs.DirDoc) {
	// Check if this directory is referenced by any sharings
	for _, ref := range dir.ReferencedBy {
		if ref.Type == consts.Sharings {
			// This directory is a sharing root, trigger an update job
			msg, err := job.NewMessage(&UpdateMsg{
				SharingID:      ref.ID,
				NewDescription: dir.DocName,
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
