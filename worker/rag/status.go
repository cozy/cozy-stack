package rag

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "rag-index-status",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      15 * time.Second,
		WorkerFunc:   WorkerIndexStatus,
	})
}

// WorkerIndexStatus handles the "rag-index-status" jobs triggered by the RAG
// indexer webhook. It updates the rag_status field in the file metadata.
// Expected payload: {"file_id": string, "status": string}
//
// CouchDB write conflicts (409) are expected under concurrent updates and are
// retried locally with a short incremental backoff, without consuming the
// job's own MaxExecCount retry budget (which stays reserved for genuine
// failures, e.g. network or instance issues).
func WorkerIndexStatus(ctx *job.TaskContext) error {
	logger := ctx.Logger()
	payload, err := ctx.UnmarshalPayload()
	if err != nil {
		return err
	}
	fileID, _ := payload["file_id"].(string)
	if fileID == "" {
		return errors.New("rag-index-status: missing file_id in payload")
	}
	status, _ := payload["status"].(string)
	if status == "" {
		return errors.New("rag-index-status: missing status in payload")
	}
	logger.Debugf("RAG: received rag_status=%q for file %s", status, fileID)

	fs := ctx.Instance.VFS()
	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			delay := time.Duration(i) * 100 * time.Millisecond
			logger.Infof("RAG: retrying rag_status update for file %s (attempt %d/%d) after %s", fileID, i+1, maxRetries, delay)
			time.Sleep(delay)
		}
		olddoc, err := fs.FileByID(fileID) // relecture fraîche à chaque tour
		if err != nil {
			return err
		}
		newdoc := olddoc.Clone().(*vfs.FileDoc)
		if newdoc.Metadata == nil {
			newdoc.Metadata = make(vfs.Metadata)
		}
		newdoc.Metadata["rag_status"] = status
		if err = fs.UpdateFileDoc(olddoc, newdoc); err == nil {
			logger.Infof("RAG: rag_status set to %q for file %s", status, fileID)
			return nil
		} else if !couchdb.IsConflictError(err) {
			return err
		}
		// 409 → on reboucle, FileByID redonne le rev à jour
	}
	return fmt.Errorf("rag-index-status: conflict after %d retries for file %s", maxRetries, fileID)
}
