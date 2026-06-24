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
	Partition       string     `json:"partition"`
	FileID          string     `json:"file_id"`
	Indexed         bool       `json:"indexed"`
	LastSuccessDate *time.Time `json:"last_success_date,omitempty"`
	LastErrorDate   *time.Time `json:"last_error_date,omitempty"`
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

	file, err := inst.VFS().FileByID(msg.FileID)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			log.Debugf("rag-index-status: file %s not found, skipping", msg.FileID)
			return nil
		}
		return err
	}

	return updateRAGStatus(inst, file, msg)
}

func updateRAGStatus(inst *instance.Instance, file *vfs.FileDoc, msg statusMessage) error {
	fs := inst.VFS()

	err := couchdb.UpdateDoc(fs, ragStatusDoc(inst, file, msg))
	if err == nil || !couchdb.IsConflictError(err) {
		return err
	}

	// A conflict means a parallel webhook already updated the file. Re-fetch it
	// and overwrite only when our event is more recent than the stored one,
	// otherwise the latest status would be lost.
	fresh, err := fs.FileByID(file.DocID)
	if err != nil {
		return err
	}
	if cm := fresh.CozyMetadata; cm != nil && cm.RAG != nil && !msgIsNewer(msg, cm.RAG) {
		return nil
	}
	return couchdb.UpdateDoc(fs, ragStatusDoc(inst, fresh, msg))
}

func ragStatusDoc(inst *instance.Instance, file *vfs.FileDoc, msg statusMessage) *vfs.FileDoc {
	newdoc := file.Clone().(*vfs.FileDoc)
	if newdoc.CozyMetadata == nil {
		newdoc.CozyMetadata = vfs.NewCozyMetadata(inst.Domain)
	}
	if newdoc.CozyMetadata.RAG == nil {
		newdoc.CozyMetadata.RAG = &vfs.RAGMetadata{}
	}
	newdoc.CozyMetadata.RAG.Indexed = msg.Indexed
	if msg.LastSuccessDate != nil {
		newdoc.CozyMetadata.RAG.LastSuccessDate = msg.LastSuccessDate
	}
	if msg.LastErrorDate != nil {
		newdoc.CozyMetadata.RAG.LastErrorDate = msg.LastErrorDate
	}
	return newdoc
}

func msgIsNewer(msg statusMessage, rag *vfs.RAGMetadata) bool {
	incoming := latestDate(msg.LastSuccessDate, msg.LastErrorDate)
	if incoming == nil {
		return false
	}
	stored := latestDate(rag.LastSuccessDate, rag.LastErrorDate)
	return stored == nil || incoming.After(*stored)
}

func latestDate(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b != nil && b.After(*a) {
		return b
	}
	return a
}
