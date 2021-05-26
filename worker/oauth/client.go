package oauth

import (
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "clean-clients",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Reserved:     true,
		Timeout:      30 * time.Second,
		WorkerFunc:   WorkerClean,
	})
}

// WorkerClean is used to clean unused OAuth clients.
func WorkerClean(ctx *job.WorkerContext) error {
	var msg oauth.CleanMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	client, err := oauth.FindClient(ctx.Instance, msg.ClientID)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if client.Pending {
		return couchdb.DeleteDoc(ctx.Instance, client)
	}
	return nil
}
