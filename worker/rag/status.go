package rag

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "rag-index-status",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      30 * time.Second,
		WorkerFunc:   WorkerIndexStatus,
	})
}

type statusMessage struct {
	Partition string `json:"partition"`
	FileID    string `json:"file_id"`
	Indexed   bool   `json:"indexed"`
}

// WorkerIndexStatus handles jobs triggered by the RAG indexer webhook. It
// updates cozyMetadata.RAG on the file document with the indexation result.
func WorkerIndexStatus(ctx *job.TaskContext) error {
	inst := ctx.Instance
	log := inst.Logger().WithNamespace("rag")

	raw, err := ctx.UnmarshalPayload()
	if err != nil {
		return err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var msg statusMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	if msg.FileID == "" {
		return fmt.Errorf("rag-index-status: missing file_id in payload")
	}

	log.Debugf("Starting rag-index-status job for file %s", msg.FileID)

	fs := inst.VFS()
	file, err := fs.FileByID(msg.FileID)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			log.Debugf("File %s not found (possibly deleted), skipping status update", msg.FileID)
			return nil
		}
		log.Errorf("Failed to get file %s: %v", msg.FileID, err)
		return err
	}

	log.Debugf("File %s: name=%s, mime=%s", msg.FileID, file.DocName, file.Mime)
	log.Debugf("Updating RAG status for file %s: indexed=%v", msg.FileID, msg.Indexed)

	return updateRAGStatus(inst, file, msg.Indexed)
}

// updateRAGStatus writes the RAG indexation result into cozyMetadata.RAG.
// It retries on CouchDB conflict errors (409) up to maxRetries times,
// re-fetching a fresh document revision each iteration to avoid stale-rev
// conflicts. Both the boolean flag and the timestamp are written in a single
// UpdateDoc call to stay atomic.
func updateRAGStatus(inst *instance.Instance, file *vfs.FileDoc, indexed bool) error {
	const maxRetries = 3
	var err error
	log := inst.Logger().WithNamespace("rag")
	fs := inst.VFS()

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			log.Infof("Retrying RAG status update for file %s (attempt %d/%d)", file.DocID, i+1, maxRetries)
		}
		var f *vfs.FileDoc
		f, err = fs.FileByID(file.DocID)
		if err != nil {
			log.Errorf("Failed to get file %s: %v", file.DocID, err)
			return err
		}
		newdoc := f.Clone().(*vfs.FileDoc)
		if newdoc.CozyMetadata == nil {
			newdoc.CozyMetadata = vfs.NewCozyMetadata(inst.Domain)
		}
		if newdoc.CozyMetadata.RAG == nil {
			newdoc.CozyMetadata.RAG = &vfs.RAGMetadata{}
		}
		newdoc.CozyMetadata.RAG.Indexed = indexed
		now := time.Now()
		if indexed {
			newdoc.CozyMetadata.RAG.LastSuccessDate = &now
		} else {
			newdoc.CozyMetadata.RAG.LastErrorDate = &now
		}
		err = couchdb.UpdateDoc(fs, newdoc)
		if err == nil {
			log.Infof("RAG status updated for file %s (indexed=%v)", file.DocID, indexed)
			return nil
		}
		if !couchdb.IsConflictError(err) {
			log.Errorf("Failed to update RAG status for file %s: %v", file.DocID, err)
			return err
		}
	}

	log.Errorf("Failed to update RAG status for file %s after %d retries: %v", file.DocID, maxRetries, err)
	return err
}
