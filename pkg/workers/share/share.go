package share

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/sharing"
)

func init() {
	if !config.IsDevRelease() {
		return
	}

	// TODO write documentation about this worker
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "share-replicate",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      300 * time.Second,
		WorkerFunc:   WorkerReplicate,
	})
}

// WorkerReplicate is used for the replication of documents to the other
// members of a sharing.
func WorkerReplicate(ctx *jobs.WorkerContext) error {
	var msg sharing.ReplicateMsg
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	logger.WithDomain(ctx.Domain()).Info(msg)
	inst, err := instance.Get(ctx.Domain())
	if err != nil {
		return err
	}
	s, err := sharing.FindSharing(inst, msg.SharingID)
	if err != nil {
		return err
	}
	return s.Replicate(inst)
}
