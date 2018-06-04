package share

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/sharing"
)

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "share-track",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      30 * time.Second,
		WorkerFunc:   WorkerTrack,
	})

	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:  "share-replicate",
		Concurrency: runtime.NumCPU(),
		// XXX the worker is not idempotent: if it fails, it adds a new job to
		// retry, but with MaxExecCount > 1, it can amplifies a lot the number
		// of retries
		MaxExecCount: 1,
		Timeout:      5 * time.Minute,
		WorkerFunc:   WorkerReplicate,
	})

	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:  "share-upload",
		Concurrency: runtime.NumCPU(),
		// XXX the worker is not idempotent: if it fails, it adds a new job to
		// retry, but with MaxExecCount > 1, it can amplifies a lot the number
		// of retries
		MaxExecCount: 1,
		Timeout:      1 * time.Hour,
		WorkerFunc:   WorkerUpload,
	})
}

// WorkerTrack is used to update the io.cozy.shared database when a document
// that matches a sharing rule is created/updated/remove
func WorkerTrack(ctx *jobs.WorkerContext) error {
	var msg sharing.TrackMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	var evt sharing.TrackEvent
	if err := ctx.UnmarshalEvent(&evt); err != nil {
		return err
	}
	inst, err := instance.Get(ctx.Domain())
	if err != nil {
		return err
	}
	inst.Logger().WithField("nspace", "share").Debugf("Track %#v - %#v", msg, evt)
	return sharing.UpdateShared(inst, msg, evt)
}

// WorkerReplicate is used for the replication of documents to the other
// members of a sharing.
func WorkerReplicate(ctx *jobs.WorkerContext) error {
	var msg sharing.ReplicateMsg
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	inst, err := instance.Get(ctx.Domain())
	if err != nil {
		return err
	}
	inst.Logger().WithField("nspace", "share").Warnf("Replicate %#v", msg)
	s, err := sharing.FindSharing(inst, msg.SharingID)
	if err != nil {
		return err
	}
	if !s.Active {
		return nil
	}
	return s.Replicate(inst, msg.Errors)
}

// WorkerUpload is used to upload files for a sharing
func WorkerUpload(ctx *jobs.WorkerContext) error {
	var msg sharing.UploadMsg
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	inst, err := instance.Get(ctx.Domain())
	if err != nil {
		return err
	}
	inst.Logger().WithField("nspace", "share").Debugf("Upload %#v", msg)
	s, err := sharing.FindSharing(inst, msg.SharingID)
	if err != nil {
		return err
	}
	if !s.Active {
		return nil
	}
	return s.Upload(inst, msg.Errors)
}
