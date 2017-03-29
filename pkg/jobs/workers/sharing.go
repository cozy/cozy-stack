package workers

import (
	"context"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
)

func init() {
	jobs.AddWorker("sharing", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 3,
		Timeout:      10 * time.Second,
		WorkerFunc:   SharingUpdates,
	})
}

// SharingUpdates checks the shared doc updates
func SharingUpdates(ctx context.Context, m *jobs.Message) error {
	doc := &couchdb.JSONDoc{}
	err := doc.UnmarshalJSON(m.Data)
	//TODO : call a senddoc worker to send updated files
	return err
}
